// Package server — project_router.go
//
// Per-account, lazy-loaded matcher from candidate workspace paths to a
// project id. Projects are user-bound: every lookup is keyed by account_id
// and a user cannot see another user's matches.
//
// Concurrency design:
//   - accounts: sync.Map[int32]*accountState. Reads (the common case) are
//     lock-free; writes (first-load-per-account, invalidation) avoid global
//     contention.
//   - Each account's matcher is an atomic.Pointer[prefixMatcher]. After the
//     initial load, every Match for that account is wait-free.
//   - accountState.loadMu serialises only the *first* load (or first reload
//     after invalidation) for one account; it never blocks other accounts.
//
// Net effect: a user editing their projects (which calls
// InvalidateAccount(theirID)) costs that user one extra DB load on their
// next request. Every other user's lookups continue unaffected — no shared
// locks held, no shared caches touched.
//
// Telemetry: Stats() returns hit/miss/load counters plus the active bucket
// count. Loads exceeding slowLoadThreshold are also logged at WARN so a
// noisy account is visible without polling.
//
// Any future writer of the project table MUST call InvalidateAccount(id)
// at the same site so the next Match for that user observes the change.
package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"picotera/pkg/db"
	"picotera/pkg/logx"

	"github.com/sirupsen/logrus"
)

type projectEntry struct {
	path      string
	projectID int32
}

// accountState owns one account's bucket. matcher is read via atomic Load
// on the hot path; loadMu serialises construction of the initial matcher.
type accountState struct {
	loadMu  sync.Mutex
	matcher atomic.Pointer[prefixMatcher]
}

type projectRouter struct {
	queries *db.Queries

	// accounts: map[int32]*accountState. sync.Map fits the workload — many
	// repeated reads of the same keys, occasional writes (first-load,
	// invalidate) that must not block other accounts.
	accounts sync.Map

	// Telemetry. atomic counters; Stats() snapshots them.
	hits           atomic.Int64
	misses         atomic.Int64
	loads          atomic.Int64
	loadFailures   atomic.Int64
	loadTotalNanos atomic.Int64
}

// slowLoadThreshold marks a per-account load that took long enough to be
// worth surfacing to operators. Tune downward if loads stay well under it.
const slowLoadThreshold = 100 * time.Millisecond

func newProjectRouter(q *db.Queries) *projectRouter {
	return &projectRouter{queries: q}
}

// Match returns the project id whose path is the longest prefix of any
// candidate within accountID's bucket. Lazy-loads the bucket on first
// touch. Returns (0, false, nil) when no entry matches, candidates is
// empty, or accountID is zero (no authenticated caller).
func (r *projectRouter) Match(ctx context.Context, accountID int32, candidates []string) (int32, bool, error) {
	if accountID == 0 || len(candidates) == 0 {
		return 0, false, nil
	}
	state := r.getOrCreate(accountID)
	m := state.matcher.Load()
	if m == nil {
		var err error
		m, err = r.ensureLoaded(ctx, accountID, state)
		if err != nil {
			return 0, false, err
		}
	}
	id, ok := m.match(candidates)
	if ok {
		r.hits.Add(1)
	} else {
		r.misses.Add(1)
	}
	return id, ok, nil
}

// InvalidateAccount drops one account's cached matcher. The next Match for
// that account reloads from the DB; other accounts are untouched. Mutation
// paths (handle_project.go writes, handleDeleteAccount) MUST call this
// with the affected account_id.
func (r *projectRouter) InvalidateAccount(accountID int32) {
	if accountID == 0 {
		return
	}
	r.accounts.Delete(accountID)
}

// Invalidate drops every account's bucket. Reserved for cases where a
// targeted InvalidateAccount isn't possible (e.g., a future schema
// migration). Day to day, prefer InvalidateAccount.
func (r *projectRouter) Invalidate() {
	r.accounts.Range(func(k, _ any) bool {
		r.accounts.Delete(k)
		return true
	})
}

// ProjectRouterStats is a snapshot of telemetry counters. Buckets is the
// number of accounts currently cached (visited by Match since startup or
// last Invalidate, minus those evicted by InvalidateAccount).
type ProjectRouterStats struct {
	Buckets        int
	Hits           int64
	Misses         int64
	Loads          int64
	LoadFailures   int64
	LoadTotalNanos int64
}

func (r *projectRouter) Stats() ProjectRouterStats {
	buckets := 0
	r.accounts.Range(func(_, _ any) bool {
		buckets++
		return true
	})
	return ProjectRouterStats{
		Buckets:        buckets,
		Hits:           r.hits.Load(),
		Misses:         r.misses.Load(),
		Loads:          r.loads.Load(),
		LoadFailures:   r.loadFailures.Load(),
		LoadTotalNanos: r.loadTotalNanos.Load(),
	}
}

func (r *projectRouter) getOrCreate(accountID int32) *accountState {
	if v, ok := r.accounts.Load(accountID); ok {
		return v.(*accountState)
	}
	fresh := &accountState{}
	actual, _ := r.accounts.LoadOrStore(accountID, fresh)
	return actual.(*accountState)
}

// ensureLoaded fetches the account's project paths and stores the built
// matcher on state. Per-account mutex prevents the thundering-herd case
// where many concurrent first-requests for the same account each issue
// their own DB query.
//
// On error the state is removed from the map so a subsequent Match
// retries — otherwise a single transient DB blip would permanently fail
// matches for that account until the next mutation/invalidate.
func (r *projectRouter) ensureLoaded(ctx context.Context, accountID int32, state *accountState) (*prefixMatcher, error) {
	state.loadMu.Lock()
	defer state.loadMu.Unlock()
	if m := state.matcher.Load(); m != nil {
		return m, nil // raced with another goroutine
	}
	start := time.Now()
	rows, err := r.queries.ListProjectPathsByAccount(ctx, accountID)
	elapsed := time.Since(start)
	r.loads.Add(1)
	r.loadTotalNanos.Add(elapsed.Nanoseconds())
	if err != nil {
		r.loadFailures.Add(1)
		r.accounts.CompareAndDelete(accountID, state)
		return nil, fmt.Errorf("project router: load account %d: %w", accountID, err)
	}
	if elapsed >= slowLoadThreshold {
		logx.WithContext(ctx).WithFields(logrus.Fields{
			"event":      "project_router.slow_load",
			"account_id": accountID,
			"entries":    len(rows),
			"elapsed_ms": elapsed.Milliseconds(),
		}).Warn("project router: slow load")
	}
	entries := make([]projectEntry, 0, len(rows))
	for _, row := range rows {
		if row.Path == "" {
			continue
		}
		entries = append(entries, projectEntry{path: row.Path, projectID: row.ProjectID})
	}
	m := buildPrefixMatcher(entries)
	state.matcher.Store(m)
	return m, nil
}

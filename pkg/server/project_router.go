// Package server — project_router.go
//
// In-memory longest-prefix matcher from a project's path candidates to a
// project id, scoped per account. Projects are user-bound: two users with
// the same paths must not see each other's project ids, so every lookup
// is keyed by account_id.
//
// Hot-path constraint: the gateway resolves a project on every request,
// so we cannot afford a DB round-trip per call. The router caches each
// account's entries on first use and serves subsequent calls from
// memory; the endpoint router uses the same pattern but with a single
// global bucket (endpoints are admin-configured, not user-bound).
//
// Multi-user design notes:
//   - Buckets load lazily, ONE account at a time. We never fetch every
//     user's projects in a single query — that would scale with total
//     user count and the cache would hold rows for accounts that may
//     never call the gateway.
//   - InvalidateAccount(id) drops one bucket; project mutations use it.
//     Invalidate() (global) stays for rare cases (e.g., a future schema
//     migration). Without per-account invalidation, every user mutation
//     would thrash every other user's cache.
//   - A "loaded empty" account is represented by a non-nil zero-length
//     slice in byAcct so we don't repeatedly hit the DB for accounts
//     that genuinely have no projects.
//
// Any future writer of the project table MUST call
// Server.projectRouter.InvalidateAccount(accountID) at the same site.
package server

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"picotera/pkg/db"
)

type projectEntry struct {
	path      string
	projectID int32
}

type projectRouter struct {
	queries *db.Queries

	mu     sync.RWMutex
	byAcct map[int32][]projectEntry // per-account, each slice sorted: len(path) desc, then projectID asc
}

func newProjectRouter(q *db.Queries) *projectRouter {
	return &projectRouter{
		queries: q,
		byAcct:  make(map[int32][]projectEntry),
	}
}

// Match returns the project id whose path is a prefix of any candidate for
// accountID, longest path first. Returns (0, false) when no entry matches,
// candidates is empty, or accountID is zero. Loads the account's bucket on
// first use; subsequent calls are served from memory.
func (r *projectRouter) Match(ctx context.Context, accountID int32, candidates []string) (int32, bool, error) {
	if accountID == 0 || len(candidates) == 0 {
		return 0, false, nil
	}

	r.mu.RLock()
	if entries, ok := r.byAcct[accountID]; ok {
		id, matched := matchEntries(entries, candidates)
		r.mu.RUnlock()
		return id, matched, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	// Re-check under write lock: a parallel call may have loaded it already.
	if entries, ok := r.byAcct[accountID]; ok {
		id, matched := matchEntries(entries, candidates)
		r.mu.Unlock()
		return id, matched, nil
	}
	if err := r.loadAccountLocked(ctx, accountID); err != nil {
		r.mu.Unlock()
		return 0, false, err
	}
	entries := r.byAcct[accountID]
	r.mu.Unlock()

	id, matched := matchEntries(entries, candidates)
	return id, matched, nil
}

func matchEntries(entries []projectEntry, candidates []string) (int32, bool) {
	for _, e := range entries {
		for _, c := range candidates {
			if strings.HasPrefix(c, e.path) {
				return e.projectID, true
			}
		}
	}
	return 0, false
}

// InvalidateAccount drops one account's cached bucket. The next Match for
// that account will reload from the DB. Cheap (one map delete) and bounded
// in blast radius — other accounts' caches are untouched.
//
// Mutation paths (handle_project.go writes, gateway_helpers.go auto-create)
// MUST call this with the account whose projects changed.
func (r *projectRouter) InvalidateAccount(accountID int32) {
	if accountID == 0 {
		return
	}
	r.mu.Lock()
	delete(r.byAcct, accountID)
	r.mu.Unlock()
}

// Invalidate drops every account's bucket. Reserved for cases where a
// targeted InvalidateAccount isn't possible (e.g., schema migrations). Day
// to day, prefer InvalidateAccount(accountID) — global invalidation forces
// every active account to reload on its next request.
func (r *projectRouter) Invalidate() {
	r.mu.Lock()
	r.byAcct = make(map[int32][]projectEntry)
	r.mu.Unlock()
}

// loadAccountLocked fetches a single account's project paths and stores
// them in byAcct[accountID]. Caller must hold r.mu for writing. Always
// stores a non-nil slice (possibly empty) so a subsequent Match knows the
// account is loaded and doesn't re-hit the DB.
func (r *projectRouter) loadAccountLocked(ctx context.Context, accountID int32) error {
	rows, err := r.queries.ListProjectPathsByAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("project router: load account %d: %w", accountID, err)
	}
	entries := make([]projectEntry, 0, len(rows))
	for _, row := range rows {
		if row.Path == "" {
			continue
		}
		entries = append(entries, projectEntry{
			path:      row.Path,
			projectID: row.ProjectID,
		})
	}
	sortProjectEntries(entries)
	r.byAcct[accountID] = entries
	return nil
}

func sortProjectEntries(entries []projectEntry) {
	// len(path) desc, ties broken by projectID asc.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			a, b := entries[j-1], entries[j]
			if len(a.path) > len(b.path) {
				break
			}
			if len(a.path) == len(b.path) && a.projectID <= b.projectID {
				break
			}
			entries[j-1], entries[j] = b, a
		}
	}
}

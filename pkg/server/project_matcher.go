// Package server — project_matcher.go
//
// prefixMatcher is a byte-keyed trie over (path, projectID) entries. Lookup
// cost is O(longest candidate) per candidate, independent of how many
// entries are stored. Replaces the previous linear-scan-over-sorted-slice
// matcher: handles a bucket of 5 paths and a bucket of 5000 paths in the
// same time for any single candidate.
//
// Match semantics: byte-prefix match on the normalized path form. Returns
// the projectID of the entry whose path is the longest prefix of any
// candidate. Ties at equal depth are broken by smallest id.
//
// Trailing-slash convention (user-facing):
//   • `/foo/bar` (no trailing slash) — loose prefix. Matches `/foo/bar/x`
//     AND `/foo/bar-baz/x`. Backward-compatible with pre-trailing-slash
//     configurations.
//   • `/foo/bar/` (trailing slash) — strict directory prefix. The trailing
//     `/` is part of the literal prefix, so `/foo/bar-baz/x` no longer
//     matches because the next byte differs.
//
// Cross-platform: paths are normalized to forward-slash separators before
// insertion + matching. Configuring `C:\Users\foo\` and a request body
// carrying `C:\Users\foo\src` (or `C:/Users/foo/src`) both work — they
// share the canonical `C:/Users/foo/` representation in the trie.
package server

import "strings"

// normalizePath canonicalizes a path for prefix matching. The only
// transformation today is backslash → forward-slash so Windows-style
// paths (C:\Users\foo) and POSIX paths (/home/foo) share trie semantics.
// Case is preserved — picotera doesn't try to be smart about Windows
// case-insensitivity (users should configure paths in the case the
// originating tool emits).
func normalizePath(p string) string {
	if !strings.ContainsRune(p, '\\') {
		return p
	}
	return strings.ReplaceAll(p, "\\", "/")
}

// prefixMatcher is read-only after construction; safe to share across
// goroutines via an atomic.Pointer.
type prefixMatcher struct {
	root *trieNode
}

type trieNode struct {
	children  map[byte]*trieNode
	isTerm    bool
	projectID int32
}

// buildPrefixMatcher constructs an immutable matcher from entries. Empty
// paths are skipped. Duplicate paths (shouldn't happen — UNIQUE(account_id,
// name) does not extend to paths but is unlikely in practice) keep the
// smallest projectID, mirroring the old sort's tie-break.
func buildPrefixMatcher(entries []projectEntry) *prefixMatcher {
	root := &trieNode{}
	for _, e := range entries {
		path := normalizePath(e.path)
		if path == "" {
			continue
		}
		node := root
		for i := 0; i < len(path); i++ {
			ch := path[i]
			if node.children == nil {
				node.children = make(map[byte]*trieNode)
			}
			child, ok := node.children[ch]
			if !ok {
				child = &trieNode{}
				node.children[ch] = child
			}
			node = child
		}
		if !node.isTerm || e.projectID < node.projectID {
			node.isTerm = true
			node.projectID = e.projectID
		}
	}
	return &prefixMatcher{root: root}
}

// matchOne walks a single candidate down the trie, recording each terminal
// it passes through. Returns the deepest match (longest prefix) and the
// projectID, with smallest-id tiebreak at equal depth.
func (m *prefixMatcher) matchOne(c string, bestID int32, bestLen int) (int32, int) {
	node := m.root
	for i := 0; i < len(c); i++ {
		if node.isTerm && (i > bestLen || (i == bestLen && node.projectID < bestID)) {
			bestID = node.projectID
			bestLen = i
		}
		child, ok := node.children[c[i]]
		if !ok {
			return bestID, bestLen
		}
		node = child
	}
	if node.isTerm {
		l := len(c)
		if l > bestLen || (l == bestLen && node.projectID < bestID) {
			bestID = node.projectID
			bestLen = l
		}
	}
	return bestID, bestLen
}

// match walks each candidate down the trie. At every node it visits, a
// terminal marks an entry whose path equals the candidate prefix at that
// depth — recording (id, depth) lets us return the longest such match
// across all candidates without revisiting the trie.
//
// Candidates are normalized first so a Windows path in the request body
// (C:\Users\foo\src) matches a forward-slash-stored entry (C:/Users/foo/).
func (m *prefixMatcher) match(candidates []string) (int32, bool) {
	var bestID int32
	bestLen := -1
	for _, c := range candidates {
		bestID, bestLen = m.matchOne(normalizePath(c), bestID, bestLen)
	}
	return bestID, bestLen >= 0
}

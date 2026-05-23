// Package server — project_matcher.go
//
// prefixMatcher is a byte-keyed trie over (path, projectID) entries. Lookup
// cost is O(longest candidate) per candidate, independent of how many
// entries are stored. Replaces the previous linear-scan-over-sorted-slice
// matcher: handles a bucket of 5 paths and a bucket of 5000 paths in the
// same time for any single candidate.
//
// Match semantics mirror the original sort-by-length-desc / projectID-asc
// behaviour: returns the projectID of the entry whose path is the longest
// prefix of any candidate. Ties at equal depth are broken by smallest id.
package server

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
		if e.path == "" {
			continue
		}
		node := root
		for i := 0; i < len(e.path); i++ {
			ch := e.path[i]
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

// match walks each candidate down the trie. At every node it visits, a
// terminal marks an entry whose path equals the candidate prefix at that
// depth — recording (id, depth) lets us return the longest such match
// across all candidates without revisiting the trie.
func (m *prefixMatcher) match(candidates []string) (int32, bool) {
	var bestID int32
	bestLen := -1
	for _, c := range candidates {
		node := m.root
		for i := 0; i < len(c); i++ {
			if node.isTerm && (i > bestLen || (i == bestLen && node.projectID < bestID)) {
				bestID = node.projectID
				bestLen = i
			}
			child, ok := node.children[c[i]]
			if !ok {
				break
			}
			node = child
		}
		// Exact match (entry.path == c) terminates at the final node.
		if node.isTerm {
			l := len(c)
			if l > bestLen || (l == bestLen && node.projectID < bestID) {
				bestID = node.projectID
				bestLen = l
			}
		}
	}
	return bestID, bestLen >= 0
}

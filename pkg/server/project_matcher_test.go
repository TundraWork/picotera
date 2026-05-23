package server

import "testing"

func TestPrefixMatcher_LongestPrefixWins(t *testing.T) {
	m := buildPrefixMatcher([]projectEntry{
		{path: "/home", projectID: 3},
		{path: "/home/user/foo", projectID: 1},
		{path: "/home/user/foo/sub", projectID: 2},
	})
	cases := []struct {
		candidate string
		wantID    int32
		wantOK    bool
	}{
		{"/home/user/foo/sub/main.go", 2, true},
		{"/home/user/foo/main.go", 1, true},
		{"/home/other", 3, true},
		{"/elsewhere", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		id, ok := m.match([]string{c.candidate})
		if ok != c.wantOK || id != c.wantID {
			t.Errorf("candidate %q: want (%d, %v), got (%d, %v)", c.candidate, c.wantID, c.wantOK, id, ok)
		}
	}
}

func TestPrefixMatcher_ExactPathMatches(t *testing.T) {
	m := buildPrefixMatcher([]projectEntry{{path: "/x", projectID: 9}})
	id, ok := m.match([]string{"/x"})
	if !ok || id != 9 {
		t.Errorf("want (9, true), got (%d, %v)", id, ok)
	}
}

func TestPrefixMatcher_TieBreakSmallerIDWins(t *testing.T) {
	// Two paths of equal length, two candidates each matching one.
	// Old semantics (length desc, id asc) returned the smaller id.
	m := buildPrefixMatcher([]projectEntry{
		{path: "/a/b", projectID: 5},
		{path: "/c/d", projectID: 3},
	})
	id, ok := m.match([]string{"/a/b/x", "/c/d/y"})
	if !ok || id != 3 {
		t.Errorf("want (3, true), got (%d, %v)", id, ok)
	}
}

func TestPrefixMatcher_LongerWinsOverShorter(t *testing.T) {
	// Both /a and /a/b are prefixes of /a/b/c; longer wins.
	m := buildPrefixMatcher([]projectEntry{
		{path: "/a", projectID: 1},
		{path: "/a/b", projectID: 2},
	})
	id, ok := m.match([]string{"/a/b/c"})
	if !ok || id != 2 {
		t.Errorf("want (2, true), got (%d, %v)", id, ok)
	}
}

func TestPrefixMatcher_MultipleCandidatesPickLongest(t *testing.T) {
	// Two candidates, two different entries match; longer-path entry wins.
	m := buildPrefixMatcher([]projectEntry{
		{path: "/short", projectID: 1},
		{path: "/much/longer/prefix", projectID: 2},
	})
	id, ok := m.match([]string{"/short/file", "/much/longer/prefix/file"})
	if !ok || id != 2 {
		t.Errorf("want (2, true), got (%d, %v)", id, ok)
	}
}

func TestPrefixMatcher_EmptyMatcher(t *testing.T) {
	m := buildPrefixMatcher(nil)
	if _, ok := m.match([]string{"/anything"}); ok {
		t.Error("empty matcher should never match")
	}
}

func TestPrefixMatcher_NonPrefixCandidateDoesNotMatch(t *testing.T) {
	// "/a/b" is NOT a prefix of "/a/c/something" — there's a diverging byte.
	m := buildPrefixMatcher([]projectEntry{{path: "/a/b", projectID: 1}})
	if _, ok := m.match([]string{"/a/c/something"}); ok {
		t.Error("diverging path should not match")
	}
}

func TestPrefixMatcher_EmptyPathsSkipped(t *testing.T) {
	// Defensive: an entry with empty path mustn't make every candidate match.
	m := buildPrefixMatcher([]projectEntry{
		{path: "", projectID: 99},
		{path: "/a", projectID: 1},
	})
	if id, ok := m.match([]string{"/a/x"}); !ok || id != 1 {
		t.Errorf("want (1, true), got (%d, %v)", id, ok)
	}
	if _, ok := m.match([]string{"/elsewhere"}); ok {
		t.Error("empty-path entry leaked into matching")
	}
}

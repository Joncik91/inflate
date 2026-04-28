package harvester

import (
	"os"
	"path/filepath"
	"strings"
)

// PromoteToRepoRoot inspects the file collector's output. If the file
// paths cluster under a single subdirectory of cwd, and that
// subdirectory (or one of its ancestors below cwd) contains a .git
// entry, returns that promoted directory.
//
// Returns ok=false when:
//   - cwd itself is already a repo
//   - the file block has no extractable absolute paths
//   - paths span more than one immediate child of cwd (ambiguous)
//   - no .git is found between the cluster root and cwd
//
// The intent is to recover from launches in a parent dir like
// /home/u/apps when the actual project is /home/u/apps/inflate, by
// using the file walker's incidental discovery as a strong signal.
func PromoteToRepoRoot(cwd, fileBlock string) (string, bool) {
	cwdAbs, err := filepath.Abs(cwd)
	if err != nil {
		return "", false
	}
	if hasGitEntry(cwdAbs) {
		return "", false // cwd already a repo
	}

	paths := extractAbsolutePaths(fileBlock)
	if len(paths) == 0 {
		return "", false
	}

	// All paths must live under cwd. Anything outside is a paste-host
	// artifact (e.g. /tmp/...) — ignore it.
	var inside []string
	for _, p := range paths {
		if strings.HasPrefix(p, cwdAbs+string(os.PathSeparator)) {
			inside = append(inside, p)
		}
	}
	if len(inside) == 0 {
		return "", false
	}

	// Find the immediate-child of cwd that each path lives under.
	// If the paths span multiple immediate children, the parent is
	// ambiguous and we don't promote.
	rel := strings.TrimPrefix(inside[0], cwdAbs+string(os.PathSeparator))
	firstSegment := strings.SplitN(rel, string(os.PathSeparator), 2)[0]
	if firstSegment == "" {
		return "", false
	}
	for _, p := range inside[1:] {
		r := strings.TrimPrefix(p, cwdAbs+string(os.PathSeparator))
		seg := strings.SplitN(r, string(os.PathSeparator), 2)[0]
		if seg != firstSegment {
			return "", false // ambiguous — multiple immediate children
		}
	}

	// Walk upward from the immediate-child looking for .git, but never
	// above cwd. The cluster's repo root is somewhere between
	// cwd/<firstSegment> and the first ancestor with a .git.
	candidate := filepath.Join(cwdAbs, firstSegment)
	for {
		if hasGitEntry(candidate) {
			return candidate, true
		}
		parent := filepath.Dir(candidate)
		if parent == cwdAbs || parent == candidate {
			return "", false // reached cwd without finding .git
		}
		candidate = parent
	}
}

// extractAbsolutePaths pulls absolute filesystem paths out of the file
// collector's rendered block. The format is predictable (one path per
// line, possibly with a "label:" header line).
//
// Cross-platform: accepts both Unix-rooted paths (/foo/bar) AND
// Windows drive-letter paths (C:\foo or C:/foo). filepath.IsAbs alone
// is not enough — on Windows, IsAbs("/foo") returns false because
// /foo is "rooted but drive-relative", not "absolute". We accept both
// because either flavor is fine for the cluster-prefix logic below.
func extractAbsolutePaths(block string) []string {
	var out []string
	for _, raw := range strings.Split(block, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// Skip label lines like "open in editor:" or
		// "recently modified (no editor detected):".
		if strings.HasSuffix(line, ":") {
			continue
		}
		if isPathLike(line) {
			out = append(out, line)
		}
	}
	return out
}

// isPathLike accepts a line as a path if filepath.IsAbs is happy with
// it OR if it looks like a Unix absolute path (starts with /). The
// second clause matters on Windows where /foo/bar reads as "rooted on
// the current drive," which IsAbs declines to call absolute but our
// callers treat the same way.
func isPathLike(line string) bool {
	if filepath.IsAbs(line) {
		return true
	}
	if strings.HasPrefix(line, "/") {
		return true
	}
	return false
}

// hasGitEntry returns true if dir contains a .git directory or a
// .git file (worktree / submodule).
func hasGitEntry(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

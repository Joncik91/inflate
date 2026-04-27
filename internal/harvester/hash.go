package harvester

import "strings"

// ProjectDirName converts an absolute project path into the directory name
// Claude Code uses under ~/.claude/projects/. It replaces every "/" with "-".
func ProjectDirName(absPath string) string {
	return strings.ReplaceAll(absPath, "/", "-")
}

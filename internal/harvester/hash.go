package harvester

import "regexp"

// nonSafeRun matches any run of characters that Claude Code replaces with
// a single "-" when constructing the project directory name under
// ~/.claude/projects/. Empirically, at least "/" and " " trigger the
// replacement; we generalize to any non-[A-Za-z0-9-] character to be safe.
var nonSafeRun = regexp.MustCompile(`[^A-Za-z0-9-]+`)

// ProjectDirName converts an absolute project path into the directory name
// Claude Code uses under ~/.claude/projects/. Each run of one-or-more
// characters outside [A-Za-z0-9-] becomes a single "-". This matches the
// observed behavior for paths containing both "/" and " " (and assumes the
// same for other punctuation).
//
// Verified mappings (see TestProjectDirNameKnownMappings):
//
//	/                                              -> "-"
//	/home/joncik                                   -> "-home-joncik"
//	/home/joncik/apps/Codexbar-fork                -> "-home-joncik-apps-Codexbar-fork"
//	/home/joncik/Projects/starting from scratch    -> "-home-joncik-Projects-starting-from-scratch"
//
// Caller must pass an absolute, POSIX-style path. Behavior on relative
// paths or Windows-style paths is undefined.
func ProjectDirName(absPath string) string {
	return nonSafeRun.ReplaceAllString(absPath, "-")
}

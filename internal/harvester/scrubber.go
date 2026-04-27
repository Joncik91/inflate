package harvester

import "regexp"

var scrubPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`),
	regexp.MustCompile(`Bearer [A-Za-z0-9._\-]+`),
	regexp.MustCompile(`(?i)password=\S+`),
	regexp.MustCompile(`(?i)token=\S+`),
}

// Scrub replaces each match of a known secret pattern with [REDACTED] and
// returns the cleaned string plus the number of substitutions performed.
func Scrub(in string) (string, int) {
	out := in
	hits := 0
	for _, re := range scrubPatterns {
		out = re.ReplaceAllStringFunc(out, func(_ string) string {
			hits++
			return "[REDACTED]"
		})
	}
	return out, hits
}

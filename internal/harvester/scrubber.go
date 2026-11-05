package harvester

import (
	"regexp"
	"sort"
)

// redactedToken is the placeholder written in place of every redacted
// span. Detectors compare candidate values against it (see scanner.go) so
// that Scrub(Scrub(x)) never finds a fresh "secret" to redact in an
// already-redacted value, keeping the whole pipeline idempotent.
const redactedToken = "[REDACTED]"

// scrubPatterns matches vendor-specific credential shapes precisely enough
// that a plain regexp is sufficient: they have a fixed, recognizable
// prefix and character alphabet, so there is no runtime-chosen delimiter
// or escape sequence for RE2's lack of backreferences/lookahead to trip
// over. Everything else (generic key=value assignments, PEM blocks) is
// handled by the hand-written scanner in scanner.go, which does need that
// extra power.
//
// A note on left anchoring, because it is the difference between these
// patterns working and leaking. A leading `\b` asserts a word/non-word
// transition, so it does NOT mean "the key starts here" -- it means "the
// character before the key is not a word character." When a key is jammed
// directly against a word character ("Xsk-ant-api03-...", "7ghp_...",
// "_npm_...") the boundary simply does not exist and the pattern does not
// match at all: the credential ships in the clear while hits reports 0.
// That shape is not exotic. This scrubber reads shell history, git state,
// editor buffers and Claude Code JSONL transcripts, where keys routinely
// abut a word character: concatenated env dumps ("FOO=barsk-ant-..."),
// base64-ish blobs, log lines written without separators, and flattened
// JSON. (The AWS pattern below was never anchored and never had the bug,
// which is what isolated the anchor as the cause.)
//
// The anchor is therefore dropped wherever it is not load-bearing, which
// is decided per pattern rather than uniformly, because the cost of
// dropping it is not uniform. For a long, distinctive prefix
// ("github_pat_", "AIzaSy", "eyJ") no English word ends in the prefix, so
// a preceding word character is noise, not a word collision, and the
// anchor only ever suppressed true positives. For the short prefix "sk-",
// which really does collide with English ("whisk-", "kiosk-mode",
// "risk-management", "task-list", "disk-usage"), the anchor stays and a
// separate unanchored branch handles the jammed case -- see below.
var scrubPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	// sk-<key material>: left word boundary so "whisk-", "kiosk-mode",
	// "risk-management", "task-list", "disk-usage" never match. Real vendor
	// keys (sk-ant-api03-..., sk-proj-..., sk-or-v1-...) embed single
	// hyphens between alnum segments, so the alphabet must allow '-' --
	// but only ever a single hyphen at a time. That's enforced by
	// structure (an alnum run, optionally followed by more single-hyphen-
	// joined alnum runs) rather than a bare "any hyphen" class, so the
	// match can never run into a PEM marker's run of dashes. Two
	// alternatives cover the legacy flat key (sk-<20+ alnum>, no hyphens)
	// and the multi-segment vendor shape (at least one hyphen-joined
	// segment, with a floor on the trailing segment's length).
	//
	// This is the one pattern whose `\b` is genuinely load-bearing, so it
	// is kept exactly as-is and the jammed case is covered additively by
	// skJammedPattern below.
	regexp.MustCompile(`\bsk-(?:[a-zA-Z0-9]{20,}|[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)+[a-zA-Z0-9]{4,})`),
	// The remaining vendor prefixes are long and distinctive enough that
	// the left anchor cost more than it bought: no English word ends in
	// "gh[poasur]_", "github_pat_", "xox[baps]-", "xapp-", "AIzaSy" or
	// "npm_", so there is no word-collision to protect against, and each
	// still carries its own length floor on the key material.
	regexp.MustCompile(`gh[poasur]_[a-zA-Z0-9]{20,}`),
	regexp.MustCompile(`github_pat_[a-zA-Z0-9_]{20,}`),
	regexp.MustCompile(`xox[baps]-[a-zA-Z0-9-]{10,}`),
	regexp.MustCompile(`xapp-[a-zA-Z0-9-]{10,}`),
	regexp.MustCompile(`AIzaSy[a-zA-Z0-9_-]{20,}`),
	regexp.MustCompile(`npm_[a-zA-Z0-9]{20,}`),
	regexp.MustCompile(`Bearer [A-Za-z0-9._\-]+`),
	regexp.MustCompile(`Authorization:\s*Basic\s+[A-Za-z0-9+/=]+`),
	// scheme://user:pass@host -- capture only the credential portion so
	// the scheme and host survive.
	regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.\-]*://[^\s/:@]+:[^\s/@]+@`),
	// Bare JWT: three dot-separated base64url segments, each long enough
	// to not be an ordinary dotted identifier or version string. Both
	// anchors are dropped: the leading one for the reason above (a JWT
	// jammed against a word char is still a JWT), and the trailing one
	// because it is the mirror image of the same defect -- a token
	// followed immediately by a word character would otherwise not match
	// either. The three length floors, not the anchors, are what keep an
	// ordinary dotted identifier or version string from matching.
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`),
}

// skJammedPattern covers the one case the anchored sk- pattern above
// structurally cannot: a key jammed directly against a word character,
// where the `\b` that protects "risk-management" also suppresses
// "Xsk-ant-api03-...".
//
// Simply deleting that `\b` is not an option, and this was measured rather
// than assumed: an unanchored version of the anchored pattern matches
// "risk-management-framework-review", "task-list-item-identifier-value",
// "asterisk-based-telephony-server" and similar ordinary hyphenated prose,
// because the anchored pattern's second alternative is satisfied by any
// run of short hyphen-joined segments.
//
// So the discriminator here is key material rather than position: every
// real sk- key contains an unbroken run of at least 20 alnum characters,
// while English hyphenated compounds are built from short segments. The
// run is reached through at most 8 single-hyphen-joined segments, which
// covers every published vendor shape (sk-ant-api03-..., sk-proj-...,
// sk-or-v1-...) while keeping the "one hyphen at a time" structure that
// stops a match from ever bridging a PEM marker's run of dashes -- a
// permissive `[a-zA-Z0-9-]` class was tried and did bridge them
// ("task-------...----AAAA..." matched), which is why it is not used.
//
// This is additive: it only ever contributes spans the anchored pattern
// did not, and mergeSpans collapses the overlap where both fire, so hits
// stays equal to the number of [REDACTED] tokens emitted.
var skJammedPattern = regexp.MustCompile(`sk-(?:[a-zA-Z0-9]+-){0,8}?[a-zA-Z0-9]{20,}`)

// Scrub replaces every recognized credential in in with [REDACTED] and
// returns the cleaned string plus the number of substitutions Scrub itself
// performed. hits equals the number of spans this call redacted -- every
// detector below reports a byte span, spans are merged and de-duplicated
// once, and exactly one [REDACTED] is written per surviving span -- so it
// is always safe to add hits across repeated calls to get a running total
// of redactions actually made.
//
// hits is NOT a count of every "[REDACTED]" substring present in the
// returned string: if in already contains the literal token "[REDACTED]"
// (e.g. text that quotes a prior tool's own redacted output), that
// occurrence passes through untouched -- correctly, since it is not a
// live secret -- and is not counted, because Scrub did not put it there.
// strings.Count(out, "[REDACTED]") can therefore legitimately exceed hits;
// that is not a leak, and hits is idempotent under Scrub(Scrub(x)) is 0 as
// designed. Comparing hits to strings.Count only makes sense in tests
// where the input is known not to already contain the literal token.
func Scrub(in string) (string, int) {
	var spans []span

	spans = append(spans, scanPEMBlocks(in)...)
	spans = append(spans, scanCredentialAssignments(in)...)
	for _, re := range scrubPatterns {
		for _, loc := range re.FindAllStringIndex(in, -1) {
			spans = append(spans, span{Start: loc[0], End: loc[1]})
		}
	}
	for _, loc := range skJammedPattern.FindAllStringIndex(in, -1) {
		spans = append(spans, span{Start: loc[0], End: loc[1]})
	}

	spans = mergeSpans(spans)
	if len(spans) == 0 {
		return in, 0
	}

	var out []byte
	pos := 0
	for _, sp := range spans {
		out = append(out, in[pos:sp.Start]...)
		out = append(out, redactedToken...)
		pos = sp.End
	}
	out = append(out, in[pos:]...)
	return string(out), len(spans)
}

// mergeSpans sorts spans by start and collapses any that overlap or touch
// into a single span, so a byte range claimed by more than one detector
// (e.g. a PEM block whose body also happens to satisfy a vendor regex)
// still produces exactly one [REDACTED] token -- keeping hits equal to the
// number of tokens actually emitted.
func mergeSpans(spans []span) []span {
	if len(spans) == 0 {
		return nil
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].End < spans[j].End
	})
	merged := make([]span, 0, len(spans))
	cur := spans[0]
	for _, sp := range spans[1:] {
		if sp.Start <= cur.End {
			if sp.End > cur.End {
				cur.End = sp.End
			}
			continue
		}
		merged = append(merged, cur)
		cur = sp
	}
	merged = append(merged, cur)
	return merged
}

package harvester

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// --- Differential fuzzer: oracle and deterministic cross product ------------
//
// checkScrubCase is the whole oracle. Everything else in this file is
// generation. The properties asserted here are the ones that are free once
// the generator knows what secret it planted:
//
//  1. LEAK (the primary property): the known secret must not appear in the
//     output. This is a definite bug when violated -- no judgement call, no
//     second implementation to differ against.
//  2. HITS ACCOUNTING: hits must equal the number of [REDACTED] tokens
//     actually emitted, checked only when the input provably contains no
//     literal "[REDACTED]" of its own (Scrub documents that a pre-existing
//     token passes through uncounted, so the comparison is only meaningful
//     on inputs that have none).
//  3. IDEMPOTENCE: Scrub(Scrub(x)) must report 0 hits and return a
//     byte-identical string. A second pass that finds something means the
//     first pass's own output is being re-detected as a secret.
//  4. JSON PRESERVATION: if the input parsed as JSON, the output must too.
//     Redaction that corrupts structure breaks every downstream consumer.

// scrubCase is one generated input plus the secret it is known to contain.
type scrubCase struct {
	Name    string
	Input   string
	Secret  string
	Shape   string
	Context string
	// ExpectRedaction lets a generator declare that non-detection is
	// acceptable for this case. Every generator currently sets it true; the
	// one live policy exemption ("prose after a colon-assigned value") is
	// recognized structurally from the input by isProsePolicyShape instead,
	// so that the fuzz target -- whose mutator can build that shape out of
	// affixes no context chose -- applies the identical rule. The field is
	// kept for a future exemption that genuinely cannot be decided from the
	// input alone, and it suppresses only the LEAK property: the
	// hits/idempotence/JSON properties must hold whether or not the value
	// was redacted.
	ExpectRedaction bool
}

// scrubFailure records one oracle violation in enough detail to reproduce
// it by hand.
type scrubFailure struct {
	Case    scrubCase
	Reason  string
	Output  string
	Hits    int
	Surface string
}

// checkScrubCase runs the oracle against one generated case and returns
// every property it violated. It returns failures rather than calling
// t.Error directly so the fuzz target and the table test can share it and
// so the table test can aggregate and de-duplicate a large number of
// related violations into one readable report.
func checkScrubCase(c scrubCase) []scrubFailure {
	var out []scrubFailure
	scrubbed, hits := Scrub(c.Input)

	expectRedaction := c.ExpectRedaction &&
		!isProsePolicyShape(c.Input, c.Secret) &&
		!isFunctionCallShape(c.Input, c.Secret)

	// 1. LEAK (whole secret).
	if expectRedaction && c.Secret != "" && strings.Contains(scrubbed, c.Secret) {
		out = append(out, scrubFailure{
			Case: c, Reason: "secret survived", Output: scrubbed, Hits: hits,
			Surface: c.Secret,
		})
	} else if expectRedaction {
		// 1b. LEAK (partial). Whole-string containment alone is not enough:
		// the worst failure this scrubber has shipped is "secret cut in
		// half, success reported", where a span is truncated at some
		// ceiling and the TAIL of the credential survives while hits still
		// reports 1. Containment of the full secret misses that entirely,
		// because the full secret is indeed gone -- most of it.
		//
		// So a surviving contiguous fragment counts too, above a length
		// floor that keeps ordinary coincidence out: at
		// leakFragmentMinLen bytes drawn from a 62-character alphabet, an
		// accidental match in surrounding filler is not a realistic
		// concern, while any real truncation of a credential leaves far
		// more than that behind.
		if frag, leaked := survivingFragment(scrubbed, c.Secret); leaked {
			out = append(out, scrubFailure{
				Case: c, Reason: "partial secret survived", Output: scrubbed, Hits: hits,
				Surface: frag,
			})
		}
	}

	// 2. HITS ACCOUNTING.
	if !strings.Contains(c.Input, redactedToken) {
		if n := strings.Count(scrubbed, redactedToken); n != hits {
			out = append(out, scrubFailure{
				Case: c, Reason: "hits does not match emitted [REDACTED] count",
				Output: scrubbed, Hits: hits,
			})
		}
	}

	// 3. IDEMPOTENCE.
	again, hits2 := Scrub(scrubbed)
	if hits2 != 0 {
		out = append(out, scrubFailure{
			Case: c, Reason: "second Scrub pass reported nonzero hits",
			Output: scrubbed, Hits: hits2,
		})
	}
	if again != scrubbed {
		out = append(out, scrubFailure{
			Case: c, Reason: "Scrub is not byte-idempotent",
			Output: scrubbed, Hits: hits2, Surface: again,
		})
	}

	// 4. JSON PRESERVATION.
	if json.Valid([]byte(c.Input)) && !json.Valid([]byte(scrubbed)) {
		out = append(out, scrubFailure{
			Case: c, Reason: "valid JSON input produced invalid JSON output",
			Output: scrubbed, Hits: hits,
		})
	}

	return out
}

// isProsePolicyShape reports whether in ends the credential value with the
// one trailing shape scanner.go rejects ON PURPOSE: a colon-assigned,
// unquoted value followed across a single space by an ordinary word, which
// is prose ("Secret: this document is confidential") rather than an
// assignment. Non-detection there is a documented policy decision, not a
// leak.
//
// It is applied here, structurally, rather than only as a per-context flag
// on the generated case, because the fuzz target's mutator can create the
// shape dynamically by appending " word" as a suffix to a context that did
// not have it. Deciding from the input keeps the table test and the fuzz
// target on exactly the same rule -- a flag-only exemption let the fuzzer
// report the policy as a bug.
//
// The rule is kept as narrow as the policy it mirrors: colon operator only,
// value unquoted, exactly space (never tab) between the value and what
// follows, and what follows must not be one of the bytes scanner.go accepts
// as a terminator. Those accepted bytes -- the structural openers and
// closers & | ; , } ) ] { [ ( and the comment introducer # -- are listed
// here explicitly rather than as "must be a letter", because the policy is
// defined by the accepted set, not by the rejected one: a following token
// starting with a DIGIT ("password: SECRET 0") is rejected by scanner.go
// exactly as a word is, and a letters-only mirror of the rule reported that
// as a leak.
func isProsePolicyShape(in, secret string) bool {
	if secret == "" {
		return false
	}
	i := strings.Index(in, secret)
	if i < 0 {
		return false
	}
	// The operator must be ':' and the value must not be quoted or
	// bracketed: walk back over horizontal space from the secret's start.
	p := i
	for p > 0 && isHSpace(in[p-1]) {
		p--
	}
	if p == 0 || in[p-1] != ':' {
		return false
	}
	// Everything from the secret to the end of the value must be bare, and
	// what follows must be horizontal space (no tab) then a letter.
	j := bareValueEnd(in, i+len(secret))
	sawSpace := false
	for j < len(in) && in[j] == ' ' {
		sawSpace = true
		j++
	}
	if !sawSpace || j >= len(in) {
		return false
	}
	if _, isLine := isLineSep(in, j); isLine {
		return false
	}
	switch in[j] {
	case '&', '|', ';', ',', '}', ')', ']', '{', '[', '(', '#':
		// An accepted terminator: not prose, still expected to redact.
		return false
	}
	return true
}

// isFunctionCallShape reports whether the credential value is immediately
// followed by '(', which scanner.go treats as a function call rather than a
// literal secret ("password=getToken()") and declines to redact on purpose.
// Like isProsePolicyShape this is recognized structurally, because the fuzz
// mutator can append '(' as a suffix to any context.
func isFunctionCallShape(in, secret string) bool {
	if secret == "" {
		return false
	}
	i := strings.Index(in, secret)
	if i < 0 {
		return false
	}
	j := bareValueEnd(in, i+len(secret))
	return j < len(in) && in[j] == '('
}

// bareValueEnd returns the index at which a bare (unquoted) value starting
// at or before i stops, following scanner.go's rule that a backslash
// escapes the next byte -- so a backslash-escaped space stays INSIDE the
// value rather than terminating it. Both policy helpers use this one
// definition; an earlier version stopped naively at the first
// non-bare-value byte, missed the escape, and reported
// `PASSWORD=SECRET\ (` (whose value really does run through the escaped
// space to the '(' and is then declined as a function call) as a leak.
func bareValueEnd(in string, i int) int {
	for i < len(in) {
		if in[i] == '\\' {
			// A trailing backslash with nothing after it must not step past
			// the end; clamp so callers can index in[j] safely.
			if i+2 > len(in) {
				return len(in)
			}
			i += 2
			continue
		}
		if !isBareValueByte(in[i]) {
			return i
		}
		i++
	}
	return len(in)
}

// leakFragmentMinLen is the shortest contiguous run of a known secret whose
// survival is reported as a partial leak. Fragments below it are not
// reported: a short run can plausibly appear in unrelated text, and the
// point of the floor is that a report from this fuzzer is never a
// judgement call. Well above the floor is where every real truncation
// lands, since a truncated credential leaves tens of bytes behind, not
// twelve.
const leakFragmentMinLen = 12

// survivingFragment returns the longest surviving PREFIX or SUFFIX of
// secret (at least leakFragmentMinLen bytes) still present in out, and
// whether one was found.
//
// Only prefixes and suffixes are checked, not every internal substring.
// That is not a shortcut for speed alone -- it is the shape of the defect.
// A truncated redaction span always cuts the credential at one end: the
// span starts at the credential and stops early (suffix survives), or it
// starts late and the head survives (prefix). An interior run surviving
// while both ends are redacted would require two disjoint spans inside one
// credential, which no detector here produces. Checking all O(n^2)
// substrings would cost far more on 80,000 cases and find nothing extra.
func survivingFragment(out, secret string) (string, bool) {
	if len(secret) < leakFragmentMinLen {
		return "", false
	}
	for n := len(secret) - 1; n >= leakFragmentMinLen; n-- {
		if suffix := secret[len(secret)-n:]; strings.Contains(out, suffix) {
			return suffix, true
		}
		if prefix := secret[:n]; strings.Contains(out, prefix) {
			return prefix, true
		}
	}
	return "", false
}

// reportFailures prints every failure in a stable, greppable form. Each
// line carries the Go-quoted input so the case can be pasted straight into
// a one-off reproduction test.
//
// Failures matching an entry in openFindings are downgraded to t.Logf: they
// are already-reported, not-yet-fixed defects, kept visible in the test log
// without turning the CI gate permanently red. TestScrubFuzz_OpenFindings
// separately asserts each of them still reproduces, so an entry cannot go
// stale after a fix without the suite noticing.
func reportFailures(t *testing.T, failures []scrubFailure) {
	t.Helper()
	for _, f := range failures {
		msg := fmt.Sprintf("LEAK[%s] shape=%s context=%s\n  reason:  %s\n  input:   %q\n  output:  %q\n  hits:    %d\n  secret:  %q",
			f.Case.Name, f.Case.Shape, f.Case.Context, f.Reason, f.Case.Input, f.Output, f.Hits, f.Surface)
		if id, known := matchOpenFinding(f); known {
			t.Logf("KNOWN-OPEN[%s] %s", id, msg)
			continue
		}
		t.Error(msg)
	}
}

// openFinding describes a defect this fuzzer found, minimized to a
// standalone reproduction, and which is deliberately NOT fixed here (fixing
// the scrubber is out of scope for a test-only tool). Each entry carries
// the minimal input that triggers it and a Match predicate identifying the
// generated cases that hit the same defect, so the cross-product gate can
// separate "already reported" from "new regression".
type openFinding struct {
	ID    string
	Desc  string
	Input string
	// WantLeak is the substring that survives in the minimal repro's output
	// (empty when the defect is structural rather than a survival, e.g. the
	// JSON-corruption and idempotence findings).
	WantLeak string
	// ReproShape and ReproContext name the generated shape/context that
	// Input was minimized FROM. Several Match predicates key off those
	// labels (a defect that is genuinely specific to, say, PEM shapes is
	// most honestly identified by the shape), so
	// TestScrubFuzz_OpenFindingsAreDistinct needs them to reconstruct the
	// failure its own repro would have produced. Empty means the predicate
	// does not look at the labels at all.
	ReproShape   string
	ReproContext string
	Match        func(scrubFailure) bool
}

// openFindings is the registry of defects found by this fuzzer against the
// current scrubber and not fixed here. Each is reproduced independently by
// TestScrubFuzz_OpenFindings, so if one is fixed the suite fails with
// "no longer reproduces" and the entry can be deleted -- an entry can never
// silently outlive the bug it documents.
var openFindings = []openFinding{
	{
		ID:           "colon-value-quote-ambiguous-with-unterminated-enclosing-string",
		Desc:         "FIXED (this session): colon-keyed assignments are now trusted inside an already-open double/backtick-quoted string, closing the two blocking under-redaction classes this entry used to sit alongside (a colon-keyed credential invisible inside a balanced-quoted shell/log string, e.g. `curl -H \"X-Api-Key: SECRET\" ...`; and inside backtick/markdown-fenced code). That fix surfaces one narrow, genuinely new residual: when the credential's value itself looks like it OPENS a nested quoted string (`\"password: \"SECRET`), and the OUTER enclosing string is never itself terminated by a third quote anywhere in the document, the scanner cannot tell whether that second quote is the value's opening delimiter or the outer string's own (missing) closing delimiter. It resolves that ambiguity toward NOT over-redacting (declining to treat the second quote as a value opener, per the same 'no second occurrence found, so this must be the enclosing string's own closer' rule that already protects `{\"note\":\"see api_key=\"}` from having its closing brace consumed) -- but that means the credential value that follows is left with no key context at all once the quote closes, and ships unredacted, hits=0. Narrow because it requires ALL of: colon operator, an unquoted-looking value that itself starts with the SAME quote character enclosing the whole string, and no third occurrence of that quote character anywhere later in the document to prove which quote was which. Left open rather than guessed at: resolving the ambiguity the other way (assume nested value) reopens the EOF-consuming-brace defect `unmatched-value-quote-in-json-string-runs-span-to-eof` already exists to prevent.",
		Input:        `"password: "SEKRITVALUE123`,
		ReproShape:   "assign-password",
		ReproContext: "assign/colon-space+unquoted+eof",
		WantLeak:     "SEKRITVALUE123",
		// Matched on the CONDITION, not the context name: the fuzz target's
		// mutator (and the deterministic table, via other leadin/context
		// combinations) can reach this same ambiguity from any colon-keyed
		// assignment whose bare value happens to start with a quote
		// character that also encloses the whole document.
		Match: func(f scrubFailure) bool {
			return f.Reason == "secret survived" &&
				hasUnterminatedEnclosingQuoteWithValueOpeningSameQuote(f.Case.Input, f.Case.Secret)
		},
	},
	{
		ID:           "truncated-pem-multiple-non-body-lines-no-body-line-ever-found",
		Desc:         "A truncated PEM block (no END marker anywhere) whose lines never contain even one that passes pemBodyLineOK -- e.g. a word-wrapped paste where EVERY line has a stray space, and the final line also carries trailing punctuation -- is redacted only through the single line directly after BEGIN, while every following non-body line (real, if garbled, key material) ships in the clear. This is a variant of the single-non-body-line truncated-PEM defect (fixed: a lone non-body line directly after BEGIN, with nothing after it, is now covered), one non-body line longer: once a SECOND non-body line follows without any body-OK line ever appearing in between, the span still stops at the first one. The safe general fix (extend through an unbounded run of non-body lines whenever no body-OK line is ever found) was rejected because it cannot be distinguished from the deliberately-bounded 'stray BEGIN marker followed by ordinary prose' case, which must NOT have its 200 lines swept in wholesale -- both shapes look identical to pemBodyLineOK (every line fails it). Left open rather than papered over with a heuristic that risks that bound.",
		Input:        "-----BEGIN RSA PRIVATE KEY-----\njiDJswf8tptOKlTr4DSIn7Pai2VC0h Uu9ltzb/fDpRUBxq3EArDeZpefnQma\nF62Cv2YDSdlUfcPp1FuNfBj4==,",
		ReproShape:   "pem-truncated-wrapped-paste-with-space",
		ReproContext: "adjacency/bare+comma",
		WantLeak:     "F62Cv2YDSdlUfcPp1FuNfBj4==",
		// Matched on the CONDITION, not the shape name: the fuzz target's
		// mutator can produce this exact "truncated PEM, no END, every
		// trailing line non-body-shaped" structure from affixes wrapped
		// around any PEM shape, not only pem-truncated-wrapped-paste-with-
		// space.
		Match: func(f scrubFailure) bool {
			return f.Reason == "secret survived" &&
				hasTruncatedPEMWithMultipleTrailingNonBodyLines(f.Case.Input)
		},
	},
	{
		ID:           "pem-in-json-string-consumes-closing-delimiters",
		Desc:         "A PEM block embedded in a JSON string value ({\"private_key\":\"-----BEGIN ...\"}) has its span extended past the JSON string's closing quote and the object's closing brace, so the redaction destroys the document structure: valid JSON in, invalid JSON out, with everything after the block removed.",
		Input:        `{"private_key":"-----BEGIN RSA PRIVATE KEY-----\nAAAABBBBCCCC\n-----END RSA PRIVATE KEY-----"}`,
		ReproShape:   "pem-rsa-private-key-short-base64",
		ReproContext: "doc/pem-in-json-string",
		WantLeak:     "",
		Match: func(f scrubFailure) bool {
			return f.Reason == "valid JSON input produced invalid JSON output" &&
				strings.HasPrefix(f.Case.Shape, "pem")
		},
	},
	{
		ID: "unmatched-value-quote-in-json-string-runs-span-to-eof",
		Desc: "A quoted-value scan inside a JSON string that finds no matching close runs its span all the way to EOF, taking the JSON string's own closing quote and the object's closing brace with it: valid JSON in, truncated garbage out, hits=1. Two triggers observed, deliberately recorded as ONE finding because minimizing them lands on the same mechanism (an opening quote with no partner, span extended to EOF), not two:\n" +
			"  (a) a shell-style single-quoted value in free text: {\"note\":\"password='SECRET\"} -- every byte from the value onward is deleted;\n" +
			"  (b) no value at all, where the credential key's '=' is the last content byte and the STRING'S OWN closing quote is read as the value's opening quote: {\"note\":\"see api_key=\"} -- the closing quote and brace are replaced by a placeholder even though there is no secret in the input whatsoever, so a document is corrupted to redact nothing.\n" +
			"An earlier draft filed these as two entries; the matchers could only be kept apart by increasingly baroque output-shape tests, which was the signal that they were one defect seen from two angles.",
		Input:        `{"note":"password='SEKRITVALUE"}`,
		ReproShape:   "assign-password",
		ReproContext: "json/single-quoted-value-inside-string",
		WantLeak:     "",
		// Matched on the CONDITION rather than the context name, since the
		// fuzz target reaches this from the assign/* contexts too by
		// wrapping them in a JSON string via mutator-supplied affixes. Both
		// triggers share one observable signature: the output is a strict
		// PREFIX of the input plus a trailing placeholder, i.e. a span that
		// began somewhere and consumed everything to EOF.
		Match: func(f scrubFailure) bool {
			if f.Reason != "valid JSON input produced invalid JSON output" {
				return false
			}
			// The signature is a span that ran to EOF: the LAST placeholder
			// in the output has nothing after it except bytes the fuzz
			// mutator appended outside the document, and the JSON's own
			// closing delimiters (a quote and a brace or bracket) are gone
			// from the tail.
			//
			// Located from the LAST placeholder rather than the first, and
			// without requiring the output to end with it: earlier
			// placeholders can come from unrelated redactions in the same
			// document, and the mutator's suffix (a stray "\r", a digit)
			// sits after the runaway one. Both of those shapes were
			// reported as new findings by narrower versions of this test
			// before being recognized as this same defect.
			i := strings.LastIndex(f.Output, redactedToken)
			if i < 0 {
				return false
			}
			tail := f.Output[i+len(redactedToken):]
			return !strings.ContainsAny(tail, `"}]`) &&
				strings.ContainsAny(f.Case.Input, `"}]`)
		},
	},
	{
		ID:           "triply-escaped-json-invisible",
		Desc:         "A credential nested two escaping levels deep ({\"content\":\"{\\\"content\\\":\\\"{\\\\\\\"password\\\\\\\":...\"}) is not detected at all, hits=0. The doubly-escaped (single-nesting) transcript shape IS handled, so the escaped-quote key recognizer covers exactly one level.",
		Input:        `{"content":"{\"content\":\"{\\\"password\\\":\\\"SEKRITVALUE\\\"}\"}"}`,
		ReproShape:   "assign-password",
		ReproContext: "json/triply-escaped",
		WantLeak:     "SEKRITVALUE",
		Match: func(f scrubFailure) bool {
			return f.Reason == "secret survived" && f.Case.Context == "json/triply-escaped"
		},
	},
	{
		ID:           "chained-colon-label-genuinely-ambiguous-with-prose",
		Desc:         "A credential assignment whose immediately preceding non-space byte is an assignment operator, AND whose own operator is ':', is rejected outright, hits=0: `config: token: SEKRITVALUE123`, `--env=password: SEKRITVALUE123`. This is the residual half of the former, broader 'chained-label-rule-drops-nested-assignment' entry: the '='-final-operator half of that entry (`--env=PGPASSWORD=x`, `config: password=x`) IS fixed (tryKeyAt's chained-label check now only rejects when the CURRENT key's own operator is ':', since '=' is not credible sentence punctuation the way ':' is), but this ':'-final-operator half is genuinely ambiguous and deliberately left rejected: `config: token: SEKRITVALUE123` is structurally IDENTICAL to `Error: token: invalid` (word: word: word), which TestScrub_ProseNotRedacted requires to stay unredacted. There is no syntactic signal in either input that tells a real secret value apart from an ordinary trailing word -- both are \"word: word: word\" -- so resolving this without a semantic judgement call would require either leaking prose-labelled sentences or dropping real secrets in the other shape, and under-redaction is what this scrubber is measured on. Left open rather than guessed at.",
		Input:        "config: token: SEKRITVALUE123",
		ReproShape:   "assign-token",
		ReproContext: "leadin/outer-colon+colon",
		WantLeak:     "SEKRITVALUE123",
		// Matched on the CONDITION rather than the context name: the fuzz
		// target's mutator can place a chained ':'-prefixed label in front of
		// any context, not just the leadin/outer-colon* and leadin/flag-eq*
		// ones the deterministic table builds on purpose.
		Match: func(f scrubFailure) bool {
			return f.Reason == "secret survived" && hasColonFinalChainedLabel(f.Case.Input)
		},
	},
	// NOTE: an earlier draft of this registry carried an entry named
	// "colon-value-followed-by-hash-or-slash-comment-without-space",
	// reported from the input "password: SEKRITVALUE123# production". It
	// was WRONG and has been removed rather than kept for appearances.
	// Minimizing it properly showed that "password: SECRET#comment" and
	// "password: SECRET//note" are both redacted correctly (hits=1): the
	// '#' is simply consumed into the bare value, which over-redacts but
	// does not leak. The original repro leaked only because of the trailing
	// " production" -- an ordinary word after a space, which is the
	// documented prose policy, not a defect. The lesson is that a
	// non-minimal repro can attribute a leak to the wrong feature of the
	// input, and the fix is to minimize before classifying.
	{
		ID: "space-separated-scheme-value-truncated-at-the-space",
		Desc: "An assignment whose value carries a space-separated scheme prefix (authorization=Basic <base64>, api_key=Bearer <token>) had only the scheme word redacted: the bare-value scan stopped at the first space and the credential material after it survived while hits reported 1. FIXED for the primary shape (the scheme word plus credential, followed only by EOF/line-end/a structural terminator/exactly one trailing word): bareValueSchemeSpan now recognizes the one interior space between a known HTTP auth scheme word (Basic, Bearer, Digest, Negotiate, NTLM) and its credential as part of the value rather than a terminator.\n" +
			"A narrower residual remains open on the ':'-operator path only: when TWO OR MORE ordinary words (not a recognized structural terminator) follow the credential -- e.g. a trailing '// comment' after 'authorization: Basic <base64>' -- the colon trailing-context check's multi-word prose defense rejects the whole match, same as it correctly does for actual prose. That defense is deliberately not relaxed further here (see isSingleTrailingWord's doc comment and the chained-colon-label-genuinely-ambiguous-with-prose entry above for the same class of tradeoff): telling a genuine multi-word trailing comment apart from multi-word prose has no more of a syntactic signal than the chained-label case does.",
		Input:        "authorization: Basic QjqwYRj15ziREUfdImWQtjdAnAlqklCV== // production credential",
		ReproShape:   "http-basic",
		ReproContext: "assign/colon+unquoted+space-slashslash-comment",
		WantLeak:     "QjqwYRj15ziREUfdImWQtjdAnAlqklCV==",
		// Matched on the CONDITION, not the shape name: keying on
		// f.Case.Shape == "http-basic" || "http-bearer" claimed ANY leak
		// carrying those labels regardless of root cause, which
		// misattributed the json/triply-escaped context's leak for these
		// same two shapes to this entry (slice order put this entry first).
		// hasColonSchemeValueWithMultiWordTrailer instead recognizes the
		// actual structural trigger described above: a colon-operator
		// assignment whose bare value is "Scheme <credential>" followed by
		// two or more ordinary trailing words.
		Match: func(f scrubFailure) bool {
			return f.Reason == "secret survived" &&
				hasColonSchemeValueWithMultiWordTrailer(f.Case.Input, f.Case.Secret)
		},
	},
}

// leakIsCausedByStrayDoubleQuote reports whether the leak in c is caused by
// an unbalanced DOUBLE quote somewhere in the input rather than by the shape
// of the credential itself. It answers that DIFFERENTIALLY, the same way the
// now-fixed single-quote sibling check did: delete the leading run of bytes
// up to and including the FIRST occurrence of a double quote and re-run
// Scrub. If that deletion makes the secret disappear, the leak was caused by
// that stray double quote putting the scanner's state out of phase.
//
// Deliberately restricted to '"' only (unlike the retired
// leakIsCausedByStrayQuote, which tried every quote character): the single
// quote is fixed now, and a predicate that still tried '\” here would
// reclassify the fixed apostrophe cases as this (still-open) finding instead
// of letting them pass clean.
//
// The differential deletion test above is necessary but was NOT sufficient:
// deleting through the FIRST double quote in a genuinely BALANCED-quote input
// (e.g. `echo "password: SEKRIT"`, `curl -H "X-Api-Key: SEKRIT" https://x`)
// also makes the secret redact, because that deletion happens to strip the
// OPENING quote of the very string the secret lives in, leaving a dangling
// unescaped `"` at the end that the scanner reads as a fresh top-level quote
// open-and-immediately-close -- not because any quote was ever actually
// unbalanced in the original input. That false positive is what let every
// balanced-quote colon leak in this file mis-attribute to this entry. The
// doubleQuotesAreUnbalanced precondition below rules that out: it is only
// true when the ORIGINAL input's own double quotes, tracked exactly the way
// scanCredentialAssignments tracks them (escapes honored only once inside an
// open quote, since a backslash has no special meaning at top level), never
// close -- i.e. the scanner is genuinely left "inside a string" at EOF. Both
// registered repro inputs for this finding satisfy that; the balanced-quote
// cases above do not.
func leakIsCausedByStrayDoubleQuote(in, secret string) bool {
	if secret == "" || !doubleQuotesAreUnbalanced(in) {
		return false
	}
	for i := 0; i < len(in); i++ {
		if in[i] != '"' {
			continue
		}
		trimmed := in[i+1:]
		if !strings.Contains(trimmed, secret) {
			// The deletion removed the secret itself; that tells us nothing.
			continue
		}
		if out, _ := Scrub(trimmed); !strings.Contains(out, secret) {
			return true
		}
	}
	return false
}

// doubleQuotesAreUnbalanced reports whether in's double quotes are genuinely
// unbalanced: tracking quote state the same way scanCredentialAssignments
// does (a backslash only escapes the next byte while already inside an open
// quote; outside a quote it is an ordinary byte), the scanner is still
// "inside" an open double quote once it reaches EOF. This intentionally
// tracks '"' only (not every quote character), mirroring the scope of the
// finding it gates: a single quote or backtick left open has no bearing on
// whether colon-keyed detection was disabled by a stray DOUBLE quote.
func doubleQuotesAreUnbalanced(in string) bool {
	inQuote := false
	escaped := false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if inQuote {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inQuote = false
			}
			continue
		}
		if c == '"' {
			inQuote = true
		}
	}
	return inQuote
}

// hasUnterminatedEnclosingQuoteWithValueOpeningSameQuote reports whether in
// contains a colon-keyed credential assignment sitting INSIDE an already-open
// quote (double or backtick), whose value position is immediately the SAME
// quote character as the one enclosing it, with no FURTHER occurrence of
// that character anywhere later in the document. That is the precise,
// genuinely ambiguous shape the
// colon-value-quote-ambiguous-with-unterminated-enclosing-string finding
// describes: the scanner cannot tell whether the value-position quote opens
// a nested value or is the enclosing string's own (missing) terminator, and
// resolves toward not over-redacting -- at the cost of leaving the
// credential's value, which has no key context once the quote state
// resolves, unredacted.
//
// Written as a plain left-to-right scan (mirroring the style of
// hasColonFinalChainedLabel) rather than by calling into the scanner's own
// quote-tracking, so the registry classifies the shape of the INPUT
// independently of the code under test.
func hasUnterminatedEnclosingQuoteWithValueOpeningSameQuote(in, secret string) bool {
	if secret == "" {
		return false
	}
	var q byte
	inQuote := false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if !inQuote {
			if c == '"' || c == '`' {
				inQuote, q = true, c
			}
			continue
		}
		if c != q {
			continue
		}
		// c == q: this occurrence either closes the current quote, or (the
		// shape this finding is about) IS the key's own value position.
		// Check whether a credential key + colon operator immediately
		// precedes it (across horizontal space only).
		p := i
		for p > 0 && isHSpace(in[p-1]) {
			p--
		}
		if p > 0 && in[p-1] == ':' {
			keyEnd := p - 1
			for keyEnd > 0 && isHSpace(in[keyEnd-1]) {
				keyEnd--
			}
			keyStart := keyEnd
			for keyStart > 0 && isIdentByte(in[keyStart-1]) {
				keyStart--
			}
			if keyStart < keyEnd && isCredentialKey(in[keyStart:keyEnd]) {
				// This occurrence of q is a candidate value-opening quote for
				// a credential key. The ambiguity is genuine only if no
				// FURTHER occurrence of q exists anywhere later in the
				// document (otherwise the scanner has a real close to find
				// and this shape does not apply) and the secret actually
				// follows it.
				rest := in[i+1:]
				if strings.Contains(rest, string(q)) {
					continue
				}
				if strings.Contains(rest, secret) {
					return true
				}
				continue
			}
		}
		inQuote = false
	}
	return false
}

// hasColonFinalChainedLabel reports whether in contains a credential-key
// identifier whose immediately preceding non-horizontal-space byte is '=' or
// ':' AND whose own operator (the first non-space byte after the identifier)
// is ':' -- the exact combination scanner.go's chained-label check in
// tryKeyAt still rejects. It is written independently of that check (a
// plain left-to-right scan over identifier runs, no quote-state machine) so
// that it classifies the INPUT rather than re-deriving the scrubber's own
// decision, which would make the suppression circular.
//
// "Credential-key identifier" here means any of the three isKeyMatch
// mechanisms in scanner.go: an exact isCredentialKey match, an
// isCredentialKeyByWord fallback match, or an isCredentialKeyByFusedSubstring
// match. The chained-label ambiguity this finding describes is about the
// ':'-vs-prose shape surrounding the key, which is identical regardless of
// which of the three mechanisms is what makes the key itself
// credential-shaped in the first place. Classifying only a subset of them
// would misreport the others' identical, already-accepted policy exemption
// (e.g. `--env=SESSION_SECRET: X`, `--env=KEYSTORE: X`) as a brand new,
// unclassified failure instead of the same known one `config: token: X`
// already is.
func hasColonFinalChainedLabel(in string) bool {
	for i := 0; i < len(in); {
		if !isIdentStart(in[i]) {
			i++
			continue
		}
		start := i
		for i < len(in) && isIdentByte(in[i]) {
			i++
		}
		if !isCredentialKey(in[start:i]) && !isCredentialKeyByWord(in[start:i]) && !isCredentialKeyByFusedSubstring(in[start:i]) {
			continue
		}
		p := start
		for p > 0 && isHSpace(in[p-1]) {
			p--
		}
		if p == 0 || (in[p-1] != '=' && in[p-1] != ':') {
			continue
		}
		j := i
		for j < len(in) && isHSpace(in[j]) {
			j++
		}
		if j < len(in) && in[j] == ':' {
			return true
		}
	}
	return false
}

// hasTruncatedPEMWithMultipleTrailingNonBodyLines reports whether in
// contains a PEM BEGIN marker (non-denied label) with no matching END marker
// anywhere in the document, followed by two or more lines, NONE of which
// passes pemBodyLineOK. That is the precise root cause of the
// truncated-pem-multiple-non-body-lines-no-body-line-ever-found finding,
// written independently of scanPEMBlocks's own bookkeeping (a plain re-scan
// of the split lines) so the registry classifies the INPUT's shape rather
// than re-deriving the scrubber's own decision. It is deliberately keyed on
// this condition rather than on a shape name: any BEGIN marker the fuzz
// mutator or a future shape produces that has this exact structure is the
// same defect, not a different one that happens to carry a different label.
func hasTruncatedPEMWithMultipleTrailingNonBodyLines(in string) bool {
	lines := splitPEMLines(in)
	for i := range lines {
		label, at, ok := pemBeginMarkerFrom(lines[i].Text, 0)
		if !ok || pemLabelIsDenied(label) {
			continue
		}
		_ = at
		// No matching END anywhere in the document for this label.
		hasEnd := false
		for j := i + 1; j < len(lines); j++ {
			if endLabel, _, endOK := pemEndMarker(lines[j].Text); endOK && strings.EqualFold(endLabel, label) {
				hasEnd = true
				break
			}
		}
		if hasEnd {
			continue
		}
		// Every line after BEGIN must fail pemBodyLineOK, and there must be
		// at least two of them: exactly one trailing non-body line is a
		// different, already-fixed shape (see scanner.go's
		// trailingRunStart/trailingRunLen handling in scanPEMBlocks).
		nonBodyCount := 0
		allNonBody := true
		for j := i + 1; j < len(lines); j++ {
			if pemBodyLineOK(lines[j].Text) {
				allNonBody = false
				break
			}
			nonBodyCount++
		}
		if allNonBody && nonBodyCount >= 2 {
			return true
		}
	}
	return false
}

// hasColonSchemeValueWithMultiWordTrailer reports whether in contains a
// COLON-operator assignment whose bare (unquoted) value is a recognized HTTP
// auth scheme word (httpAuthSchemes) followed by a space and secret, and
// whose value is in turn followed by TWO OR MORE ordinary trailing words
// (not a recognized structural terminator) -- the exact residual shape the
// space-separated-scheme-value-truncated-at-the-space finding describes.
// Written independently of tryKeyAt's own trailing-context check (a plain
// scan over the input) so the registry classifies the shape of the INPUT
// rather than re-deriving the scrubber's decision, and keyed on this
// condition rather than on the http-basic/http-bearer shape names so a leak
// carrying those same labels for an unrelated reason (e.g. the
// json/triply-escaped context, which is a completely different defect) is
// not misattributed here.
func hasColonSchemeValueWithMultiWordTrailer(in, secret string) bool {
	if secret == "" {
		return false
	}
	i := strings.Index(in, secret)
	if i < 0 {
		return false
	}
	// The scheme word plus one space must sit directly before the secret.
	found := false
	for _, scheme := range httpAuthSchemes {
		prefix := scheme + " "
		if i >= len(prefix) && in[i-len(prefix):i] == prefix {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	// The operator preceding the scheme word (across the key and horizontal
	// space) must be ':'.
	p := i - 1
	for p >= 0 && in[p] != ':' && in[p] != '\n' {
		p--
	}
	if p < 0 || in[p] != ':' {
		return false
	}
	// Two or more ordinary trailing words must follow the credential, none
	// of which is a recognized structural terminator.
	j := bareValueEnd(in, i+len(secret))
	words := 0
	for {
		for j < len(in) && in[j] == ' ' {
			j++
		}
		wordStart := j
		for j < len(in) && in[j] != ' ' {
			if _, isLine := isLineSep(in, j); isLine {
				break
			}
			j++
		}
		if j == wordStart {
			break
		}
		words++
		if words >= 2 {
			return true
		}
	}
	return false
}

// matchOpenFinding reports whether f is an instance of an already-reported,
// not-yet-fixed defect. The first matching entry wins, so openFindings is
// ordered most-specific-first: the PEM entries claim their cases before the
// broader "span ran to EOF" entry, which would otherwise describe them too
// (they are a special case of it, but their PEM-specific root cause is what
// a fixer needs to read).
func matchOpenFinding(f scrubFailure) (string, bool) {
	for _, of := range openFindings {
		if of.Match(f) {
			return of.ID, true
		}
	}
	return "", false
}

// TestScrubFuzz_OpenFindings pins each defect in openFindings to a
// standalone minimal reproduction. It fails in BOTH directions:
//
//   - if the defect stops reproducing, the registry entry is stale and must
//     be deleted (otherwise it would keep suppressing cross-product
//     failures that are now genuine regressions);
//   - if a repro's shape drifts, the mismatch surfaces here rather than
//     silently widening the suppression.
//
// These inputs are hand-minimized FROM generated cross-product failures.
// They are not seeds: nothing in the generator knows about them, and
// deleting this registry does not change which cases the fuzzer explores.
func TestScrubFuzz_OpenFindings(t *testing.T) {
	for _, of := range openFindings {
		of := of
		t.Run(of.ID, func(t *testing.T) {
			out, hits := Scrub(of.Input)
			t.Logf("%s\n  input:  %q\n  output: %q\n  hits:   %d", of.Desc, of.Input, out, hits)
			switch of.ID {
			case "pem-in-json-string-consumes-closing-delimiters",
				"unmatched-value-quote-in-json-string-runs-span-to-eof":
				if json.Valid([]byte(out)) {
					t.Errorf("finding no longer reproduces: output is valid JSON, delete this registry entry")
				}
			default:
				if !strings.Contains(out, of.WantLeak) {
					t.Errorf("finding no longer reproduces: %q no longer survives in %q, delete this registry entry",
						of.WantLeak, out)
				}
			}
		})
	}
}

// TestScrubFuzz_OpenFindingsAreDistinct guards the registry against the two
// ways its Match predicates go wrong, both of which were hit while building
// it:
//
//   - a predicate widens until it swallows a NEIGHBOURING finding, which
//     silently reduces two reported defects to one;
//   - a predicate narrows until it no longer claims its OWN minimal repro,
//     which turns that defect back into an unclassified failure and reds the
//     gate for something already known.
//
// Both are caught by running each entry's own minimal input through the same
// classifier the cross product uses and requiring it to come back with that
// entry's ID. Since matchOpenFinding returns the first match, this also
// pins the most-specific-first ORDER of the slice: reordering it so a broad
// entry precedes a narrow one fails here rather than quietly re-labelling
// hundreds of cases.
func TestScrubFuzz_OpenFindingsAreDistinct(t *testing.T) {
	for _, of := range openFindings {
		of := of
		t.Run(of.ID, func(t *testing.T) {
			out, hits := Scrub(of.Input)
			// Reconstruct the failure the cross product would report for
			// this input, so the classifier sees exactly what it sees there.
			c := scrubCase{
				Name: of.ID, Input: of.Input, Secret: of.WantLeak,
				Shape: of.ReproShape, Context: of.ReproContext, ExpectRedaction: true,
			}
			var reason string
			switch {
			case of.WantLeak != "" && strings.Contains(out, of.WantLeak):
				reason = "secret survived"
			case json.Valid([]byte(of.Input)) && !json.Valid([]byte(out)):
				reason = "valid JSON input produced invalid JSON output"
			default:
				reason = "second Scrub pass reported nonzero hits"
			}
			f := scrubFailure{Case: c, Reason: reason, Output: out, Hits: hits, Surface: of.WantLeak}

			id, known := matchOpenFinding(f)
			if !known {
				t.Errorf("registry entry %s does not classify its own minimal repro %q (reason %q); its Match predicate is too narrow",
					of.ID, of.Input, reason)
				return
			}
			if id != of.ID {
				t.Errorf("registry entry %s classifies its own minimal repro %q as %q instead; one of those Match predicates is too broad, or the slice order puts the broader entry first",
					of.ID, of.Input, id)
			}
		})
	}
}

// TestScrubFuzz_BlockerThree_StrayDoubleQuotePredicateRequiresGenuineImbalance
// is the regression test for BLOCKER 3: leakIsCausedByStrayDoubleQuote used
// to delete everything through the FIRST double quote and treat the leak as
// caused-by-a-stray-quote if that alone made the secret redact -- which is
// true of nearly any leak inside a BALANCED pair of quotes too (deleting the
// opening quote of a balanced pair still perturbs quote parity locally), so
// every balanced-quote colon leak was silently misattributed to this
// finding instead of being reported. Both cases below have literal `"`
// characters and would have satisfied the old (pre-fix) predicate; only the
// second is a genuine case of an unbalanced quote putting the scanner's
// state out of phase.
func TestScrubFuzz_BlockerThree_StrayDoubleQuotePredicateRequiresGenuineImbalance(t *testing.T) {
	cases := []struct {
		name       string
		in, secret string
		want       bool
	}{
		{"balanced quotes: not a stray-quote leak", `echo "password: SEKRIT"`, "SEKRIT", false},
		{"balanced quotes: not a stray-quote leak (curl header)", `curl -H "X-Api-Key: SEKRITVALUE123" https://x`, "SEKRITVALUE123", false},
		{"genuinely unbalanced quote: is a stray-quote leak", "msg=\"unterminated\napi_key: SEKRITVALUE123", "SEKRITVALUE123", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := leakIsCausedByStrayDoubleQuote(c.in, c.secret)
			if got != c.want {
				t.Errorf("leakIsCausedByStrayDoubleQuote(%q, %q) = %v, want %v", c.in, c.secret, got, c.want)
			}
		})
	}
}

// TestScrubFuzz_BlockerFour_ShapeNameKeyedPredicatesAreGone is the
// regression test for BLOCKER 4: two openFindings Match predicates used to
// key on f.Case.Shape (an exact shape-name string) rather than on the
// structural condition that actually causes the defect, which claims ANY
// leak carrying that shape name regardless of root cause. Demonstrated
// concretely here: the json/triply-escaped context leaks for the http-basic
// shape for a COMPLETELY DIFFERENT reason (the triply-escaped-json-invisible
// defect -- no vendor pattern recognizes "Basic <base64>" on its own) than
// the space-separated-scheme-value-truncated-at-the-space finding
// describes, and the old shape-name-keyed predicate (f.Case.Shape ==
// "http-basic" || "http-bearer") mis-swallowed it because slice order put
// that entry first. It must now classify to the correct entry.
//
// http-bearer is not exercised here even though the old predicate named it
// too: in this same context it is independently caught by the vendor
// `Bearer [A-Za-z0-9._\-]+` regex regardless of the assignment scanner's
// escaping depth, so it never actually reaches "secret survived" in
// json/triply-escaped -- asserting on it here would assert something that
// was never a real leak of this shape.
func TestScrubFuzz_BlockerFour_ShapeNameKeyedPredicatesAreGone(t *testing.T) {
	var httpBasic secretShape
	for _, s := range allShapes() {
		if s.Name == "http-basic" {
			httpBasic = s
		}
	}
	if httpBasic.Name == "" {
		t.Fatal("test setup: http-basic shape not found in allShapes()")
	}
	var triplyEscaped scrubContext
	found := false
	for _, ctx := range allContexts() {
		if ctx.Name == "json/triply-escaped" {
			triplyEscaped, found = ctx, true
			break
		}
	}
	if !found {
		t.Fatal("test setup: json/triply-escaped context not found in allContexts()")
	}

	c := triplyEscaped.Build(httpBasic)
	failures := checkScrubCase(c)
	var got []scrubFailure
	for _, f := range failures {
		if f.Reason == "secret survived" {
			got = append(got, f)
		}
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly one 'secret survived' failure for http-basic/json/triply-escaped, got %d: %+v", len(got), failures)
	}
	id, known := matchOpenFinding(got[0])
	if !known {
		t.Fatalf("failure for http-basic/json/triply-escaped does not classify against any open finding")
	}
	if id == "space-separated-scheme-value-truncated-at-the-space" {
		t.Errorf("misattributed to the shape-name-keyed finding (BLOCKER 4 regression): got id=%s, want triply-escaped-json-invisible", id)
	}
	if id != "triply-escaped-json-invisible" {
		t.Errorf("classified as %s, want triply-escaped-json-invisible", id)
	}
}

// --- Slice 1: shapes in isolation -------------------------------------------

// TestScrubFuzz_FillersAreInert guards the generator itself. A filler that
// Scrub redacts would mask a real leak: the surviving-secret assertion
// would still pass on an output whose only redaction came from the noise
// around the secret, and the hits accounting would be attributed to the
// wrong span. This is the same "test passing for the wrong reason" trap
// that using a published vendor example (AKIAIOSFODNN7EXAMPLE) as filler
// falls into, which is why no such literal appears anywhere in the fuzzer.
func TestScrubFuzz_FillersAreInert(t *testing.T) {
	for _, f := range benignFillers {
		t.Run(f, func(t *testing.T) {
			out, hits := Scrub(f)
			if hits != 0 || out != f {
				t.Errorf("filler %q is not inert: Scrub returned %q hits=%d", f, out, hits)
			}
		})
	}
}

// TestScrubFuzz_ShapesInIsolation checks the shape axis on its own, before
// any context is wrapped around it. A shape that leaks bare is a defect in
// the detector itself; a shape that only leaks in some context is a defect
// in how the scanner handles that context. Separating them keeps the
// cross-product report readable.
func TestScrubFuzz_ShapesInIsolation(t *testing.T) {
	for _, s := range allShapes() {
		s := s
		t.Run(s.Name, func(t *testing.T) {
			c := scrubCase{
				Name: s.Name, Input: s.Text, Secret: s.Secret,
				Shape: s.Name, Context: "bare", ExpectRedaction: true,
			}
			reportFailures(t, checkScrubCase(c))
		})
	}
}

// TestScrubFuzz_EveryCredentialKey exercises every key in credentialKeys in
// the single simplest context, so that no key in the set is silently
// unexercised by the (deliberately narrower) assignmentKeysUnderTest list
// the cross product uses.
func TestScrubFuzz_EveryCredentialKey(t *testing.T) {
	i := 0
	for key := range credentialKeys {
		i++
		val := "Zq" + fakeBody(500+i, 18)
		t.Run(key, func(t *testing.T) {
			c := scrubCase{
				Name: "key-" + key, Input: key + "=" + val, Secret: val,
				Shape: "assignment", Context: "bare", ExpectRedaction: true,
			}
			reportFailures(t, checkScrubCase(c))
		})
	}
}

// --- Slice 2: the deterministic cross product -------------------------------

// TestScrubFuzz_CrossProduct is the CI gate: the full deterministic cross
// product of every shape against every context, with no fuzzing engine and
// no special flags. It is what actually catches regressions; the native
// fuzz target in scrubber_fuzz_test.go explores beyond this table but is
// not run by default.
//
// Failures are aggregated rather than reported one-per-subtest, because a
// single detector defect typically fires across dozens of context/shape
// pairs and one readable grouped report is more useful than 200 identical
// t.Error lines. The per-context subtest structure is kept so `go test
// -run` can isolate one context.
func TestScrubFuzz_CrossProduct(t *testing.T) {
	start := time.Now()
	shapes := allShapes()
	contexts := allContexts()

	total := 0
	for _, ctx := range contexts {
		ctx := ctx
		t.Run(ctx.Name, func(t *testing.T) {
			var failures []scrubFailure
			for _, s := range shapes {
				if !ctx.Accepts(s) {
					continue
				}
				c := ctx.Build(s)
				total++
				failures = append(failures, checkScrubCase(c)...)
			}
			reportFailures(t, failures)
		})
	}
	t.Logf("cross product: %d shapes x %d contexts = %d cases in %s",
		len(shapes), len(contexts), total, time.Since(start).Round(time.Millisecond))
}

// TestScrubFuzz_Audit runs the same cross product with the openFindings
// suppression switched OFF and prints a grouped taxonomy of every violation
// instead of a per-case dump. It is how you answer "what does this scrubber
// leak, in total, right now" without reading 2,000 individual failures, and
// it is the tool used to measure a candidate fix (or to measure what an
// older revision leaked).
//
// It is skipped unless SCRUBFUZZ_AUDIT=1, because it is a reporting tool
// rather than an assertion: on a tree with known-open findings it would
// always "fail", which is exactly the permanently-red gate reportFailures
// exists to avoid.
func TestScrubFuzz_Audit(t *testing.T) {
	if os.Getenv("SCRUBFUZZ_AUDIT") != "1" {
		t.Skip("set SCRUBFUZZ_AUDIT=1 to run the leak taxonomy report")
	}
	start := time.Now()
	shapes := allShapes()
	contexts := allContexts()

	// Grouped by (reason, shape kind, context family) so one defect
	// collapses to one line no matter how many pairs it fires on, with an
	// example input kept for each group.
	type groupKey struct{ reason, kind, family string }
	type groupVal struct {
		count   int
		example scrubFailure
	}
	groups := map[groupKey]*groupVal{}
	kindOf := map[string]string{}
	for _, s := range shapes {
		kindOf[s.Name] = s.Kind
	}

	total, leaking := 0, 0
	for _, ctx := range contexts {
		for _, s := range shapes {
			if !ctx.Accepts(s) {
				continue
			}
			c := ctx.Build(s)
			total++
			fs := checkScrubCase(c)
			if len(fs) > 0 {
				leaking++
			}
			for _, f := range fs {
				family := f.Case.Context
				if i := strings.IndexByte(family, '/'); i >= 0 {
					family = family[:i]
				}
				k := groupKey{f.Reason, kindOf[f.Case.Shape], family}
				g := groups[k]
				if g == nil {
					g = &groupVal{}
					groups[k] = g
				}
				g.count++
				if g.example.Reason == "" {
					g.example = f
				}
			}
		}
	}

	keys := make([]groupKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if groups[keys[i]].count != groups[keys[j]].count {
			return groups[keys[i]].count > groups[keys[j]].count
		}
		return keys[i].reason < keys[j].reason
	})

	t.Logf("AUDIT: %d cases, %d with at least one violation, %d distinct groups, %s",
		total, leaking, len(keys), time.Since(start).Round(time.Millisecond))
	for _, k := range keys {
		g := groups[k]
		id, known := matchOpenFinding(g.example)
		if !known {
			id = "UNCLASSIFIED"
		}
		t.Logf("AUDITGROUP count=%d reason=%q kind=%s ctxfamily=%s finding=%s\n  example-context: %s\n  example-shape:   %s\n  example-input:   %q\n  example-output:  %q\n  example-hits:    %d",
			g.count, k.reason, k.kind, k.family, id,
			g.example.Case.Context, g.example.Case.Shape,
			g.example.Case.Input, g.example.Output, g.example.Hits)
	}
}

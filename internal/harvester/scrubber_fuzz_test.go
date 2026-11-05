package harvester

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Native Go fuzz target --------------------------------------------------
//
// FuzzScrub explores beyond the deterministic cross product in
// scrubfuzz_test.go. The cross product is the CI gate (it runs under a
// plain `go test`, in under a second, and is what a reviewer reads); this
// target is what finds the shapes nobody enumerated.
//
// The design problem native Go fuzzing poses for a scrubber is that its
// mutator produces arbitrary bytes, and an arbitrary byte string has no
// known secret in it -- which throws away the whole oracle. A crash-only
// target would be nearly useless here: Scrub does not panic, it
// under-redacts.
//
// The fix is to keep the oracle by never letting the mutator choose the
// secret. The corpus entry is (shapeIdx, contextIdx, prefix, suffix,
// keyName, valueSeed): the fuzzer picks WHICH known shape and WHICH known
// context to combine, and gets free rein over the surrounding bytes, but
// the secret itself is always synthesized by the test from valueSeed, so
// the test always knows exactly what must not survive. The mutator's real
// job is the third and fourth arguments -- the adjacent bytes -- which is
// precisely where every real leak in this scrubber has lived.
//
// Run it with:
//
//	go test ./internal/harvester/ -run '^$' -fuzz FuzzScrub -fuzztime 60s
//
// The seed corpus is checked in under testdata/fuzz/FuzzScrub/ so the
// interesting starting points are reproducible rather than rediscovered
// from scratch on every run.

// fuzzSecretAlphabetLen bounds the synthesized value length so a single
// corpus entry cannot generate a multi-megabyte input and dominate the
// fuzzing budget with one slow case.
const fuzzMaxSecretLen = 64

// fuzzSynthValue builds the opaque credential value for a fuzz case. It is
// deliberately NOT taken from the mutator: the value must be a string the
// test knows in full and which no vendor regex would match on its own, so
// that a surviving value proves the assignment scanner failed rather than
// proving some vendor pattern happened not to fire.
func fuzzSynthValue(seed uint32) string {
	n := 12 + int(seed%uint32(fuzzMaxSecretLen-12))
	return "Zq" + fakeBody(int(seed), n)
}

// leadingIdentIsCredentialKey reports whether the first identifier run in
// s (skipping any leading non-identifier bytes such as a quote or a brace)
// is a credential key. It answers "would a jamming prefix rename this
// input's key?", which is what distinguishes a generator artifact from a
// real jamming leak.
func leadingIdentIsCredentialKey(s string) bool {
	i := 0
	for i < len(s) && !isIdentStart(s[i]) {
		i++
	}
	start := i
	for i < len(s) && isIdentByte(s[i]) {
		i++
	}
	return start < i && isCredentialKey(s[start:i])
}

// fuzzScrubSeed is one corpus entry: which shape and context to combine,
// which affixes to wrap them in, and the seed for the synthesized value.
// Name is used only as the filename when the entry is materialized into
// testdata/fuzz/FuzzScrub/ (see TestFuzzScrub_SeedCorpusIsCheckedIn), so a
// reader can tell at a glance which starting point each corpus file is.
type fuzzScrubSeed struct {
	Name             string
	ShapeIdx, CtxIdx int
	Prefix, Suffix   string
	Key              string
	ValueSeed        uint32
}

// fuzzScrubSeeds is the checked-in seed corpus, kept as data rather than
// inline f.Add calls so that exactly the same list feeds both the fuzz
// target's in-code seeds and the files under testdata/fuzz/FuzzScrub/. One
// source of truth: a corpus file cannot drift from the seed it documents.
//
// Every entry is indices and affixes only. No credential literal is written
// into the corpus, so the checked-in testdata contains nothing that even
// resembles a key -- the value is synthesized from ValueSeed at run time.
//
// Coverage is chosen for the defect FAMILIES the contexts exist to probe
// (adjacency/jamming, trailing terminators, quoting, JSON nesting, YAML
// blocks, env/shell, PEM markers, lead-ins), not for breadth: the fuzzer
// mutates indices freely, so a seed's job is to land the engine inside each
// family's coverage neighborhood, not to enumerate it.
var fuzzScrubSeeds = []fuzzScrubSeed{
	{"bare", 0, 0, "", "", "password", 1},
	{"jam-letter-hash-comment", 1, 5, "X", " # prod", "api_key", 2},
	{"jam-digit-tab-column", 5, 12, "7", "\tcolumn", "token", 3},
	{"jam-underscore-paren", 11, 40, "_", " (prod)", "secret", 4},
	{"jam-word-crlf", 17, 100, "prefixword", "\r\n", "PGPASSWORD", 5},
	{"pem-marker-after-prose", 20, 200, "note ", "", "authorization", 6},
	{"chained-label-equals", 25, 400, "=", "}", "private_key", 7},
	{"prose-prefix-comma", 30, 800, "the value is ", ",", "client_secret", 8},
	{"tab-prefix-pipe", 40, 1200, "\t", "|", "passphrase", 9},
	{"newline-prefix-slash-comment", 50, 1600, "\n", "// note", "dsn", 10},
	{"word-prefix-bracket", 60, 1650, "task", "]", "refresh_token", 11},
	{"dot-prefix-paren", 70, 1670, ".", ")", "api_key2", 12},
	{"slash-prefix-ampersand", 80, 1675, "/", "&", "x-api-key", 13},
	{"colon-prefix-semicolon", 90, 1677, ":", ";", "oauth2_token", 14},
	{"quote-prefix", 3, 500, "\"", "", "password", 15},
	{"apostrophe-prefix", 7, 900, "'", "", "token", 16},
	{"backtick-prefix", 13, 1300, "`", "", "secret", 17},
	{"escaped-quote-prefix", 23, 1400, "\\\"", "", "auth", 18},
	{"trailing-open-paren", 33, 1500, "", "(", "credential", 19},
	{"trailing-word", 43, 1550, "", " word", "pat", 20},
}

// TestFuzzScrub_SeedCorpusIsCheckedIn materializes fuzzScrubSeeds into
// testdata/fuzz/FuzzScrub/ and fails if what is on disk differs, so the
// corpus is reproducible from source rather than being an opaque set of
// files somebody once generated.
//
// Run with -update to (re)write the files after changing the seed list:
//
//	go test ./internal/harvester/ -run TestFuzzScrub_SeedCorpusIsCheckedIn -update
//
// Go's own fuzzing engine reads these files automatically, so they are seeds
// in the engine's sense as well as documentation. They duplicate the in-code
// f.Add seeds on purpose: f.Add works when the package is vendored without
// testdata, and the files work when the corpus is inspected or extended by
// hand.
func TestFuzzScrub_SeedCorpusIsCheckedIn(t *testing.T) {
	dir := filepath.Join("testdata", "fuzz", "FuzzScrub")
	if *updateCorpus {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range fuzzScrubSeeds {
		s := s
		t.Run(s.Name, func(t *testing.T) {
			want := fmt.Sprintf("go test fuzz v1\nint(%d)\nint(%d)\nstring(%q)\nstring(%q)\nstring(%q)\nuint32(%d)\n",
				s.ShapeIdx, s.CtxIdx, s.Prefix, s.Suffix, s.Key, s.ValueSeed)
			path := filepath.Join(dir, s.Name)
			if *updateCorpus {
				if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("seed corpus file missing: %v (run with -update to regenerate)", err)
			}
			if string(got) != want {
				t.Errorf("seed corpus file %s is stale:\n got: %q\nwant: %q\n(run with -update to regenerate)",
					path, got, want)
			}
		})
	}
}

// updateCorpus regenerates the checked-in seed corpus instead of verifying
// it. Declared here rather than in an init so the flag name shows up in
// `go test -args -h` for this package.
var updateCorpus = flag.Bool("update", false, "regenerate the checked-in FuzzScrub seed corpus under testdata/")

// FuzzScrub crosses a mutator-chosen shape and context against
// mutator-chosen surrounding bytes, and applies the same oracle the
// deterministic table test uses.
func FuzzScrub(f *testing.F) {
	shapes := allShapes()
	contexts := allContexts()

	// Seeds cover one entry per broad family, plus the adjacency and
	// nesting shapes that historically leaked. Values here are indices and
	// affixes only: no secret literal is ever written into the corpus, so
	// the checked-in testdata contains nothing credential-shaped.
	for _, s := range fuzzScrubSeeds {
		f.Add(s.ShapeIdx, s.CtxIdx, s.Prefix, s.Suffix, s.Key, s.ValueSeed)
	}

	f.Fuzz(func(t *testing.T, shapeIdx, ctxIdx int, prefix, suffix, key string, valueSeed uint32) {
		// Index selection is modular rather than rejecting out-of-range
		// values, so no mutation is wasted: every integer the mutator
		// produces still selects a real shape/context pair.
		if len(shapes) == 0 || len(contexts) == 0 {
			t.Skip()
		}
		s := shapes[((shapeIdx%len(shapes))+len(shapes))%len(shapes)]
		ctx := contexts[((ctxIdx%len(contexts))+len(contexts))%len(contexts)]
		if !ctx.Accepts(s) {
			t.Skip()
		}

		// Override the shape's value (and, for assignment shapes, its key)
		// with mutator-influenced-but-test-known material. The key is only
		// substituted when the mutator supplied one that is actually a
		// credential key: an arbitrary key name would make non-detection
		// correct behavior and destroy the oracle.
		// Only the generic-assignment shapes get a mutator-seeded value.
		// Vendor and PEM shapes keep their own literal: for those, Text,
		// Secret and AssignmentValue are three views of the SAME credential
		// (the vendor key itself), and overwriting only AssignmentValue
		// would leave contexts embedding one string while the oracle
		// checked for another -- reporting a "leak" whose secret was never
		// in the input. Every such report is noise that costs a real
		// triage, so the invariant is enforced here rather than patched
		// later.
		if s.Kind == "assignment" {
			s.AssignmentValue = fuzzSynthValue(valueSeed)
			s.Secret = s.AssignmentValue
			s.Text = s.AssignmentKey + "=" + s.AssignmentValue
			if isCredentialKey(key) {
				s.AssignmentKey = key
				s.Text = key + "=" + s.AssignmentValue
			}
		}

		c := ctx.Build(s)
		// The mutator's affixes go OUTSIDE the built case, so they can
		// never accidentally contain or split the secret: the oracle stays
		// exact. Affixes containing the placeholder itself are dropped,
		// since a pre-existing "[REDACTED]" makes the hits-accounting
		// property inapplicable by design (see Scrub's own doc comment).
		if strings.Contains(prefix, redactedToken) || strings.Contains(suffix, redactedToken) {
			t.Skip()
		}
		// A prefix ending in an identifier byte, immediately in front of a
		// case that begins with an identifier, does not jam against the
		// key: it RENAMES it ("A" + "dsn" = "Adsn", "A" + "SECRET" =
		// "ASECRET"), and the renamed identifier is correctly not a
		// credential key, so non-detection is right behavior rather than a
		// leak. Skipping this keeps the oracle honest.
		//
		// The check is on whether the FIRST identifier of the case is a
		// credential key, not on a literal prefix match against
		// s.AssignmentKey: contexts legitimately transform the key
		// (uppercasing it for .env, quoting it for JSON), and an
		// AssignmentKey-only check missed every one of those. Jamming
		// against the credential VALUE is a different thing and is still
		// generated -- that is exactly the \b-anchor defect class, which
		// the adjacency contexts cover deliberately.
		if len(prefix) > 0 && isIdentByte(prefix[len(prefix)-1]) &&
			leadingIdentIsCredentialKey(c.Input) {
			t.Skip()
		}
		// YAML is indentation-significant, so a prefix that does not end at
		// a line boundary re-indents the first line and can turn a valid
		// document into an invalid one -- e.g. " " in front of
		// "authToken: >\nSECRET\n" leaves the block body at column 0 under
		// a column-1 key, which YAML does not permit. Non-detection of an
		// invalid document is not a leak, so those combinations are
		// skipped. Prefixes that end in a newline (and therefore leave the
		// document's own indentation intact) are still generated.
		if strings.HasPrefix(ctx.Name, "yaml/") && prefix != "" &&
			!strings.HasSuffix(prefix, "\n") {
			t.Skip()
		}
		c.Input = prefix + c.Input + suffix
		if strings.Contains(prefix+suffix, c.Secret) {
			// The mutator reproduced the secret in the surrounding bytes,
			// which would make "the secret survived" ambiguous.
			t.Skip()
		}

		for _, fail := range checkScrubCase(c) {
			if id, known := matchOpenFinding(fail); known {
				t.Skipf("known-open finding %s", id)
			}
			t.Errorf("LEAK[%s] shape=%s context=%s\n  reason: %s\n  input:  %q\n  output: %q\n  hits:   %d\n  secret: %q",
				fail.Case.Name, fail.Case.Shape, fail.Case.Context, fail.Reason,
				fail.Case.Input, fail.Output, fail.Hits, fail.Surface)
		}
	})
}

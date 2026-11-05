package harvester

import (
	"fmt"
	"strings"
)

// --- Differential fuzzer: secret shapes -------------------------------------
//
// This file and its siblings (scrubfuzz_contexts_test.go,
// scrubfuzz_test.go, scrubber_fuzz_test.go) implement a generative
// differential fuzzer for Scrub. The oracle it exploits is one most fuzz
// targets do not have: the test GENERATES the secret, so it knows exactly
// which byte sequence must not survive. No crash, no second implementation,
// no human judgement -- any generated input whose known secret token still
// appears in Scrub's output is a definite under-redaction bug.
//
// The generator is two composable axes crossed against each other:
//
//	SHAPE   (this file)  -- what the credential looks like: the vendor
//	                        formats scrubPatterns recognizes, generic
//	                        key=value assignments over credentialKeys, and
//	                        PEM blocks over labels and body shapes.
//	CONTEXT (contexts)   -- what the credential is embedded in: quoting,
//	                        JSON, doubly-escaped JSON, YAML, .env, shell,
//	                        trailing comments, jamming characters, EOF and
//	                        CRLF handling, unterminated delimiters.
//
// Every shape is a synthetic literal built from a fixed obviously-fake
// alphabet (see fakeBody). No real credential, and no PUBLISHED VENDOR
// EXAMPLE STRING, ever appears here. Published examples (the AWS
// AKIAIOSFODNN7EXAMPLE key being the notorious one) are specifically
// banned as filler because they match a live vendor pattern and would
// silently satisfy an assertion that was meant to fail -- a test passing
// for the wrong reason. Filler values in this fuzzer are drawn from
// benignFillers, which are checked (see TestScrubFuzz_FillersAreInert) to
// be inert under Scrub.

// fakeAlphabet is the character pool every synthetic secret body is drawn
// from. It is deliberately alphanumeric-only so a body can be spliced into
// any vendor shape's own alphabet without accidentally introducing a
// delimiter, and the generated runs are visibly patterned ("Aa0Bb1...")
// rather than random-looking, so a literal that escapes into a log or a
// diff is obviously synthetic.
const fakeAlphabet = "AaBbCc0Dd1Ee2Ff3Gg4Hh5Ii6Jj7Kk8Ll9MmNnOoPpQqRrSsTtUuVvWwXxYyZz"

// fakeBody returns a deterministic pseudo-random run of n characters from
// fakeAlphabet, seeded by seed so different call sites get different (but
// reproducible) bodies.
func fakeBody(seed, n int) string {
	var b strings.Builder
	b.Grow(n)
	// A simple LCG: deterministic, dependency-free, and adequate for
	// producing distinguishable-but-reproducible filler.
	x := uint32(seed)*2654435761 + 12345
	for i := 0; i < n; i++ {
		x = x*1664525 + 1013904223
		b.WriteByte(fakeAlphabet[int(x>>16)%len(fakeAlphabet)])
	}
	return b.String()
}

// fakeLowerAlphabet is a letters-only alphabet restricted to lowercase
// only: no digit AND no uppercase letter anywhere. Used by fakeLowerBody to
// generate a value that is guaranteed, by construction, to fail all three
// looksLikeCredentialValue disjuncts at once (no digit, no mixed case) as
// long as it is also kept under looksLikeCredentialValueMinLen bytes -- see
// fusedSubstringAssignmentShapes for why that combination, not merely
// "digit-free," is the value class this fuzzer previously could not
// generate at all.
const fakeLowerAlphabet = "abcdefghijklmnopqrstuvwxyz"

// fakeLowerBody is fakeBody restricted to fakeLowerAlphabet: a deterministic
// pseudo-random run of n lowercase letters, single-case and digit-free by
// construction on every seed, every run.
func fakeLowerBody(seed, n int) string {
	var b strings.Builder
	b.Grow(n)
	x := uint32(seed)*2654435761 + 12345
	for i := 0; i < n; i++ {
		x = x*1664525 + 1013904223
		b.WriteByte(fakeLowerAlphabet[int(x>>16)%len(fakeLowerAlphabet)])
	}
	return b.String()
}

// fakeUpperBody is fakeBody restricted to [0-9A-Z], for shapes (AWS) whose
// alphabet excludes lowercase.
func fakeUpperBody(seed, n int) string {
	const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var b strings.Builder
	b.Grow(n)
	x := uint32(seed)*2246822519 + 7
	for i := 0; i < n; i++ {
		x = x*1664525 + 1013904223
		b.WriteByte(alpha[int(x>>16)%len(alpha)])
	}
	return b.String()
}

// fakeB64Body returns n characters of base64-alphabet filler, for PEM
// bodies.
func fakeB64Body(seed, n int) string {
	const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var b strings.Builder
	b.Grow(n)
	x := uint32(seed)*40503 + 99991
	for i := 0; i < n; i++ {
		x = x*1664525 + 1013904223
		b.WriteByte(alpha[int(x>>16)%len(alpha)])
	}
	return b.String()
}

// secretShape is one generated credential literal plus the substring that
// must not survive Scrub.
//
// Text is what gets embedded into a context. Secret is the portion of Text
// whose survival constitutes a leak. They differ for the assignment and PEM
// shapes: "password=SEKRIT" must be embedded whole (the key is what makes
// it detectable) but only "SEKRIT" must disappear, since Scrub deliberately
// preserves keys and PEM markers are preserved for denied labels.
//
// Kind groups shapes for reporting: which family of detector is under test.
type secretShape struct {
	Name   string
	Kind   string
	Text   string
	Secret string
	// SelfContained reports whether Text alone (with no assignment key
	// wrapped around it) is expected to be redacted. Vendor and PEM shapes
	// are self-contained; a bare generic value like "hunter2" is not -- it
	// only becomes a secret when attached to a credential key.
	SelfContained bool
	// AssignmentValue is the value used when a context needs to build its
	// own "key=value" wrapper (JSON, YAML, .env, etc.). For vendor shapes
	// it is the vendor literal; for generic shapes it is the opaque value.
	AssignmentValue string
	// AssignmentKey is the credential key name a context should use when
	// building an assignment around AssignmentValue.
	AssignmentKey string
}

// vendorShapes returns one synthetic literal for each vendor credential
// format scrubPatterns claims to recognize. Bodies are sized comfortably
// above each pattern's own length floor so a shape's failure is never
// merely "too short".
func vendorShapes() []secretShape {
	mk := func(name, text string) secretShape {
		return secretShape{
			Name:            name,
			Kind:            "vendor",
			Text:            text,
			Secret:          text,
			SelfContained:   true,
			AssignmentValue: text,
			AssignmentKey:   "api_key",
		}
	}
	shapes := []secretShape{
		mk("aws-akia", "AKIA"+fakeUpperBody(1, 16)),
		mk("anthropic-sk-ant", "sk-ant-api03-"+fakeBody(2, 40)),
		mk("openai-sk-proj", "sk-proj-"+fakeBody(3, 40)),
		mk("openai-sk-flat", "sk-"+fakeBody(4, 32)),
		mk("openrouter-sk-or-v1", "sk-or-v1-"+fakeBody(5, 40)),
		mk("github-ghp", "ghp_"+fakeBody(6, 36)),
		mk("github-gho", "gho_"+fakeBody(7, 36)),
		mk("github-ghu", "ghu_"+fakeBody(8, 36)),
		mk("github-ghs", "ghs_"+fakeBody(9, 36)),
		mk("github-ghr", "ghr_"+fakeBody(10, 36)),
		mk("github-gha", "gha_"+fakeBody(11, 36)),
		mk("github-pat", "github_pat_"+fakeBody(12, 40)),
		mk("slack-xoxb", "xoxb-"+fakeBody(13, 12)+"-"+fakeBody(14, 12)),
		mk("slack-xoxp", "xoxp-"+fakeBody(15, 12)+"-"+fakeBody(16, 12)),
		mk("slack-xoxa", "xoxa-"+fakeBody(17, 12)+"-"+fakeBody(18, 12)),
		mk("slack-xoxs", "xoxs-"+fakeBody(19, 12)+"-"+fakeBody(20, 12)),
		mk("slack-xapp", "xapp-1-"+fakeBody(21, 16)),
		mk("google-aizasy", "AIzaSy"+fakeBody(22, 33)),
		mk("npm-token", "npm_"+fakeBody(23, 36)),
		mk("jwt", "eyJ"+fakeBody(24, 20)+"."+fakeBody(25, 24)+"."+fakeBody(26, 24)),
	}
	// Bearer / Authorization Basic / URL credentials carry surrounding
	// syntax that is itself part of the pattern, so Text and Secret differ.
	bearerTok := fakeBody(27, 36)
	shapes = append(shapes, secretShape{
		Name: "http-bearer", Kind: "vendor",
		Text: "Bearer " + bearerTok, Secret: bearerTok,
		SelfContained: true, AssignmentValue: "Bearer " + bearerTok, AssignmentKey: "authorization",
	})
	basicTok := fakeBody(28, 32) + "=="
	shapes = append(shapes, secretShape{
		Name: "http-basic", Kind: "vendor",
		Text: "Authorization: Basic " + basicTok, Secret: basicTok,
		SelfContained: true, AssignmentValue: "Basic " + basicTok, AssignmentKey: "authorization",
	})
	urlPass := fakeBody(29, 20)
	shapes = append(shapes, secretShape{
		Name: "url-userinfo", Kind: "vendor",
		Text: "postgres://svcuser:" + urlPass + "@db.internal:5432/app", Secret: urlPass,
		SelfContained: true, AssignmentValue: "postgres://svcuser:" + urlPass + "@db.internal:5432/app",
		AssignmentKey: "connection_string",
	})
	return shapes
}

// assignmentKeysUnderTest is the subset of credentialKeys exercised by the
// generic-assignment shapes. It covers every naming convention the key
// collapser has to normalize (plain word, snake_case, kebab-case,
// SCREAMING_CASE, digit-suffixed, dotted-prefix) rather than every key in
// the set, keeping the deterministic cross product inside its time budget.
// TestScrubFuzz_EveryCredentialKey separately checks all of credentialKeys
// in the single simplest context, so no key is silently unexercised.
var assignmentKeysUnderTest = []string{
	"password",
	"PASSWORD",
	"api_key",
	"api-key",
	"apiKey",
	"secret",
	"client_secret",
	"AWS_SECRET_ACCESS_KEY",
	"PGPASSWORD",
	"token",
	"refresh_token",
	"authToken",
	"private_key",
	"passphrase",
	"api_key2",
	"oauth2_token",
	"dsn",
	"x-api-key",
}

// genericAssignmentShapes returns generic key=value credential shapes: a
// credential key from assignmentKeysUnderTest paired with an opaque value
// that no vendor regex would ever match on its own. That last property is
// load-bearing: if the value were vendor-shaped, the independent vendor
// regex would redact it regardless of whether the hand-written assignment
// scanner worked at all, and the test would pass for the wrong reason.
func genericAssignmentShapes() []secretShape {
	var out []secretShape
	for i, key := range assignmentKeysUnderTest {
		val := "Zq" + fakeBody(100+i, 18) // opaque; no vendor prefix
		out = append(out, secretShape{
			Name:            "assign-" + key,
			Kind:            "assignment",
			Text:            key + "=" + val,
			Secret:          val,
			SelfContained:   false,
			AssignmentValue: val,
			AssignmentKey:   key,
		})
	}
	return out
}

// wordFallbackKeysUnderTest is the ordinary .env-style secret key names
// that are NOT themselves literal members of credentialKeys (unlike every
// entry in assignmentKeysUnderTest, which all collapse to an exact
// allow-list member): each is only recognized via isCredentialKeyByWord,
// the per-word fallback that matches one whole separator/camelCase-
// delimited word of the identifier (e.g. ADMIN_PASSWORD's "password",
// RAILS_MASTER_KEY's "key") rather than the identifier as a whole. Before
// that fallback existed, every one of these leaked in full: the closed
// exact-match allow-list has no entry for "adminpassword", "jwtsecret",
// "stripesecretkey" and never will, since enumerating every possible
// prefix a real secret key name might carry is an open set. Values are
// deliberately opaque (no vendor prefix) for the same reason
// genericAssignmentShapes' values are: a vendor-shaped value would let an
// independent vendor regex redact it regardless of whether this fallback
// works at all.
var wordFallbackKeysUnderTest = []string{
	"ADMIN_PASSWORD",
	"JWT_SECRET",
	"SESSION_SECRET",
	"POSTGRES_PASSWORD",
	"MYSQL_PASSWORD",
	"REDIS_PASSWORD",
	"SMTP_PASSWORD",
	"NPM_TOKEN",
	"VERCEL_TOKEN",
	"WEBHOOK_SECRET",
	"SIGNING_SECRET",
	"ENCRYPTION_SECRET",
	"RAILS_MASTER_KEY",
	"DJANGO_SECRET_KEY",
	"SSH_PASSPHRASE",
}

// wordFallbackAssignmentShapes returns the same shape of secretShape as
// genericAssignmentShapes, but for wordFallbackKeysUnderTest instead of
// assignmentKeysUnderTest -- the credential-substring-in-a-compound-
// identifier class isCredentialKeyByWord exists to cover.
//
// A key recognized by isCredentialKeyByWord now redacts its value
// UNCONDITIONALLY, exactly like an exact credentialKeys match -- see
// tryKeyAt's isKeyMatch doc comment for why gating this path by
// looksLikeCredentialValue was the design defect this round of work
// replaces. Values here are therefore deliberately varied (digit-bearing,
// digit-free, long, short) not to satisfy any value-shape gate -- there is
// none left to satisfy on this path -- but simply as ordinary generator
// hygiene: an opaque, non-vendor-shaped value that exercises the key
// recognition path on its own terms regardless of what it looks like.
func wordFallbackAssignmentShapes() []secretShape {
	var out []secretShape
	for i, key := range wordFallbackKeysUnderTest {
		val := "Zq" + fakeBody(300+i, 18) // opaque; no vendor prefix
		out = append(out, secretShape{
			Name:            "assign-wordfallback-" + key,
			Kind:            "assignment",
			Text:            key + "=" + val,
			Secret:          val,
			SelfContained:   false,
			AssignmentValue: val,
			AssignmentKey:   key,
		})
		// A short (<16 bytes), single-case, digit-free value: the class a
		// value-shape gate would have rejected, kept here as a first-class
		// generated case (rather than assumed inert) now that the key-match
		// path this shape exercises has no such gate left to satisfy at all.
		valShort := fakeLowerBody(300+i, 10)
		out = append(out, secretShape{
			Name:            "assign-wordfallback-short-nodigit-" + key,
			Kind:            "assignment",
			Text:            key + "=" + valShort,
			Secret:          valShort,
			SelfContained:   false,
			AssignmentValue: valShort,
			AssignmentKey:   key,
		})
	}
	return out
}

// fusedSubstringKeysUnderTest is a synthetic single-fused-word (no
// separator, no camelCase boundary) identifier for each of prefix, middle,
// and suffix position of a credential word inside a larger compound --
// exactly the three positions isCredentialKeyByFusedSubstring exists to
// recognize (see that function's doc comment). "middle" entries splice the
// credential word between two ordinary non-credential fragments so the
// match cannot be mistaken for a prefix or suffix case.
//
// This is the fix for the fuzz shape gap the sixth gate round found: the
// PREVIOUS version of this cross product could reach a credential word only
// at the END of a fused identifier (isCredentialKeyByFusedSuffix's own
// scope), so PASSWORDADMIN (prefix), SECRETJWT (prefix), KEYSTORE (middle),
// MYPASSWORDVALUE (middle), and APPSECRETSTORE (middle) were structurally
// unreachable by this generator no matter how many seeds or contexts ran --
// the shape of the KEY, not the value, was what earlier rounds' generator
// could not produce.
var fusedSubstringKeysUnderTest = []struct {
	Name string
	Key  string
}{
	{"prefix-password", "PASSWORDADMIN"},
	{"prefix-secret", "SECRETJWT"},
	{"prefix-pass", "XPASS"},
	{"middle-key", "KEYSTORE"},
	{"middle-password", "MYPASSWORDVALUE"},
	{"middle-secret", "APPSECRETSTORE"},
	{"suffix-token", "TOKENVALUE"},
	{"suffix-auth", "AUTHDATA"},
	{"suffix-pass", "DBPASS"},
	{"suffix-secret", "JWTSECRET"},
}

// fusedSubstringAssignmentShapes returns the same shape of secretShape as
// wordFallbackAssignmentShapes, but for fusedSubstringKeysUnderTest: the
// prefix/middle/suffix fused-identifier class isCredentialKeyByFusedSubstring
// exists to cover, with no separator or camelCase boundary anywhere in the
// key for splitIdentWords to find (see that function's doc comment for why
// that structural property is what makes this class invisible to
// isCredentialKeyByWord on its own).
//
// Two value variants per key, exactly mirroring wordFallbackAssignmentShapes:
// an ordinary opaque value, and a short/single-case/digit-free one. Neither
// is gated by looksLikeCredentialValue any more (this key-match path is
// unconditional, like every other isKeyMatch path), so both are generated
// simply to exercise the key-recognition path itself across the value shapes
// a real secret manager might actually produce.
func fusedSubstringAssignmentShapes() []secretShape {
	var out []secretShape
	for i, k := range fusedSubstringKeysUnderTest {
		val := "Zq" + fakeBody(600+i, 18) // opaque; no vendor prefix
		out = append(out, secretShape{
			Name:            "assign-fusedsubstring-" + k.Name,
			Kind:            "assignment",
			Text:            k.Key + "=" + val,
			Secret:          val,
			SelfContained:   false,
			AssignmentValue: val,
			AssignmentKey:   k.Key,
		})
		valShort := fakeLowerBody(600+i, 10)
		out = append(out, secretShape{
			Name:            "assign-fusedsubstring-short-nodigit-" + k.Name,
			Kind:            "assignment",
			Text:            k.Key + "=" + valShort,
			Secret:          valShort,
			SelfContained:   false,
			AssignmentValue: valShort,
			AssignmentKey:   k.Key,
		})
	}
	return out
}

// pemLabelsUnderTest are the private-key labels a PEM block may carry.
// CERTIFICATE and PUBLIC KEY are deliberately excluded: they are
// allow-listed non-secrets (pemLabelIsDenied) and their survival is
// correct, not a leak.
var pemLabelsUnderTest = []string{
	"RSA PRIVATE KEY",
	"PRIVATE KEY",
	"EC PRIVATE KEY",
	"OPENSSH PRIVATE KEY",
	"DSA PRIVATE KEY",
	"ENCRYPTED PRIVATE KEY",
	"PGP PRIVATE KEY BLOCK",
}

// pemBodyVariant describes one way a PEM block's body can be shaped. The
// non-base64 variants are not exotic: `openssl pkcs12 -nodes` really does
// emit "Key Attributes:" and "Bag Attributes:" lines inside the block, and
// a terminal- or word-wrapped paste really does put spaces and punctuation
// inside a body line. Each variant is a place a naive "stop at the first
// line that doesn't look like base64" scan truncates the span while still
// reporting a hit.
type pemBodyVariant struct {
	Name string
	// Body returns the body lines (each without a trailing newline) and the
	// substring of them that must not survive.
	Body func(seed int) (lines []string, secret string)
}

func pemBodyVariants() []pemBodyVariant {
	return []pemBodyVariant{
		{"short-base64", func(seed int) ([]string, string) {
			l := []string{fakeB64Body(seed, 64), fakeB64Body(seed+1, 64), fakeB64Body(seed+2, 20) + "=="}
			return l, l[len(l)-1]
		}},
		{"long-base64", func(seed int) ([]string, string) {
			var l []string
			for i := 0; i < 30; i++ {
				l = append(l, fakeB64Body(seed+i, 64))
			}
			return l, l[len(l)-1]
		}},
		{"rfc1421-headers", func(seed int) ([]string, string) {
			l := []string{
				"Proc-Type: 4,ENCRYPTED",
				"DEK-Info: AES-128-CBC," + fakeUpperBody(seed, 32),
				"",
				fakeB64Body(seed, 64),
				fakeB64Body(seed+1, 40) + "=",
			}
			return l, l[len(l)-1]
		}},
		{"key-attributes-line", func(seed int) ([]string, string) {
			l := []string{
				fakeB64Body(seed, 64),
				"Key Attributes: <No Attributes>",
				fakeB64Body(seed+1, 64),
				fakeB64Body(seed+2, 24) + "==",
			}
			return l, l[len(l)-1]
		}},
		{"bag-attributes-line", func(seed int) ([]string, string) {
			l := []string{
				"Bag Attributes",
				"    localKeyID: 01 02 03 04",
				fakeB64Body(seed, 64),
				fakeB64Body(seed+1, 24) + "==",
			}
			return l, l[len(l)-1]
		}},
		{"wrapped-paste-with-space", func(seed int) ([]string, string) {
			l := []string{
				fakeB64Body(seed, 30) + " " + fakeB64Body(seed+1, 30),
				fakeB64Body(seed+2, 24) + "==",
			}
			return l, l[len(l)-1]
		}},
		{"blank-line-in-body", func(seed int) ([]string, string) {
			l := []string{fakeB64Body(seed, 64), "", fakeB64Body(seed+1, 24) + "=="}
			return l, l[len(l)-1]
		}},
	}
}

// pemShapes returns PEM block shapes across labels and body variants,
// including truncated blocks (no END marker) whose body must still be
// redacted.
func pemShapes() []secretShape {
	var out []secretShape
	variants := pemBodyVariants()
	seed := 200
	for _, label := range pemLabelsUnderTest {
		for _, v := range variants {
			seed++
			lines, secret := v.Body(seed)
			body := strings.Join(lines, "\n")
			text := fmt.Sprintf("-----BEGIN %s-----\n%s\n-----END %s-----", label, body, label)
			out = append(out, secretShape{
				Name:            "pem-" + strings.ReplaceAll(strings.ToLower(label), " ", "-") + "-" + v.Name,
				Kind:            "pem",
				Text:            text,
				Secret:          secret,
				SelfContained:   true,
				AssignmentValue: text,
				AssignmentKey:   "private_key",
			})
		}
	}
	// Truncated blocks: no END marker anywhere. The body must still be
	// redacted through the last body-shaped line.
	for _, v := range variants {
		seed++
		lines, secret := v.Body(seed)
		text := "-----BEGIN RSA PRIVATE KEY-----\n" + strings.Join(lines, "\n")
		out = append(out, secretShape{
			Name:            "pem-truncated-" + v.Name,
			Kind:            "pem",
			Text:            text,
			Secret:          secret,
			SelfContained:   true,
			AssignmentValue: text,
			AssignmentKey:   "private_key",
		})
	}
	// Over-ceiling truncated block: no END marker anywhere, and the body
	// runs well past pemMaxScanLines (200) -- unlike every other truncated
	// shape above, whose bodies are only a handful of lines. This is the
	// class that used to leak in full while hits still reported 1: only
	// the BEGIN marker was redacted, and every body line past line 200,
	// including the genuinely secret-bearing last line, shipped in the
	// clear. Body lines are uniform, well-formed base64 (never a
	// non-body-shaped line), so nothing here can be mistaken for the
	// deliberately-bounded "stray BEGIN marker in ordinary prose"
	// over-redaction guard the scrubber also has to satisfy.
	{
		seed++
		const overCeilingBodyLines = 250 // comfortably past pemMaxScanLines=200
		var lines []string
		for n := 0; n < overCeilingBodyLines; n++ {
			lines = append(lines, fakeB64Body(seed+n, 64))
		}
		secret := lines[len(lines)-1]
		text := "-----BEGIN RSA PRIVATE KEY-----\n" + strings.Join(lines, "\n") + "\n"
		out = append(out, secretShape{
			Name:            "pem-truncated-over-ceiling-no-end-marker",
			Kind:            "pem",
			Text:            text,
			Secret:          secret,
			SelfContained:   true,
			AssignmentValue: text,
			AssignmentKey:   "private_key",
		})
	}
	return out
}

// allShapes is the full shape axis: vendor formats, generic assignments,
// and PEM blocks.
func allShapes() []secretShape {
	var out []secretShape
	out = append(out, vendorShapes()...)
	out = append(out, genericAssignmentShapes()...)
	out = append(out, wordFallbackAssignmentShapes()...)
	out = append(out, fusedSubstringAssignmentShapes()...)
	out = append(out, pemShapes()...)
	return out
}

// benignFillers are non-secret strings used as surrounding noise. Every one
// must be inert under Scrub (TestScrubFuzz_FillersAreInert enforces this):
// a filler that is itself redacted would mask a leak, because the surviving
// secret check would still pass on an output whose redaction came entirely
// from the filler.
var benignFillers = []string{
	"the quick brown fox",
	"user=alice",
	"status=ok",
	"note: see the runbook",
	"risk-management-framework-review",
	"task-list-item-identifier-value",
	"whisk-and-fold-the-batter",
	"version 1.2.3",
	"disk-usage-report-generated",
}

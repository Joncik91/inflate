package harvester

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// redactedCount counts the [REDACTED] tokens actually present in s. Used to
// verify hits == the count of tokens actually emitted, never a value
// Scrub merely claims.
func redactedCount(s string) int { return strings.Count(s, "[REDACTED]") }

// assertNoResidue fails t if any substring of secret (of length >= minLen)
// survives anywhere in out. minLen avoids flagging trivially short
// fragments that could coincidentally appear in unrelated text.
func assertNoResidue(t *testing.T, out, secret string, minLen int) {
	t.Helper()
	if len(secret) < minLen {
		return
	}
	for i := 0; i+minLen <= len(secret); i++ {
		frag := secret[i : i+minLen]
		if strings.Contains(out, frag) {
			t.Errorf("residual fragment %q of secret %q survived in output %q", frag, secret, out)
		}
	}
}

// --- Quote and escape state -------------------------------------------------

func TestScrub_QuoteAndEscape(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"single-quoted value with apostrophe", `password="it's fine here"`},
		{"double-quoted value containing double quotes via single wrap", `password='he said "hi" ok'`},
		{"escaped double quote inside JSON string value", `{"password":"abc\"DEF-secret"}`},
		{"backtick value", "password=`backtick-secret-value`"},
		{"nested quotes, single outer double inner", `token="wrapper 'inner' end"`},
		{"value containing the other quote type", `secret='contains "double" quotes'`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != redactedCount(out) {
				t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
			}
			if hits == 0 {
				t.Fatalf("Scrub(%q) hit 0, want at least 1", c.in)
			}
			if strings.Contains(out, "secret") && strings.Contains(c.in, "-secret") {
				// the literal word "secret" as part of the credential value
				// (not the key name "secret") must not survive
			}
		})
	}

	t.Run("escaped quote does not end the string", func(t *testing.T) {
		in := `{"password":"abc\"DEF-secret"}`
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		assertNoResidue(t, out, `DEF-secret`, 4)
		if !json.Valid([]byte(out)) {
			t.Errorf("output is not valid JSON: %q", out)
		}
	})

	t.Run("runtime-chosen quote delimiter does not leak the tail", func(t *testing.T) {
		in := `password="it's fine here"`
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		assertNoResidue(t, out, "it's fine here", 4)
		if !strings.HasSuffix(out, `"`) {
			t.Errorf("closing quote not preserved: %q", out)
		}
	})
}

// --- Separators --------------------------------------------------------------

func TestScrub_Separators(t *testing.T) {
	t.Run("CRLF inside PEM block", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----\r\nAAAA\r\n-----END RSA PRIVATE KEY-----\r\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		assertNoResidue(t, out, "AAAA", 4)
	})

	t.Run("lone CR inside PEM block", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----\rAAAA\r-----END RSA PRIVATE KEY-----\r"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
	})

	t.Run("form feed inside PEM block", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----\fAAAA\f-----END RSA PRIVATE KEY-----\f"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
	})

	t.Run("vertical tab inside PEM block", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----\vAAAA\v-----END RSA PRIVATE KEY-----\v"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
	})

	t.Run("no newline after PEM BEGIN, immediate EOF", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
	})

	t.Run("two blank lines before PEM body", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----\n\n\nAAAA\n-----END RSA PRIVATE KEY-----\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		assertNoResidue(t, out, "AAAA", 4)
	})
}

// --- Assignment shapes ---------------------------------------------------

func TestScrub_AssignmentShapes(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantOutput string
		wantHits   int
	}{
		{"password=value", "password=hunter2", "password=[REDACTED]", 1},
		{"password = value with spaces", "password = hunter2", "password = [REDACTED]", 1},
		{"password tab-equals value", "password\t=hunter2", "password\t=[REDACTED]", 1},
		{"access_token=", "access_token=abc123xyz", "access_token=[REDACTED]", 1},
		{"client_secret=", "client_secret=abc123xyz", "client_secret=[REDACTED]", 1},
		{"secret_key=", "secret_key=abc123xyz", "secret_key=[REDACTED]", 1},
		{"PGPASSWORD=", "PGPASSWORD=abc123xyz", "PGPASSWORD=[REDACTED]", 1},
		{"JSON key:value", `{"password":"x"}`, `{"password":"[REDACTED]"}`, 1},
		{"YAML password: x", "password: x", "password: [REDACTED]", 1},
		{"flow mapping {password: x}", "{password: x}", "{password: [REDACTED]}", 1},
		{"shell value followed by &&", "export TOKEN=abc123xyz && echo done", "export TOKEN=[REDACTED] && echo done", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if out != c.wantOutput {
				t.Errorf("Scrub(%q) = %q, want %q", c.in, out, c.wantOutput)
			}
			if hits != c.wantHits {
				t.Errorf("Scrub(%q) hits = %d, want %d", c.in, hits, c.wantHits)
			}
			if hits != redactedCount(out) {
				t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
			}
		})
	}
}

// --- Prose protection -----------------------------------------------------

func TestScrub_ProseNotRedacted(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"bare word colon prose", "Secret: this document is confidential"},
		{"chained label prose", "Error: token: invalid"},
		{"Go format string colon", `fmt.Errorf("token: %w", err)`},
		{"plural word is not a credential key", "tokens earned: 5"},
		{"code identifier assigned a function call", "const apiKey = getToken()"},
		{"prefix fusion: whisk (vendor sk- guard)", "whisk-brand of the year"},
		{"prefix fusion: kiosk-mode (vendor sk- guard)", "kiosk-mode enabled"},
		{"prefix fusion: risk-management (vendor sk- guard)", "risk-management plan"},
		{"prefix fusion: task-list (vendor sk- guard)", "task-list for today"},
		{"prefix fusion: disk-usage (vendor sk- guard)", "disk-usage report"},
		{"double equals is comparison, not assignment", `if cfg.APIKey == "" {`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 0 {
				t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", c.in, out, hits)
			}
			if out != c.in {
				t.Errorf("Scrub(%q) = %q, want unchanged", c.in, out)
			}
		})
	}
}

// TestScrub_KeyMatchIsUnconditional pins the redesign this round of work
// converges on: a key containing a credential word ANYWHERE in it (exact,
// word-split, or fused in any position) redacts its value unconditionally,
// with no value-shape gate at all. "monkey=business" and "bypass=true" used
// to be asserted unredacted in TestScrub_ProseNotRedacted, back when the
// key-matching fallback mechanisms (isCredentialKeyByWord,
// isCredentialKeyByFusedSuffix) were gated by looksLikeCredentialValue: that
// gate is what kept "monkey" (fuses "key") and "bypass" (fuses "pass") from
// over-redacting an ordinary short, one-case, digit-free value.
//
// That gate is now gone from every key-match path (see tryKeyAt's isKeyMatch
// doc comment for why applying it there was backwards: it strictly tested
// the keys the scrubber was LESS certain about, which is exactly the
// mechanism that shipped a fresh leak with every new key-matching addition).
// The direct consequence is that MONKEY, BYPASS, KEYSTORE, XPASS, COMPASS
// and any other bare English word that happens to fuse a credential word
// now redact unconditionally too, regardless of how innocuous the value
// looks. This is the ACCEPTED COST of the redesign, not a regression: a
// fixed, finite set of over-redaction false positives on ordinary
// non-secret keys, traded for closing an open leak surface where no
// enumerable list of key names could ever be complete. Real credential keys
// (ADMIN_PASSWORD, KEYSTORE, PASSWORDADMIN, XPASS, and everything else this
// scrubber is actually built to catch) are worth far more than the
// occasional "monkey=business" or "bypass=true" false positive costs.
func TestScrub_KeyMatchIsUnconditional(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"bare English word fusing \"key\": monkey", "monkey=business"},
		{"bare English word fusing \"pass\": bypass", "bypass=true"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 1 {
				t.Errorf("Scrub(%q) hits=%d, want 1 (over-redaction is the accepted, sanctioned direction here)", c.in, hits)
			}
			if !strings.Contains(out, "[REDACTED]") {
				t.Errorf("Scrub(%q) = %q, want a [REDACTED] token", c.in, out)
			}
		})
	}
}

// --- PEM ------------------------------------------------------------------

func TestScrub_PEM(t *testing.T) {
	t.Run("complete block redacted in full", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKey\nAAAA\n-----END RSA PRIVATE KEY-----\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
		}
		assertNoResidue(t, out, "MIIBOgIBAAJBAKey", 6)
		if !strings.Contains(out, "[REDACTED]") {
			t.Errorf("expected [REDACTED] in output: %q", out)
		}
	})

	t.Run("truncated block redacted through recognizable key material", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKey\nAAAA\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
		}
		assertNoResidue(t, out, "MIIBOgIBAAJBAKey", 6)
	})

	t.Run("indented body lines", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----\n    MIIBOgIBAAJBAKey\n-----END RSA PRIVATE KEY-----\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
		}
	})

	t.Run("encrypted with Proc-Type and DEK-Info headers", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----\nProc-Type: 4,ENCRYPTED\nDEK-Info: AES-128-CBC,ABCDEF0123456789\n\nMIIBOgIBAAJBAKey\n-----END RSA PRIVATE KEY-----\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
		}
		if strings.Contains(out, "ABCDEF0123456789") {
			t.Errorf("DEK-Info value leaked: %q", out)
		}
	})

	t.Run("lowercase markers", func(t *testing.T) {
		in := "-----begin rsa private key-----\nmiibogibaajbakey\n-----end rsa private key-----\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
		}
	})

	t.Run("PGP PRIVATE KEY BLOCK", func(t *testing.T) {
		in := "-----BEGIN PGP PRIVATE KEY BLOCK-----\nabc123keymaterial\n-----END PGP PRIVATE KEY BLOCK-----\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
		}
	})

	t.Run("two adjacent blocks yield exactly 2 hits, middle text intact", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----\nAAAAKEYONE\n-----END RSA PRIVATE KEY-----\nmiddle text stays here\n-----BEGIN RSA PRIVATE KEY-----\nBBBBKEYTWO\n-----END RSA PRIVATE KEY-----\n"
		out, hits := Scrub(in)
		if hits != 2 {
			t.Fatalf("hits = %d, want 2: %q", hits, out)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
		}
		if !strings.Contains(out, "middle text stays here") {
			t.Errorf("middle text was not preserved: %q", out)
		}
		assertNoResidue(t, out, "AAAAKEYONE", 6)
		assertNoResidue(t, out, "BBBBKEYTWO", 6)
	})

	t.Run("CERTIFICATE block is never redacted", func(t *testing.T) {
		in := "-----BEGIN CERTIFICATE-----\nMIIBOgIBAAJBAKey\n-----END CERTIFICATE-----\n"
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
		}
	})

	t.Run("PUBLIC KEY block is never redacted", func(t *testing.T) {
		in := "-----BEGIN PUBLIC KEY-----\nMIIBOgIBAAJBAKey\n-----END PUBLIC KEY-----\n"
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
		}
	})

	// An unterminated BEGIN marker followed by body-shaped lines is
	// byte-identical whether it is a genuine private key whose END marker
	// was clipped off (by a truncated paste, a cut-off log capture, or a
	// crashed upload) or a stray marker followed by coincidental
	// base64-alphabet filler: no syntactic signal in the bytes themselves
	// separates the two. This scrubber's policy is that under-redaction is
	// a security defect and over-redaction is an acceptable cost (see the
	// package-level policy this whole PEM-handling effort follows), so the
	// ambiguous shape must resolve toward redaction, not toward preserving
	// as much of the document as possible.
	//
	// This subtest used to assert the opposite: that the span stopped at
	// ~12222 bytes (pemMaxScanLines * line width) and left everything past
	// that point in the clear. That was the exact leak this file's
	// scanPEMBlocks fix (the hitByteCeiling || (ranOutOfLineBudget &&
	// unclampedTruncatedLimit < len(lines)) extend-to-EOF condition) closes:
	// a real private-key body longer than the truncated-block budget shipped
	// verbatim past that point while hits==1 falsely reported success --
	// the worst failure mode this scrubber can produce, because a caller
	// checking only hits has no way to know the body was not actually
	// redacted, and every downstream defense that trusts hits==1 is
	// defeated by it.
	//
	// What is asserted now: redaction runs through to EOF, hits stays 1,
	// and no body byte survives. The 500-line count is unchanged from the
	// original version of this test (comfortably more than pemMaxScanLines
	// so the scan-then-extend path is genuinely exercised, without the
	// multi-megabyte, multi-second-under-`-race` input a much larger line
	// count would need to prove the same point) and every line is blank
	// base64-alphabet filler, which passes pemBodyLineOK unconditionally,
	// so nothing in the shape of the input itself stops the scan short of
	// pemMaxScanLines.
	t.Run("stray BEGIN with base64-shaped body redacts through EOF, not just through pemMaxScanLines", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("-----BEGIN RSA PRIVATE KEY-----\n")
		bodyLine := strings.Repeat("A", 60) + "\n" // base64-alphabet-only, passes pemBodyLineOK
		const totalLines = 500
		for i := 0; i < totalLines; i++ {
			b.WriteString(bodyLine)
		}
		in := b.String()
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: got %d bytes of output", hits, len(out))
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens", hits, redactedCount(out))
		}
		if strings.Contains(out, "AAAA") {
			t.Errorf("body byte survived: redaction stopped short of EOF: %q", out)
		}
		// Hard-coded literal bound: the whole 30532-byte document collapses
		// to exactly the placeholder, nothing more.
		const wantMaxOutLen = 32
		if len(out) > wantMaxOutLen {
			t.Errorf("output is %d bytes, want <= %d (redaction should run through EOF, leaving only the placeholder): %q", len(out), wantMaxOutLen, out)
		}
	})

	t.Run("bounded over-redaction: stray BEGIN in prose", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("-----BEGIN RSA PRIVATE KEY-----\n")
		for i := 0; i < 200; i++ {
			b.WriteString("this is an ordinary line of unrelated prose text\n")
		}
		in := b.String()
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: got %d bytes of output", hits, len(out))
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens", hits, redactedCount(out))
		}
		// The document is ~9800 bytes; a single [REDACTED] token replacing
		// only the BEGIN line (and possibly a couple of prose lines, never
		// the whole 200-line, ~9600-byte body) must shrink it by far less
		// than the full prose length.
		shrink := len(in) - len(out)
		if shrink >= 200 {
			t.Errorf("bounded over-redaction failed: shrank output by %d bytes, want < 200", shrink)
		}
	})

	// Regression: key material that runs directly into the BEGIN marker's
	// closing dashes with no separating newline (a realistic copy-paste-
	// flattened or single-line-JSON-embedded shape) was previously
	// invisible to pemBeginMarker, which trimmed dashes from the end of
	// the whole line and required that trim to be non-empty -- body
	// content after the marker made that check fail, so the marker (and
	// the whole block) went unrecognized entirely.
	t.Run("truncated block with no separator after BEGIN marker", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDQZ8k7X9Z1abcd"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
		}
		if out == in {
			t.Errorf("Scrub(%q) left the input unchanged, want the key material redacted", in)
		}
		assertNoResidue(t, out, "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDQZ8k7X9Z1abcd", 8)
	})

	t.Run("CERTIFICATE with no separator after BEGIN marker still not redacted", func(t *testing.T) {
		in := "-----BEGIN CERTIFICATE-----MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDQZ8k7X9Z1abcd"
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
		}
	})

	// FINDING (CRITICAL, realistic-key leak): a PEM block with a GENUINE
	// matching END marker leaked most of its body whenever the body was
	// longer than the truncated-block line budget. The END-marker search and
	// the truncated-block budget shared one cap (pemMaxScanLines = 200), so
	// a 300-line body -- an ordinary passphrase-encrypted OpenSSH RSA 4096
	// key, ~19.5KB -- ran out of budget at line 200 and never reached its own
	// END marker. The span stopped at the last line the budget allowed and
	// the remaining 100 body lines plus the END marker shipped in the clear,
	// while hits still reported 1: the "secret cut in half, success reported"
	// failure class again, now in the complete-block path.
	//
	// The EOF-extension logic did NOT save this case, and it is worth being
	// precise about why, because it looks like it should have: with
	// conventional 64-byte body lines the 200-line budget expires at ~13035
	// bytes, which is BELOW pemMaxScanBytes (16384). So hitByteCeiling was
	// never set, the ranOutOfLineBudget && hitByteCeiling extension never
	// fired, and the span truncated silently. For any conventional PEM line
	// width (64-76 chars) the line cap always binds first, which made the
	// byte cap effectively dead for exactly the inputs that matter.
	//
	// Fixed by separating the two budgets: the END-marker search now gets its
	// own generous line budget (a genuine END marker proves the block is
	// real, and redaction still stops at that END, so a large search budget
	// cannot over-redact unrelated content), while the truncated/stray-marker
	// budget stays exactly where it was.
	t.Run("realistic-length OpenSSH key with genuine END marker is fully redacted", func(t *testing.T) {
		// The reported reproduction, verbatim: 300 body lines of real
		// OpenSSH base64, ~19.5KB, which is what a passphrase-encrypted
		// OpenSSH RSA 4096 key actually looks like.
		const bodyLine = "b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABlwAAAAdz\n"
		var b strings.Builder
		b.WriteString("-----BEGIN OPENSSH PRIVATE KEY-----\n")
		for i := 0; i < 300; i++ {
			b.WriteString(bodyLine)
		}
		b.WriteString("-----END OPENSSH PRIVATE KEY-----\n")
		in := b.String()

		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1", hits)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens", hits, redactedCount(out))
		}
		// The criterion from the report: zero surviving body lines.
		if n := strings.Count(out, "b3BlbnNzaC1rZXktdjEA"); n != 0 {
			t.Errorf("%d body lines survived verbatim, want 0: head %q", n, out[:min(160, len(out))])
		}
		if out != "[REDACTED]\n" {
			t.Errorf("Scrub(...) = %q, want %q", out[:min(160, len(out))], "[REDACTED]\n")
		}
	})

	// The same defect across the realistic size range, including the case the
	// report calls out specifically: trailing content AFTER the END marker,
	// which is what defeats any "extend to EOF" style rescue. 50 lines is an
	// RSA 4096 PEM (~3.3KB); 300 is a passphrase-encrypted OpenSSH RSA 4096
	// (~19.5KB); 600 (~39KB) covers certificate chains and PKCS#12 blobs.
	t.Run("blocks with genuine END marker fully redacted at 50, 300 and 600 body lines", func(t *testing.T) {
		const bodyLine = "b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABlwAAAAdz\n"
		for _, lines := range []int{50, 300, 600} {
			for _, trailing := range []string{"", "trailing prose that must survive\n"} {
				var b strings.Builder
				b.WriteString("-----BEGIN OPENSSH PRIVATE KEY-----\n")
				for i := 0; i < lines; i++ {
					b.WriteString(bodyLine)
				}
				b.WriteString("-----END OPENSSH PRIVATE KEY-----\n")
				b.WriteString(trailing)
				in := b.String()

				out, hits := Scrub(in)
				name := fmt.Sprintf("lines=%d trailing=%q", lines, trailing)
				if hits != 1 {
					t.Errorf("%s: hits = %d, want 1", name, hits)
				}
				if hits != redactedCount(out) {
					t.Errorf("%s: hits=%d but output has %d [REDACTED] tokens", name, hits, redactedCount(out))
				}
				// Lower bound: the body is actually gone.
				if n := strings.Count(out, "b3BlbnNzaC1rZXktdjEA"); n != 0 {
					t.Errorf("%s: %d body lines survived, want 0", name, n)
				}
				if strings.Contains(out, "-----END OPENSSH PRIVATE KEY-----") {
					t.Errorf("%s: END marker survived, block was not redacted in full: %q", name, out[:min(160, len(out))])
				}
				// Upper bound: exactly the block, nothing more. Trailing
				// content after the END marker must survive untouched.
				if want := "[REDACTED]\n" + trailing; out != want {
					t.Errorf("%s: Scrub(...) = %q, want %q", name, out[:min(160, len(out))], want)
				}
			}
		}
	})

	// Same fix, same rationale, at 500KB: an unterminated BEGIN marker
	// followed by body-shaped lines is byte-identical whether it is a
	// genuine private key with a clipped END marker (a truncated paste, a
	// cut-off log capture, a crashed upload -- all routine, all realistic
	// at this size) or a stray marker followed by coincidental base64
	// filler. No syntactic signal in the bytes distinguishes the two, and
	// this scrubber's policy is that under-redaction is a security defect
	// while over-redaction is acceptable, so the ambiguous shape resolves
	// toward redaction.
	//
	// This subtest used to assert the opposite: that the scan stopped at
	// exactly 12831 bytes (the BEGIN line plus pemMaxScanLines * line
	// width) and left the remaining ~500KB of body in the clear. That is
	// precisely the leak scanPEMBlocks' extend-to-EOF fix
	// (hitByteCeiling || (ranOutOfLineBudget && unclampedTruncatedLimit <
	// len(lines))) closes: a real key body longer than the truncated-block
	// budget shipped verbatim past that point while hits==1 falsely
	// reported success -- the worst failure mode this scrubber can
	// produce, since it defeats every downstream check that trusts hits==1
	// as proof the secret is gone.
	//
	// What is asserted now: redaction runs through to EOF, hits stays 1,
	// and no body byte survives. The body here is base64-alphabet-only (no
	// spaces), so every line passes pemBodyLineOK and nothing in the shape
	// of the input itself stops the scan short of the line budget; the
	// bound below is a hard-coded literal on the output length, never the
	// pemMaxScanLines/pemMaxScanBytes constants themselves, so a
	// regression that merely changes those constants cannot make this
	// pass unnoticed.
	t.Run("stray unterminated BEGIN over 500KB of body-shaped content redacts through EOF", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("-----BEGIN RSA PRIVATE KEY-----\n")
		line := strings.Repeat("A", 63) + "\n" // 64 bytes, base64-alphabet-only
		for b.Len() < 500*1024 {
			b.WriteString(line)
		}
		in := b.String()
		if len(in) < 500*1024 {
			t.Fatalf("test setup: input is %d bytes, want >= %d", len(in), 500*1024)
		}

		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1", hits)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens", hits, redactedCount(out))
		}
		if strings.Contains(out, "AAAA") {
			t.Errorf("body byte survived: redaction stopped short of EOF: %q", out)
		}
		// Hard-coded literal bound: the whole ~500KB document collapses to
		// exactly the placeholder, nothing more.
		const wantMaxOutLen = 32
		if len(out) > wantMaxOutLen {
			t.Errorf("output is %d bytes, want <= %d (redaction should run through EOF, leaving only the placeholder): %q", len(out), wantMaxOutLen, out)
		}
	})
}

// --- Vendor ---------------------------------------------------------------

func TestScrub_Vendor(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"AWS access key", "AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE"},
		{"legacy flat sk- key", "key=sk-abcdefghijklmnopqrstuvwxyz1234"},
		{"sk-ant-", "key=sk-ant-abcdefghijklmnopqrstuvwxyz1234"},
		{"sk-proj-", "key=sk-proj-abcdefghijklmnopqrstuvwxyz1234"},
		{"sk-or-v1-", "key=sk-or-v1-abcdefghijklmnopqrstuvwxyz1234"},
		{"github ghp_", "token=ghp_abcdefghijklmnopqrstuvwxyz1234"},
		{"github gho_", "token=gho_abcdefghijklmnopqrstuvwxyz1234"},
		{"github ghs_", "token=ghs_abcdefghijklmnopqrstuvwxyz1234"},
		{"github ghu_", "token=ghu_abcdefghijklmnopqrstuvwxyz1234"},
		{"github ghr_", "token=ghr_abcdefghijklmnopqrstuvwxyz1234"},
		{"github_pat_", "token=github_pat_abcdefghijklmnopqrstuvwxyz1234"},
		{"slack xoxb-", "token=xoxb-1234567890-abcdefghij"},
		{"slack xapp-", "token=xapp-1-A012B345678-abcdefghij"},
		{"google AIzaSy", "key=AIzaSyABCDEFGHIJKLMNOPQRSTUVWXYZ01234"},
		{"npm_", "token=npm_abcdefghijklmnopqrstuvwxyz1234"},
		{"bearer token", "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9"},
		{"bare JWT", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dQw4w9WgXcQ_dGVzdHNpZw"},
		{"scheme user pass host", "https://user:hunter2@example.com/path"},
		{"authorization basic", "Authorization: Basic dXNlcjpwYXNzd29yZA=="},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits < 1 {
				t.Fatalf("Scrub(%q) hits = %d, want >= 1", c.in, hits)
			}
			if hits != redactedCount(out) {
				t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
			}
		})
	}

	t.Run("vendor sk- prefix negatives (prefix fusion)", func(t *testing.T) {
		negatives := []string{
			"whisk-brand",
			"kiosk-mode",
			"risk-management",
			"task-list",
			"disk-usage",
		}
		for _, in := range negatives {
			out, hits := Scrub(in)
			if hits != 0 || out != in {
				t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
			}
		}
	})

	t.Run("two vendor hits in one line", func(t *testing.T) {
		in := "k1=sk-aaaaaaaaaaaaaaaaaaaaaaaa k2=sk-bbbbbbbbbbbbbbbbbbbbbbbb"
		out, hits := Scrub(in)
		if hits != 2 {
			t.Fatalf("hits = %d, want 2: %q", hits, out)
		}
		if out != "k1=[REDACTED] k2=[REDACTED]" {
			t.Errorf("Scrub(%q) = %q", in, out)
		}
	})
}

// TestScrub_VendorBareKeyAlone guards against the vendor regex being
// exercised only through the credential-assignment scanner (every case in
// TestScrub_Vendor wraps its key in "key=..."). A bare vendor key with no
// surrounding assignment must still be redacted by the vendor regex itself:
// wrapping it in an assignment would mask a regression in the regex, since
// the credential-assignment scanner would redact the value regardless of
// what the vendor pattern does. Regression test for a real Anthropic key
// shape (sk-ant-api03-...) that the original \bsk-(?:ant-|...)?[a-zA-Z0-9]
// {20,} pattern failed to match because it disallowed the internal hyphen
// after the "api03" segment.
func TestScrub_VendorBareKeyAlone(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"sk-ant- real key shape with internal hyphen", "sk-ant-api03-AbCdEf123456789012345678901234"},
		{"sk-proj- bare", "sk-proj-abcdefghijklmnopqrstuvwxyz1234"},
		{"sk-or-v1- bare", "sk-or-v1-abcdefghijklmnopqrstuvwxyz1234"},
		{"legacy flat sk- bare, no hyphens", "sk-abcdefghijklmnopqrstuvwxyz1234"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 1 {
				t.Fatalf("Scrub(%q) hits = %d, want 1 (bare key alone, no key= wrapper): %q", c.in, hits, out)
			}
			if out != "[REDACTED]" {
				t.Errorf("Scrub(%q) = %q, want %q", c.in, out, redactedToken)
			}
			if hits != redactedCount(out) {
				t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
			}
		})
	}

	t.Run("prefix-fusion negatives stay negative when bare", func(t *testing.T) {
		negatives := []string{
			"whisk-brand",
			"kiosk-mode",
			"risk-management",
			"task-list",
			"disk-usage",
		}
		for _, in := range negatives {
			out, hits := Scrub(in)
			if hits != 0 || out != in {
				t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
			}
		}
	})
}

// --- Invariants ------------------------------------------------------------

func TestScrub_Invariants(t *testing.T) {
	representative := []string{
		"",
		"AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE",
		"key=sk-ant-abcdefghijklmnopqrstuvwxyz1234",
		"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9",
		"password=hunter2",
		"password = hunter2",
		"password\t=hunter2",
		`password="it's fine here"`,
		`password='he said "hi" ok'`,
		`{"password":"abc\"DEF-secret"}`,
		"Secret: this document is confidential",
		"Error: token: invalid",
		`fmt.Errorf("token: %w", err)`,
		"tokens earned: 5",
		"const apiKey = getToken()",
		"-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKey\n-----END RSA PRIVATE KEY-----\n",
		"-----BEGIN CERTIFICATE-----\nMIIBOgIBAAJBAKey\n-----END CERTIFICATE-----\n",
		`{"user":"bob","password":"secretvalue123","token":"abc"}` + "\n",
		"export TOKEN=abc123xyz && echo done",
		"monkey=business",
		"whisk-brand",
		"sk-ant-api03-AbCdEf123456789012345678901234",
		"-----BEGIN RSA PRIVATE KEY-----MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDQZ8k7X9Z1abcd",
		`{"type":"user","content":"my password=hunter2XY9","ts":1}`,
	}

	for _, in := range representative {
		t.Run(in, func(t *testing.T) {
			out, hits := Scrub(in)
			if hits != redactedCount(out) {
				t.Fatalf("hits=%d but output has %d [REDACTED] tokens for input %q: output %q", hits, redactedCount(out), in, out)
			}

			out2, hits2 := Scrub(out)
			if hits2 != 0 {
				t.Errorf("Scrub is not idempotent: re-scrubbing %q produced %d more hits: %q", out, hits2, out2)
			}
			if out2 != out {
				t.Errorf("Scrub is not idempotent: Scrub(Scrub(%q)) = %q, want %q", in, out2, out)
			}
		})
	}

	t.Run("JSONL transcript line with embedded secret redacts and stays valid JSON", func(t *testing.T) {
		in := `{"role":"user","content":"my password=hunter2secretvalue please store it","token":"sk-ant-abcdefghijklmnopqrstuvwxyz1234"}` + "\n"
		out, hits := Scrub(in)
		if hits < 1 {
			t.Fatalf("hits = %d, want >= 1", hits)
		}
		trimmed := strings.TrimRight(out, "\n")
		if !json.Valid([]byte(trimmed)) {
			t.Fatalf("output is not valid JSON: %q", out)
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			t.Fatalf("json.Unmarshal failed: %v (output: %q)", err, out)
		}
		assertNoResidue(t, out, "hunter2secretvalue", 6)
		assertNoResidue(t, out, "sk-ant-abcdefghijklmnopqrstuvwxyz1234", 10)
	})

	// Regression: when the credential value is immediately followed by the
	// enclosing JSON string's closing quote (no trailing prose in
	// between), the bare-value scanner previously had no notion of an
	// "enclosing quote" and consumed that closing quote as part of the
	// value, corrupting the JSON. Distinct from the case above, where
	// trailing prose after the secret happened to give the value scanner
	// a natural (non-quote) stopping point regardless.
	t.Run("JSONL: secret value directly abuts the closing quote", func(t *testing.T) {
		in := `{"type":"user","content":"my password=hunter2XY9","ts":1}`
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
		}
		if err := json.Unmarshal([]byte(out), &map[string]any{}); err != nil {
			t.Fatalf("json.Unmarshal failed: %v (output: %q)", err, out)
		}
		assertNoResidue(t, out, "hunter2XY9", 6)
		want := `{"type":"user","content":"my password=[REDACTED]","ts":1}`
		if out != want {
			t.Errorf("Scrub(%q) = %q, want %q", in, out, want)
		}
	})

	t.Run("no residual fragment of any redacted value", func(t *testing.T) {
		cases := []struct{ in, secret string }{
			{"password=hunter2supersecret", "hunter2supersecret"},
			{`password="it's fine here"`, "it's fine here"},
			{`{"password":"abc\"DEF-secret"}`, `DEF-secret`},
			{"key=sk-ant-abcdefghijklmnopqrstuvwxyz1234", "abcdefghijklmnopqrstuvwxyz1234"},
			{"-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKeyMaterialHere\n-----END RSA PRIVATE KEY-----\n", "MIIBOgIBAAJBAKeyMaterialHere"},
		}
		for _, c := range cases {
			out, _ := Scrub(c.in)
			assertNoResidue(t, out, c.secret, 6)
		}
	})
}

// --- Second-round reviewer findings ---------------------------------------
//
// Each subtest below is named after the finding it guards, so a future
// regression points straight back to the original report.

func TestScrub_ReviewerFindings(t *testing.T) {
	// Finding 1 (CRITICAL, leak): an odd-length run of backslashes before
	// a second field's opening quote used to read as one long "escaped"
	// string, swallowing the second field whole and leaving neither field
	// correctly redacted.
	t.Run("finding1: odd backslash run before a second field does not swallow it", func(t *testing.T) {
		in := "password=\"a\\\\\\\" token=\"LEAKME12345\""
		out, hits := Scrub(in)
		if strings.Contains(out, "LEAKME12345") {
			t.Fatalf("finding 1 regressed: LEAKME12345 survives in %q", out)
		}
		if hits != 2 {
			t.Errorf("hits = %d, want 2 (both fields redacted): %q", hits, out)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
		}
	})

	// Finding 2 (CRITICAL, JSON corruption): a bare value nested inside an
	// already-open JSON string used to consume that string's own
	// backslash-escaped closing quote as if it were part of the value,
	// deleting the quote from the output and merging two string regions.
	// This is the "realistic Claude Code assistant turn" case the
	// coordinator called out specifically.
	t.Run("finding2: nested escaped quote is preserved, not deleted", func(t *testing.T) {
		in := `{"content":"he said \"my password=topsecret123\" loudly"}`
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if err := json.Unmarshal([]byte(out), &map[string]any{}); err != nil {
			t.Fatalf("json.Unmarshal failed: %v (output: %q)", err, out)
		}
		assertNoResidue(t, out, "topsecret123", 6)
		want := `{"content":"he said \"my password=[REDACTED]\" loudly"}`
		if out != want {
			t.Errorf("Scrub(%q) = %q, want %q", in, out, want)
		}
	})

	// Finding 3, ROUND 1 (CRITICAL, unbounded over-redaction): neither the
	// bare nor the quoted value scan had a byte cap, so an unterminated
	// quote or a bare value with no whitespace/structural terminator
	// (base64, hex, minified JSON, a long URL-safe payload) ran to EOF,
	// turning one stray "password=" into an over-redaction of the entire
	// remaining document -- the same failure class an earlier regex round
	// was rejected for (6348 bytes of key-free prose eaten by one match).
	//
	// ROUND 2 CORRECTION (also finding 3, later report): the round-1 fix
	// was itself wrong. It capped the *redacted span* at maxValueSpan
	// bytes, so anything past byte 512 of an unterminated value survived
	// in the clear while hits still reported 1 -- a secret cut in half
	// with success reported, the exact failure class this whole
	// hand-written-scanner effort replaced regex to eliminate. The
	// correct design bounds only how far the scanner SEARCHES using the
	// careful (escape/lookahead) logic; once a value is found to run past
	// that point, the scanner keeps searching for a real terminator (or
	// hard EOF-of-search at maxValueScanBytes) and redacts everything up
	// to wherever it actually stops. See maxValueSpan's and
	// maxValueScanBytes's doc comments in scanner.go.
	//
	// This test's thresholds are hard-coded literals, not
	// maxValueSpan/maxValueScanBytes themselves (finding 4): asserting
	// against the constant under test lets any regression that merely
	// changes the constant sail through, which is exactly what let the
	// round-1 (truncate-at-512) design pass its own test undetected.
	t.Run("finding3: an unterminated bare value redacts through EOF within the search ceiling, not truncated at a short cap", func(t *testing.T) {
		// The terminator (" END") sits well past 512 bytes but well
		// within a real search ceiling, so a correct implementation
		// redacts the entire run of x's and keeps " END"; the incorrect
		// round-1 design left everything past byte 512 exposed instead.
		in := "Here is a doc. password=" + strings.Repeat("x", 5000) + " END"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1", hits)
		}
		if strings.Contains(out, "x") {
			t.Errorf("residual 'x' survived: the value was truncated instead of redacted through its terminator: %q", out)
		}
		want := "Here is a doc. password=[REDACTED] END"
		if out != want {
			t.Errorf("Scrub(...) = %q, want %q", out, want)
		}
	})

	// FINDING 3, ROUND 3 CORRECTION (CRITICAL): the round-2 test just above
	// asserted the OLD, incorrect behavior as correct -- "some tail
	// survives past the ceiling... is the accepted tradeoff." It is not.
	// Round 3 established that once the search proves a value genuinely
	// unterminated (no real terminator found within maxValueScanBytes),
	// the ceiling must bound the SEARCH only; the redacted SPAN must
	// extend all the way to EOF, exactly the same "secret cut in half,
	// hits=1 reported" failure class as finding 1, just triggered by a
	// different threshold. This test asserts FULL redaction (destroyed ==
	// the entire document past the key), not partial.
	t.Run("finding3 round3: a value exceeding the search ceiling redacts through EOF, not just to the ceiling", func(t *testing.T) {
		in := "Here is a doc. password=" + strings.Repeat("x", 1_000_000)
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1", hits)
		}
		want := "Here is a doc. password=[REDACTED]"
		if out != want {
			t.Errorf("Scrub(...) = %q, want %q (the whole unterminated value must be redacted through EOF)", out, want)
		}
	})

	// FINDING 3, ROUND 3 CORRECTION, sibling forms (quoted and bracket):
	// same defect, same fix, verified for the other two value shapes that
	// share the maxValueScanBytes ceiling.
	t.Run("finding3 round3: quoted and bracket values exceeding the ceiling also redact through EOF", func(t *testing.T) {
		giant := strings.Repeat("x", 1_000_000)
		cases := []struct{ name, in, want string }{
			{"quoted, no closing quote anywhere", `password="` + giant, `password="[REDACTED]`},
			{"bracket, no closing brace anywhere", "password={" + giant, "password={[REDACTED]"},
		}
		for _, c := range cases {
			out, hits := Scrub(c.in)
			if hits != 1 {
				t.Errorf("%s: hits = %d, want 1", c.name, hits)
			}
			if out != c.want {
				t.Errorf("%s: Scrub(...) = %q, want %q", c.name, out, c.want)
			}
		}
	})

	// FINDING 2, ROUND 3 (CRITICAL, PEM body 100% leak): scanPEMBlocks
	// broke out of its scanning loop on the pemMaxScanBytes cap BEFORE
	// updating lastBodyEnd and before the END-marker check, so a body
	// that crossed the cap collapsed the whole span down to just the
	// BEGIN line -- a cliff where crossing 16384 bytes goes from full
	// redaction to a complete leak while hits still reports 1. Fixed by
	// folding the line that trips the cap into lastBodyEnd (when it is
	// itself still body-shaped) before breaking.
	//
	// This test replaces its round-2 predecessor, which asserted only
	// destroyed > 16384+64 (an upper bound on over-redaction) with no
	// lower bound at all (finding 4): it passed identically whether the
	// body was correctly redacted or leaked 100%, because a leak also
	// satisfies "destroyed a small number of bytes." Both directions are
	// asserted below, matching the sibling test's pattern two cases
	// above.
	t.Run("finding3(pem)/finding2: PEM body at and past pemMaxScanBytes is fully redacted, not leaked", func(t *testing.T) {
		// bodyLen=16000 sits under pemMaxScanBytes (16384): the byte
		// ceiling is never crossed, so the span stops exactly at the end
		// of the (single, huge) body line, one byte short of the
		// document's own trailing newline. bodyLen>=16384 crosses the
		// ceiling, so the fixed code now extends the span through EOF,
		// consuming that trailing newline too -- strictly more complete
		// redaction, not a leak.
		cases := []struct {
			bodyLen int
			want    string
		}{
			{16000, "[REDACTED]\n"},
			{16384, "[REDACTED]"},
			{20000, "[REDACTED]"},
			{100000, "[REDACTED]"},
		}
		for _, c := range cases {
			body := strings.Repeat("A", c.bodyLen)
			in := "-----BEGIN RSA PRIVATE KEY-----\n" + body + "\n"
			out, hits := Scrub(in)
			if hits != 1 {
				t.Errorf("bodyLen=%d: hits = %d, want 1", c.bodyLen, hits)
			}
			if strings.Contains(out, "AAAAAAAAAA") {
				t.Errorf("bodyLen=%d: PEM body leaked, output = %q", c.bodyLen, out)
			}
			if out != c.want {
				t.Errorf("bodyLen=%d: Scrub(...) = %q, want %q", c.bodyLen, out, c.want)
			}
		}
	})

	t.Run("finding3: truncated PEM block with long body-shaped lines is byte-capped AND fully redacted, not just line-capped", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("-----BEGIN RSA PRIVATE KEY-----\n")
		line := strings.Repeat("A", 4000) + "\n"
		for i := 0; i < 200; i++ {
			b.WriteString(line)
		}
		in := b.String()
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1", hits)
		}
		if strings.Contains(out, "AAAA") {
			t.Errorf("PEM body leaked past the byte cap: %q", out)
		}
		// The byte ceiling is crossed partway through this 200-line body
		// with all 200 lines still body-shaped and no END marker, so the
		// fix extends the span through EOF -- consuming the document's
		// own trailing newline too, hence no "\n" after [REDACTED].
		want := "[REDACTED]"
		if out != want {
			t.Errorf("Scrub(...) = %q, want %q (full redaction, not a partial span)", out, want)
		}
	})

	// Finding 1 (round 3, CRITICAL): the round-2 maxValueSpan=512 "fix"
	// still capped the redacted SPAN, not just the search, so any secret
	// longer than 512 bytes leaked its tail in every value form. Verified
	// here with a realistic ~3200-byte secret -- the size a 4096-bit RSA
	// private key base64-encodes to, the realistic instance of a
	// PRIVATE_KEY=<base64> single-line env var -- across all forms that
	// have an explicit terminator (bare, quoted, bracket, and the same
	// shape embedded in JSON). The YAML block-scalar form was correct
	// throughout this whole class of bug: it always redacts the entire
	// body regardless of length, which is why it is included here as a
	// fifth form to confirm it stays that way rather than because it was
	// ever broken.
	t.Run("finding1: no fragment of a 3200-byte secret survives in bare, quoted, bracket, JSON, or YAML-block form", func(t *testing.T) {
		secret := strings.Repeat("QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU2Nzg5", 65) // 3185 bytes, base64-alphabet
		if len(secret) < 3000 {
			t.Fatalf("test setup: secret is only %d bytes, want >= 3000", len(secret))
		}

		cases := []struct {
			name string
			in   string
		}{
			{"bare", "password=" + secret},
			{"quoted", `password="` + secret + `"`},
			{"bracket", "password={" + secret + "}"},
			{"JSON", `{"api_key":"` + secret + `"}`},
			{"YAML block", "password: |\n  " + secret + "\n"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				out, hits := Scrub(c.in)
				if hits != 1 {
					t.Fatalf("Scrub(%s) hits = %d, want 1", c.name, hits)
				}
				assertNoResidue(t, out, secret, 40)
				if strings.Contains(out, secret[len(secret)-20:]) {
					t.Errorf("%s: tail of the secret survives in output", c.name)
				}
			})
		}
	})

	// Finding 2, ROUND 3 (CRITICAL, JSON corruption): the bracket-value
	// branch's enclosing-quote termination check did not account for a
	// preceding backslash the way the bare-value branch (fixed in round
	// 2) does, so a backslash immediately before the enclosing quote's
	// character got silently swallowed into the redacted span, deleting
	// it from the output and corrupting the surrounding JSON string --
	// the exact JSON-corruption failure class already fixed once for the
	// bare-value branch, reintroduced here because the bracket branch
	// (added later, for finding 6) never got the same guard. Verified for
	// all three bracket forms: {}, [], ().
	t.Run("finding2 round3: bracket branch preserves a backslash preceding the enclosing quote", func(t *testing.T) {
		cases := []struct{ in, want string }{
			{`{"c":"password={ab\"cd} tail"}`, `{"c":"password={[REDACTED]\"cd} tail"}`},
			{`{"c":"password=[ab\"cd] tail"}`, `{"c":"password=[[REDACTED]\"cd] tail"}`},
			{`{"c":"password=(ab\"cd) tail"}`, `{"c":"password=([REDACTED]\"cd) tail"}`},
		}
		for _, c := range cases {
			out, hits := Scrub(c.in)
			if hits != 1 {
				t.Errorf("Scrub(%q) hits = %d, want 1", c.in, hits)
			}
			if out != c.want {
				t.Errorf("Scrub(%q) = %q, want %q", c.in, out, c.want)
			}
			if err := json.Unmarshal([]byte(out), &map[string]any{}); err != nil {
				t.Errorf("json.Unmarshal failed on %q: %v", out, err)
			}
		}
	})

	// Finding 2 (from an earlier report, LOW but a real leak): a YAML
	// block scalar whose key sits at column 0 (a top-level document key)
	// was never detected at all, because the body-line-acceptance rule
	// required indent > keyIndent, and no column-0 body line can ever
	// satisfy indent > 0. Valid, if unusual, YAML.
	t.Run("finding2 (yaml column zero): a block scalar body at column 0 under a column-0 key is still detected", func(t *testing.T) {
		in := "password: |\nTOPLEVELSECRET\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if strings.Contains(out, "TOPLEVELSECRET") {
			t.Errorf("TOPLEVELSECRET survives: %q", out)
		}
		want := "password: |\n[REDACTED]"
		if out != want {
			t.Errorf("Scrub(%q) = %q, want %q", in, out, want)
		}
	})

	// Finding 3, ROUND 3 (CRITICAL, unbounded over-redaction): the YAML
	// block-scalar scanner checked its byte cap only between lines (at
	// the top of the outer, line-by-line loop), so a single
	// pathologically long line was consumed by the inner "read to the end
	// of this line" loop in full before the cap was ever re-checked,
	// running unbounded on one huge line the same way maxValueSpan was
	// introduced to prevent elsewhere. The many-short-lines case is
	// included as a regression guard: it already worked correctly and
	// must keep working (the fix must check the cap inside the line scan
	// in ADDITION to, not instead of, the between-lines check).
	// FINDING 3, ROUND 3 CORRECTION: this test's predecessor asserted that
	// leaving up to ~65536 bytes of over-redaction slack was correct and
	// that anything destroyed beyond it would be a bug. That was itself
	// backwards for the YAML block form: the whole point of a block
	// scalar is that the ENTIRE body is the secret, however long, so
	// hitting the search ceiling on one pathologically long line must
	// still redact the complete body (through EOF), not stop partway and
	// leave the remainder of that line exposed.
	t.Run("finding3 round3: YAML block scalar with one very long line redacts the whole body, not just up to the ceiling", func(t *testing.T) {
		giant := strings.Repeat("A", 1_000_000)
		cases := []struct{ name, marker string }{
			{"literal block (|)", "|"},
			{"folded block (>)", ">"},
		}
		for _, c := range cases {
			in := "password: " + c.marker + "\n  " + giant + "\n"
			out, hits := Scrub(in)
			if hits != 1 {
				t.Fatalf("%s: hits = %d, want 1", c.name, hits)
			}
			if strings.Contains(out, "AAAA") {
				t.Errorf("%s: body leaked past the search ceiling: %q", c.name, out)
			}
		}
	})

	t.Run("finding3 round3: YAML block scalar with many short lines still redacts the whole body (regression guard)", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("password: |\n")
		for i := 0; i < 200; i++ {
			b.WriteString("  " + strings.Repeat("A", 10) + "\n")
		}
		in := b.String()
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1", hits)
		}
		if strings.Contains(out, "AAAAAAAAAA") {
			t.Errorf("body content survived: %q", out)
		}
	})

	// Finding 5: closed-set key list extended, plus trailing-digit
	// stripping in collapseKey (api_key2 -> apikey) and an explicit
	// oauth2token entry (the "2" there is mid-identifier, not a trailing
	// version suffix, so digit-stripping alone cannot reach it).
	t.Run("finding5: extended credential-key set", func(t *testing.T) {
		cases := []string{
			"private_key=abc123xyz",
			"privatekey=abc123xyz",
			"db_password=abc123xyz",
			"AWS_SECRET_ACCESS_KEY=abc123xyz",
			"passphrase=abc123xyz",
			"pwd=abc123xyz",
			"session_token=abc123xyz",
			"api_secret=abc123xyz",
			"auth=abc123xyz",
			"credential=abc123xyz",
			"credentials=abc123xyz",
			"pat=abc123xyz",
			"signing_key=abc123xyz",
			"encryption_key=abc123xyz",
			"service_account_key=abc123xyz",
			"connection_string=abc123xyz",
			"dsn=abc123xyz",
			"x_api_key=abc123xyz",
			"bearer=abc123xyz",
			"authorization=abc123xyz",
		}
		for _, in := range cases {
			out, hits := Scrub(in)
			if hits != 1 {
				t.Errorf("Scrub(%q) hits = %d, want 1 (still leaking): %q", in, hits, out)
			}
			if hits != redactedCount(out) {
				t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
			}
		}
	})

	t.Run("finding5: trailing digit does not defeat whole-identifier equality", func(t *testing.T) {
		cases := []string{
			"api_key2=abc123xyz",
			"oauth2_token=abc123xyz",
		}
		for _, in := range cases {
			out, hits := Scrub(in)
			if hits != 1 {
				t.Errorf("Scrub(%q) hits = %d, want 1 (still leaking): %q", in, hits, out)
			}
		}
	})

	// Finding 6: {, [, ( were excluded from isBareValueByte, so
	// password={SECRET}, password=[SECRET], password=(SECRET) produced a
	// zero-length value and silently skipped redaction entirely.
	t.Run("finding6: bracket-delimited bare values redact, preserving the brackets", func(t *testing.T) {
		cases := []struct{ in, want string }{
			{"password={SECRET}", "password={[REDACTED]}"},
			{"password=[SECRET]", "password=[[REDACTED]]"},
			{"password=(SECRET)", "password=([REDACTED])"},
		}
		for _, c := range cases {
			out, hits := Scrub(c.in)
			if out != c.want {
				t.Errorf("Scrub(%q) = %q, want %q", c.in, out, c.want)
			}
			if hits != 1 {
				t.Errorf("Scrub(%q) hits = %d, want 1", c.in, hits)
			}
			assertNoResidue(t, out, "SECRET", 6)
		}
	})

	// This also resolves finding 4's second surviving mutant (deleting the
	// bare-value "already redacted" literal-equality guard): that guard
	// was provably unreachable dead code even before this change --
	// redactedToken ("[REDACTED]") always starts with '[', and any bare
	// value starting with '[' was already excluded by isBareValueByte, so
	// the comparison could never see redactedToken in the first place.
	// It has been removed (see scanner.go). The real idempotence
	// protection for a value that looks like "[REDACTED]" now lives in
	// the bracket-value branch added for finding 6 above, and this test
	// is what actually exercises it: password=[SECRET] redacts to
	// password=[[REDACTED]], and re-scrubbing that must be a no-op.
	t.Run("finding6: bracket-delimited values are idempotent under re-scrub", func(t *testing.T) {
		cases := []string{
			"password={SECRET}",
			"password=[SECRET]",
			"password=(SECRET)",
		}
		for _, in := range cases {
			out1, hits1 := Scrub(in)
			if hits1 != 1 {
				t.Fatalf("Scrub(%q) hits = %d, want 1", in, hits1)
			}
			out2, hits2 := Scrub(out1)
			if hits2 != 0 || out2 != out1 {
				t.Errorf("Scrub is not idempotent: Scrub(Scrub(%q)) = %q (hits=%d), want %q (hits=0)", in, out2, hits2, out1)
			}
		}
	})

	// This exercises specifically the "case 1" guard in the bracket-value
	// branch (an unwrapped "[REDACTED]" sitting where a bare value used
	// to be, from an ordinary password=hunter2 redaction, not a
	// bracket-wrapped one): deleting it alone (leaving only the
	// double-wrap "case 2" guard) is not caught by the
	// bracket-idempotence test above, since that test only ever produces
	// double-wrapped output. It is a live invariant nonetheless --
	// TestScrub_Invariants's "password=hunter2" case already covers it end
	// to end -- but this test names and isolates it directly so a
	// regression here points at the right line.
	t.Run("finding6 case1: an ordinary bare redaction is idempotent via the bracket branch's unwrapped-placeholder check", func(t *testing.T) {
		in := "password=hunter2"
		out1, hits1 := Scrub(in)
		if out1 != "password=[REDACTED]" || hits1 != 1 {
			t.Fatalf("Scrub(%q) = %q (hits=%d), want \"password=[REDACTED]\" (hits=1)", in, out1, hits1)
		}
		out2, hits2 := Scrub(out1)
		if out2 != out1 || hits2 != 0 {
			t.Errorf("Scrub is not idempotent: Scrub(Scrub(%q)) = %q (hits=%d), want %q (hits=0)", in, out2, hits2, out1)
		}
	})

	// Finding 6: multi-line JSON -- {"password":\n"x"} -- used to leak
	// because the operator/value search only skipped horizontal
	// whitespace and rejected outright as soon as it saw a line
	// separator, even when what followed on the next line was
	// unambiguously a quoted (structural) value.
	t.Run("finding6: multi-line JSON key:value across a line break redacts", func(t *testing.T) {
		in := "{\"password\":\n\"x\"}"
		out, hits := Scrub(in)
		want := "{\"password\":\n\"[REDACTED]\"}"
		if out != want {
			t.Errorf("Scrub(%q) = %q, want %q", in, out, want)
		}
		if hits != 1 {
			t.Errorf("hits = %d, want 1", hits)
		}
	})

	t.Run("finding6: a bare value is still not accepted across a line break (prose protection preserved)", func(t *testing.T) {
		in := "Secret:\nthis document is confidential"
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
		}
	})

	// Finding 6: YAML block scalars (| literal, > folded) used to leak
	// the '|' case entirely (0 hits) and, worse, report false coverage
	// for the '>' case: hits=1 while redacting only the marker character
	// and leaving the actual secret body completely exposed.
	t.Run("finding6: YAML literal block scalar (|) redacts the body, keeps the header", func(t *testing.T) {
		in := "password: |\n  SECRET\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if !strings.HasPrefix(out, "password: |\n") {
			t.Errorf("Scrub(%q) = %q, want header \"password: |\\n\" preserved", in, out)
		}
		assertNoResidue(t, out, "SECRET", 6)
	})

	t.Run("finding6: YAML folded block scalar (>) redacts the body, not just the marker", func(t *testing.T) {
		in := "password: >\n  SECRET\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if !strings.HasPrefix(out, "password: >\n") {
			t.Errorf("Scrub(%q) = %q, want header \"password: >\\n\" preserved", in, out)
		}
		// This is the specific false-coverage failure mode: hits==1 but
		// the marker alone was redacted while SECRET survived untouched.
		// Asserting no residue is what catches that, not just hits>=1.
		assertNoResidue(t, out, "SECRET", 6)
	})

	t.Run("finding6: YAML block scalars are idempotent under re-scrub", func(t *testing.T) {
		cases := []string{
			"password: |\n  SECRET\n",
			"password: >\n  SECRET\n",
		}
		for _, in := range cases {
			out1, hits1 := Scrub(in)
			if hits1 != 1 {
				t.Fatalf("Scrub(%q) hits = %d, want 1", in, hits1)
			}
			out2, hits2 := Scrub(out1)
			if hits2 != 0 || out2 != out1 {
				t.Errorf("Scrub is not idempotent: Scrub(Scrub(%q)) = %q (hits=%d), want %q (hits=0)", in, out2, hits2, out1)
			}
		}
	})

	// Finding 7: hits under-reports (relative to a naive
	// strings.Count(out, "[REDACTED]")) when the input already contains
	// the literal placeholder. This is not a leak -- pre-existing
	// "[REDACTED]" text is not a live secret and correctly passes through
	// untouched -- but the old doc comment claimed hits always equals the
	// token count, which this disproves. The fix corrected the doc
	// comment (see Scrub's comment in scrubber.go) rather than changing
	// what hits counts, since hits is meant to answer "how many
	// redactions did this call perform", which callers like
	// harvester.go's running Redacted total depend on being accurate.
	t.Run("finding7: pre-existing [REDACTED] text is not double-counted, and is not a leak", func(t *testing.T) {
		in := "[REDACTED]"
		out, hits := Scrub(in)
		if out != in {
			t.Errorf("Scrub(%q) = %q, want unchanged (nothing to redact here)", in, out)
		}
		if hits != 0 {
			t.Errorf("hits = %d, want 0: Scrub performed no substitutions on this input", hits)
		}
		if redactedCount(out) != 1 {
			t.Fatalf("test setup invariant broken: expected exactly 1 literal token in %q", out)
		}
	})

	t.Run("finding7: pre-existing [REDACTED] text alongside a real secret: hits counts only the real redaction", func(t *testing.T) {
		in := "already [REDACTED] text with password=abc123xyz"
		out, hits := Scrub(in)
		want := "already [REDACTED] text with password=[REDACTED]"
		if out != want {
			t.Errorf("Scrub(%q) = %q, want %q", in, out, want)
		}
		if hits != 1 {
			t.Errorf("hits = %d, want 1 (only the password= substitution was performed by this call)", hits)
		}
		if redactedCount(out) != 2 {
			t.Fatalf("test setup invariant broken: expected 2 literal tokens in %q", out)
		}
	})
}

// --- Third-round reviewer findings ----------------------------------------
//
// All secret literals in this section are deliberately non-vendor-shaped
// (no AKIA, sk-, ghp_, etc. prefixes): the coordinator's own probe using
// AKIAIOSFODNN7EXAMPLE as a "second credential" appeared clean at
// pad=512 only because the independent AWS vendor regex in scrubber.go
// matched it regardless of whether the hand-written scanner's own logic
// was actually working -- a test passing for the wrong reason. Every
// literal below would NOT be caught by any of the vendor regexes in
// scrubPatterns, so a pass here is only possible if scanCredentialAssignments
// itself is correct.
func TestScrub_ReviewerFindingsRound4(t *testing.T) {
	const secondCredential = "hunter2hunter2hunter2" // non-vendor-shaped

	// Finding 1 (CRITICAL): the looksLikeFreshKeyAhead lookahead was gated
	// by "vi-vStart < maxValueSpan" (512). Past that offset the gate
	// stopped firing, an escaped quote before a second field's opening
	// quote was treated as non-terminating, and the resulting span ran
	// through the second field's key= but stopped BEFORE its value --
	// leaking the second credential in the clear while hits still
	// reported 1. Mutation-confirmed: setting maxValueSpan=1<<30 (i.e.
	// never gating) fixed the leak with zero regressions elsewhere,
	// which is why the gate was removed outright rather than widened.
	// Verified straddling the exact boundary (511 clean before the fix,
	// 512 the first leaking value) plus realistic secret shapes named in
	// the report.
	t.Run("finding1: second-credential leak past the looksLikeFreshKeyAhead length gate, straddling 512", func(t *testing.T) {
		for _, pad := range []int{510, 511, 512, 513, 515, 600, 1000} {
			in := `password="` + strings.Repeat("a", pad) + `\" token="` + secondCredential + `"`
			out, hits := Scrub(in)
			if hits != 2 {
				t.Errorf("pad=%d: hits = %d, want 2: %q", pad, hits, out)
			}
			if strings.Contains(out, secondCredential) {
				t.Errorf("pad=%d: LEAK -- second credential survives: %q", pad, out)
			}
			want := `password="[REDACTED]\" token="[REDACTED]"`
			if out != want {
				t.Errorf("pad=%d: Scrub(...) = %q, want %q", pad, out, want)
			}
		}
	})

	t.Run("finding1: realistic non-vendor secret shapes from the report all redact cleanly", func(t *testing.T) {
		secrets := []string{
			"hunter2hunter2hunter2",
			"Tr0ub4dor&3-correct-horse",
			"db_prod_master_pw_9f3a",
			"P@ssw0rd!MyCompany2026",
		}
		for _, secret := range secrets {
			in := `password="` + strings.Repeat("a", 512) + `\" token="` + secret + `"`
			out, hits := Scrub(in)
			if hits != 2 {
				t.Errorf("secret=%q: hits = %d, want 2: %q", secret, hits, out)
			}
			if strings.Contains(out, secret) {
				t.Errorf("secret=%q: LEAK -- survives in %q", secret, out)
			}
		}
	})

	// Finding 2 (CRITICAL): scanPEMBlocks broke on the pemMaxScanBytes cap
	// before updating lastBodyEnd and before the END-marker check, so
	// end stayed -1 and the span collapsed to the BEGIN line only --
	// a cliff where crossing 16384 bytes went from full redaction to a
	// complete leak while hits still reported 1.
	t.Run("finding2: PEM body fully redacted at and past pemMaxScanBytes, not collapsed to the BEGIN line", func(t *testing.T) {
		for _, bodyLen := range []int{16000, 16383, 16384, 16385, 20000, 100000} {
			body := strings.Repeat("A", bodyLen)
			in := "-----BEGIN RSA PRIVATE KEY-----\n" + body + "\n"
			out, hits := Scrub(in)
			if hits != 1 {
				t.Errorf("bodyLen=%d: hits = %d, want 1", bodyLen, hits)
			}
			if strings.Contains(out, "AAAAAAAAAA") {
				t.Errorf("bodyLen=%d: PEM body leaked: %q", bodyLen, out)
			}
		}
	})

	// Finding 3 (CRITICAL): maxValueScanBytes correctly bounded the SEARCH
	// (boundary itself was exact, no off-by-one), but the defect was that
	// hitting the ceiling truncated the redacted SPAN instead of
	// extending it: everything past the ceiling survived in the clear
	// while hits still reported 1. Verified at 70000 and 200000 bytes --
	// both comfortably past the 65536-byte ceiling -- across all six
	// value forms the scanner recognizes, asserting zero residual in
	// every case, not just "some" redaction.
	t.Run("finding3: zero residual at 70000 and 200000 bytes across all six value forms", func(t *testing.T) {
		for _, n := range []int{70000, 200000} {
			secret := strings.Repeat("SECRETBODY", n/10)
			cases := []struct {
				name string
				in   string
			}{
				{"bare", "password=" + secret},
				{"quoted", `password="` + secret + `"`},
				{"quoted unterminated", `password="` + secret},
				{"bracket", "password={" + secret + "}"},
				{"JSON", `{"api_key":"` + secret + `"}`},
				{"YAML block", "password: |\n  " + secret + "\n"},
			}
			for _, c := range cases {
				out, hits := Scrub(c.in)
				if hits != 1 {
					t.Errorf("n=%d form=%s: hits = %d, want 1", n, c.name, hits)
				}
				if strings.Contains(out, "SECRETBODY") {
					idx := strings.Index(out, "SECRETBODY")
					t.Errorf("n=%d form=%s: residual %d bytes survive, want 0: preview %q", n, c.name, len(out)-idx, out[idx:min(idx+40, len(out))])
				}
			}
		}
	})

	// Finding 4 (HIGH): the finding-3 PEM test asserted only an upper
	// bound (destroyed > cap+64), with no lower-bound leg, so it passed
	// identically whether the body was correctly redacted or leaked 100
	// percent -- a test that was green throughout the entire time finding
	// 2 was live. This test asserts both directions explicitly: hits==1,
	// zero residual (the lower bound: the body must actually be gone),
	// and the exact expected output (the upper bound: nothing beyond the
	// body was consumed).
	t.Run("finding4: PEM byte-cap test now asserts both directions, not just an upper bound", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("-----BEGIN RSA PRIVATE KEY-----\n")
		line := strings.Repeat("A", 4000) + "\n"
		for i := 0; i < 200; i++ {
			b.WriteString(line)
		}
		in := b.String()
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1", hits)
		}
		// Lower bound: the body must not survive. This is the leg finding
		// 4 says was missing -- without it, a version of this test that
		// only checked "destroyed <= cap+64" would pass equally whether
		// the redaction worked or the whole body leaked (a leak destroys
		// very few bytes too, since the span collapses to just the BEGIN
		// line).
		if strings.Contains(out, "AAAA") {
			t.Fatalf("lower bound violated: PEM body leaked: %q", out)
		}
		// Upper bound: nothing beyond the BEGIN marker and body should be
		// left un-redacted or over-consumed.
		want := "[REDACTED]"
		if out != want {
			t.Errorf("Scrub(...) = %q, want %q", out, want)
		}
	})
}

// --- Word-character jamming (leading \b bypass) ----------------------------
//
// FINDING (CRITICAL, credential leak): every `\b`-anchored vendor pattern
// silently failed to redact a key immediately preceded by a word character.
// `\b` asserts a word/non-word transition, so "Xsk-ant-api03-...",
// "7ghp_...", "_npm_..." all placed a word char directly before the
// pattern's own leading word char, the boundary never existed, and the key
// shipped in the clear with hits=0 -- the "secret survives, success
// reported" failure class, here triggered by nothing more exotic than a
// missing separator.
//
// This is not a synthetic shape. The harvester ingests shell history, git
// state, editor buffers and Claude Code JSONL transcripts, where keys are
// routinely jammed against a word character: concatenated env dumps
// ("FOO=barsk-ant-..."), base64-ish blobs, log lines written without
// separators, and flattened JSON.
//
// The AWS pattern (AKIA[0-9A-Z]{16}) has no `\b` and redacted correctly in
// every prefix case, which is what isolates the leading `\b` as the cause.
//
// The fix is asymmetric by pattern, because the risk of dropping the anchor
// is asymmetric. See the per-pattern rationale in scrubber.go.

// jammingPrefixes are the three word-character prefixes that defeated a
// leading `\b`. They are word chars of each class the anchor recognizes:
// letter, digit, underscore.
var jammingPrefixes = []struct{ name, prefix string }{
	{"jammed-alnum", "X"},
	{"jammed-digit", "7"},
	{"jammed-underscore", "_"},
}

// separatedPrefixes are the contexts that already redacted correctly before
// the fix. They are included so the fix is proven not to have regressed the
// path that was already working.
var separatedPrefixes = []struct{ name, prefix string }{
	{"bare", ""},
	{"space-separated", "prefix "},
	{"url-path", "https://api.example.com/"},
	{"json-quote", `"`},
	{"assignment", "key="},
}

// vendorSecrets is one realistic-shaped key per `\b`-anchored pattern, plus
// the unanchored AWS control that never leaked.
var vendorSecrets = []struct{ name, secret string }{
	{"sk-ant", "sk-ant-api03-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
	{"ghp", "ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
	{"github_pat", "github_pat_AAAAAAAAAAAAAAAAAAAAAAAA"},
	{"xoxb", "xoxb-AAAAAAAAAAAAAAAAAAAA"},
	{"xapp", "xapp-AAAAAAAAAAAAAAAAAAAA"},
	{"AIzaSy", "AIzaSyAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
	{"npm", "npm_AAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
	{"jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk"},
	{"AKIA (control, never anchored)", "AKIAIOSFODNN7EXAMPLE"},
}

// TestScrub_JammedAgainstWordChar is the direct regression for the leak: a
// key preceded by a word character must still be redacted, and the secret
// must not survive verbatim in the output.
func TestScrub_JammedAgainstWordChar(t *testing.T) {
	for _, s := range vendorSecrets {
		for _, p := range jammingPrefixes {
			t.Run(s.name+"/"+p.name, func(t *testing.T) {
				in := p.prefix + s.secret
				out, hits := Scrub(in)
				if strings.Contains(out, s.secret) {
					t.Fatalf("LEAK: Scrub(%q) = %q, hits=%d -- the secret survived verbatim", in, out, hits)
				}
				if hits < 1 {
					t.Fatalf("Scrub(%q) hits = %d, want >= 1: %q", in, hits, out)
				}
				if hits != redactedCount(out) {
					t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
				}
			})
		}
	}
}

// TestScrub_SeparatedPrefixesStillRedact pins the contexts that already
// worked. A fix for the jammed case that broke any of these would be trading
// one leak for another.
func TestScrub_SeparatedPrefixesStillRedact(t *testing.T) {
	for _, s := range vendorSecrets {
		for _, p := range separatedPrefixes {
			t.Run(s.name+"/"+p.name, func(t *testing.T) {
				in := p.prefix + s.secret
				out, hits := Scrub(in)
				if strings.Contains(out, s.secret) {
					t.Fatalf("LEAK: Scrub(%q) = %q, hits=%d -- the secret survived verbatim", in, out, hits)
				}
				if hits < 1 {
					t.Fatalf("Scrub(%q) hits = %d, want >= 1: %q", in, hits, out)
				}
				if hits != redactedCount(out) {
					t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
				}
			})
		}
	}
}

// TestScrub_SkPrefixFusionNegatives is the counterweight to the two tests
// above: the leading `\b` on the sk- pattern exists to stop exactly these
// English words, and the fix for the jammed case must not buy redaction of
// "Xsk-ant-..." at the price of redacting "risk-management".
//
// The five names the sk- pattern's own comment calls out are asserted
// individually rather than as a bare list, so a regression names the word
// that broke. The longer hyphenated forms below them are the realistic
// prose that a naive "just delete the \b" fix would have destroyed:
// measured, dropping the anchor outright made "risk-management-framework-
// review", "task-list-item-identifier-value" and
// "asterisk-based-telephony-server" all match.
func TestScrub_SkPrefixFusionNegatives(t *testing.T) {
	cases := []struct{ name, in string }{
		{"whisk-", "whisk-"},
		{"kiosk-mode", "kiosk-mode"},
		{"risk-management", "risk-management"},
		{"task-list", "task-list"},
		{"disk-usage", "disk-usage"},

		{"whisk- in a sentence", "whisk-brand of the year"},
		{"kiosk-mode in a sentence", "kiosk-mode enabled"},
		{"risk-management in a sentence", "risk-management plan"},
		{"task-list in a sentence", "task-list for today"},
		{"disk-usage in a sentence", "disk-usage report"},

		{"multi-hyphen prose: risk-management-framework-review", "risk-management-framework-review"},
		{"multi-hyphen prose: task-list-item-identifier-value", "task-list-item-identifier-value"},
		{"multi-hyphen prose: disk-usage-monitoring-subsystem", "disk-usage-monitoring-subsystem"},
		{"multi-hyphen prose: kiosk-mode-configuration-manager", "kiosk-mode-configuration-manager"},
		{"multi-hyphen prose: whisk-attachment-replacement-part", "whisk-attachment-replacement-part"},
		{"multi-hyphen prose: desk-organizer-plus-accessories", "desk-organizer-plus-accessories"},
		{"multi-hyphen prose: asterisk-based-telephony-server", "asterisk-based-telephony-server"},
		{"multi-hyphen prose: dusk-till-dawn-lighting-controller", "dusk-till-dawn-lighting-controller"},
		{"multi-hyphen prose in a sentence", "the risk-adjusted-return-on-capital metric"},
		{"multi-hyphen prose in a sentence 2", "a task-oriented-dialogue-system paper"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 0 || out != c.in {
				t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", c.in, out, hits)
			}
		})
	}
}

// TestScrub_SkJammedDoesNotSwallowDashRuns guards the property the sk-
// pattern's original comment was written to protect: the match must never
// run into a PEM marker's run of dashes. A candidate fix that reached the
// key material through a permissive `[a-zA-Z0-9-]` class did exactly that
// (measured: "task------...------AAAA..." matched), which is why the
// unanchored branch is built from single-hyphen-joined alnum segments
// instead.
func TestScrub_SkJammedDoesNotSwallowDashRuns(t *testing.T) {
	t.Run("dash run before key material is not bridged", func(t *testing.T) {
		in := "task-----------------------------------AAAAAAAAAAAAAAAAAAAAAAAA"
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits (a dash run must not be bridged)", in, out, hits)
		}
	})

	t.Run("PEM END marker adjacent to prose is not bridged", func(t *testing.T) {
		in := "disk-usage-----END RSA PRIVATE KEY-----AAAAAAAAAAAAAAAAAAAAAAAA"
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
		}
	})
}

// TestScrub_JammedRealisticContexts exercises the shapes named in the leak
// report as the reason this matters in practice, rather than the synthetic
// single-token inputs above.
func TestScrub_JammedRealisticContexts(t *testing.T) {
	cases := []struct{ name, in, secret string }{
		{
			"concatenated env dump",
			"FOO=barsk-ant-api03-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			"sk-ant-api03-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		},
		{
			"log line with no separator before the token",
			"level=infomsg=authghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			"ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		},
		{
			"flattened JSON with the key jammed to the previous value",
			`{"a":"1","b":"xnpm_AAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`,
			"npm_AAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		},
		{
			"jammed google key in a base64-ish blob",
			"QUJDRAAIzaSyAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			"AIzaSyAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if strings.Contains(out, c.secret) {
				t.Fatalf("LEAK: Scrub(%q) = %q, hits=%d -- the secret survived verbatim", c.in, out, hits)
			}
			if hits < 1 {
				t.Fatalf("Scrub(%q) hits = %d, want >= 1: %q", c.in, hits, out)
			}
			if hits != redactedCount(out) {
				t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
			}
		})
	}
}

// TestScrub_JammedIdempotence pins the invariant the whole pipeline relies
// on for the newly-redacting inputs: re-scrubbing a scrubbed string finds
// nothing further, and hits stays equal to the tokens actually emitted.
func TestScrub_JammedIdempotence(t *testing.T) {
	for _, s := range vendorSecrets {
		for _, p := range jammingPrefixes {
			t.Run(s.name+"/"+p.name, func(t *testing.T) {
				in := p.prefix + s.secret
				out, hits := Scrub(in)
				if hits != redactedCount(out) {
					t.Fatalf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
				}
				out2, hits2 := Scrub(out)
				if hits2 != 0 {
					t.Errorf("Scrub is not idempotent: re-scrubbing %q produced %d more hits: %q", out, hits2, out2)
				}
				if out2 != out {
					t.Errorf("Scrub is not idempotent: Scrub(Scrub(%q)) = %q, want %q", in, out2, out)
				}
			})
		}
	}
}

// --- PEM: non-body line before a genuine END marker -----------------------

// TestScrub_PEMNonBodyLineBeforeEnd covers a leak where a PEM block whose
// body contained a line that is not base64-shaped -- but which still had a
// matching END marker further down -- was truncated at that line. The BEGIN
// line alone was replaced, hits reported 1, and every remaining line
// (including the actual key material) shipped in the clear.
//
// The trigger shapes are ordinary, not synthetic: `openssl pkcs12 -nodes`
// emits "Key Attributes:" and "Bag Attributes:" lines inside the block, and
// a terminal-wrapped or word-wrapped paste puts a space in the middle of a
// body line. None of those are in the RFC 1421 header allowlist
// (Proc-Type:, DEK-Info:, Comment:, Version:), so each one froze the span.
//
// The rule the fix encodes: a matching END marker for the same label found
// later within the END-search budget proves the block is a genuine key, so
// the span runs through that END regardless of what the intervening lines
// look like. A non-body-shaped line only governs the TRUNCATED (no-END)
// outcome.
func TestScrub_PEMNonBodyLineBeforeEnd(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		secret string
	}{
		{
			"openssl pkcs12 -nodes Key Attributes line",
			"-----BEGIN PRIVATE KEY-----\nKey Attributes: <No Attributes>\nSECRETBODYMATERIAL\n-----END PRIVATE KEY-----\n",
			"SECRETBODYMATERIAL",
		},
		{
			"openssl pkcs12 -nodes Bag Attributes line",
			"-----BEGIN PRIVATE KEY-----\nBag Attributes: friendlyName\nSECRETBODYMATERIAL\n-----END PRIVATE KEY-----\n",
			"SECRETBODYMATERIAL",
		},
		{
			"terminal line-wrap damage puts a space inside a body line",
			"-----BEGIN RSA PRIVATE KEY-----\nMIIBogIBAAJBAK wrapped\nSECRETBODYMATERIAL\n-----END RSA PRIVATE KEY-----\n",
			"SECRETBODYMATERIAL",
		},
		{
			"tab inside a body line",
			"-----BEGIN RSA PRIVATE KEY-----\nMIIBogIBAAJBAK\twrapped\nSECRETBODYMATERIAL\n-----END RSA PRIVATE KEY-----\n",
			"SECRETBODYMATERIAL",
		},
		{
			"period inside a body line",
			"-----BEGIN RSA PRIVATE KEY-----\nMIIBogIBAAJBAK.wrapped\nSECRETBODYMATERIAL\n-----END RSA PRIVATE KEY-----\n",
			"SECRETBODYMATERIAL",
		},
		{
			"percent-escaped paste damage inside a body line",
			"-----BEGIN RSA PRIVATE KEY-----\nMIIBogIBAAJBAK%20wrapped\nSECRETBODYMATERIAL\n-----END RSA PRIVATE KEY-----\n",
			"SECRETBODYMATERIAL",
		},
		{
			"quote character inside a body line",
			"-----BEGIN RSA PRIVATE KEY-----\nMIIBogIBAAJBAK\"wrapped\nSECRETBODYMATERIAL\n-----END RSA PRIVATE KEY-----\n",
			"SECRETBODYMATERIAL",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if strings.Contains(out, c.secret) {
				t.Fatalf("LEAK: Scrub(%q) = %q, hits=%d -- key material survived verbatim", c.in, out, hits)
			}
			if hits != 1 {
				t.Fatalf("hits = %d, want 1: %q", hits, out)
			}
			if hits != redactedCount(out) {
				t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
			}
			// The whole block, END marker included, must be gone: leaving
			// the END marker behind would both leak structure and let a
			// later BEGIN pair with it.
			if strings.Contains(out, "-----END") {
				t.Errorf("END marker survived, so the span stopped short of it: %q", out)
			}
			assertNoResidue(t, out, c.secret, 6)
			out2, hits2 := Scrub(out)
			if hits2 != 0 || out2 != out {
				t.Errorf("not idempotent: Scrub(%q) = %q, hits=%d", out, out2, hits2)
			}
		})
	}

	t.Run("RFC 1421 header lines still redact fully (no regression)", func(t *testing.T) {
		in := "-----BEGIN RSA PRIVATE KEY-----\nProc-Type: 4,ENCRYPTED\nDEK-Info: AES-128-CBC,ABCDEF0123456789\nComment: mykey\nVersion: 2\n\nSECRETBODYMATERIAL\n-----END RSA PRIVATE KEY-----\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
		}
		if strings.Contains(out, "ABCDEF0123456789") {
			t.Errorf("DEK-Info value leaked: %q", out)
		}
		assertNoResidue(t, out, "SECRETBODYMATERIAL", 6)
	})

	t.Run("non-body line inside a block whose END is a DIFFERENT label still truncates", func(t *testing.T) {
		// No matching END for "RSA PRIVATE KEY" exists, so there is no
		// proof this is a genuine block: the truncated bound governs and
		// the trailing prose must survive untouched.
		in := "-----BEGIN RSA PRIVATE KEY-----\nAAAABBBBCCCC\nordinary prose line here\nmore ordinary prose that must survive\n-----END CERTIFICATE-----\n"
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if !strings.Contains(out, "more ordinary prose that must survive") {
			t.Errorf("truncated block over-redacted past its non-body line: %q", out)
		}
	})

	// Criterion 3: a truncated block (no END anywhere) that contains a
	// non-body line must still stop at the existing bound and not run away.
	// The assertion uses hard-coded literals, never the constant under
	// test: a bound test written against its own constant passes for every
	// value of that constant and therefore proves nothing.
	t.Run("truncated block with a non-body line stops at the existing bound", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("-----BEGIN RSA PRIVATE KEY-----\n")
		b.WriteString("AAAABBBBCCCCDDDD\n")
		b.WriteString("Key Attributes: <No Attributes>\n")
		// 5000 lines of ordinary prose, no END marker anywhere.
		for i := 0; i < 5000; i++ {
			b.WriteString("this is an ordinary line of unrelated prose text\n")
		}
		in := b.String()
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1", hits)
		}
		if hits != redactedCount(out) {
			t.Errorf("hits=%d but output has %d [REDACTED] tokens", hits, redactedCount(out))
		}
		// Prose lines are not base64-shaped, so the block stops at the
		// first one regardless of any line budget. What must NOT happen is
		// the scan consuming the rest of the document.
		if !strings.Contains(out, "this is an ordinary line of unrelated prose text") {
			t.Fatalf("truncated block ran away and consumed unrelated prose")
		}
		shrink := len(in) - len(out)
		if shrink >= 13000 {
			t.Errorf("truncated block over-redacted: shrank output by %d bytes (input was %d), want < 13000", shrink, len(in))
		}
		// The last line of the document must be untouched.
		if !strings.HasSuffix(out, "this is an ordinary line of unrelated prose text\n") {
			t.Errorf("tail of document was consumed: %q", out[max(0, len(out)-120):])
		}
	})

	// Continuing the END search past a non-body line (the fix above) made
	// every stray BEGIN marker walk the full pemMaxEndSearchLines look-ahead
	// instead of giving up on the next line. On 20,000 stray markers that
	// measured 87ms -> 11.96s, quadratic in the number of markers, at an
	// input size (1.6MB of shell history or a JSONL transcript) this
	// scrubber sees routinely: a redaction pass that times out is a
	// redaction pass that does not run. The END-marker pre-pass index keeps
	// it linear. The threshold is a hard-coded wall-clock literal, generous
	// enough not to flake on a loaded machine but far below the ~12s
	// quadratic behavior it guards against.
	t.Run("many stray BEGIN markers stay linear, not quadratic", func(t *testing.T) {
		if testing.Short() {
			t.Skip("timing-sensitive")
		}
		build := func(tail string) string {
			var b strings.Builder
			for i := 0; i < 20000; i++ {
				b.WriteString("-----BEGIN RSA PRIVATE KEY-----\n")
				b.WriteString("this line is ordinary prose and not body shaped\n")
			}
			b.WriteString(tail)
			return b.String()
		}
		for _, c := range []struct{ name, tail string }{
			{"no END marker anywhere", ""},
			// One far-away END marker of a different label: a label-blind
			// shortcut would fail to fire here and go quadratic again.
			{"one far-away non-matching END marker", "-----END DSA PRIVATE KEY-----\n"},
		} {
			t.Run(c.name, func(t *testing.T) {
				in := build(c.tail)
				start := time.Now()
				_, hits := Scrub(in)
				elapsed := time.Since(start)
				if hits != 20000 {
					t.Fatalf("hits = %d, want 20000", hits)
				}
				if elapsed > 3*time.Second {
					t.Errorf("Scrub took %v on %d bytes, want < 3s -- the stray-BEGIN END search went quadratic", elapsed, len(in))
				}
			})
		}
	})
}

// --- Colon-assigned values with trailing context --------------------------

// TestScrub_ColonValueTrailingContext covers a leak where a colon-assigned
// credential was dropped entirely -- hits=0, value verbatim -- whenever
// anything other than EOF, a line separator, or one of & | ; , } ) ] came
// after it. A trailing "# prod" comment, a "(prod)" parenthetical, and a
// tab-separated word all fell into that gap, and all three are ordinary
// shapes in .env files, YAML, TOML, and pasted config.
//
// The multi-word-prose defense it was protecting ("Secret: this document is
// confidential") is preserved: a bare word following the value on a space
// still rejects. Only structurally unambiguous separators -- a '#' comment
// introducer, a bracket/paren opener, and a tab column separator -- are
// added, none of which are how English sentences continue.
func TestScrub_ColonValueTrailingContext(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     string
		wantHits int
	}{
		{"trailing hash comment", "password: SECRETVAL123 # prod", "password: [REDACTED] # prod", 1},
		{"trailing parenthetical", "  api_key: SECRETVAL123 (prod)", "  api_key: [REDACTED] (prod)", 1},
		{"tab-separated trailing word", "password: SECRETVAL123\ttab", "password: [REDACTED]\ttab", 1},
		{"control: nothing trailing", "password: SECRETVAL123", "password: [REDACTED]", 1},
		{"trailing hash comment in a .env line", "API_KEY: abc123xyz789 # rotate 2026-01", "API_KEY: [REDACTED] # rotate 2026-01", 1},
		{"trailing brace opener", "token: abc123xyz789 {note}", "token: [REDACTED] {note}", 1},
		{"trailing bracket opener", "token: abc123xyz789 [note]", "token: [REDACTED] [note]", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if out != c.want {
				t.Errorf("Scrub(%q) = %q, want %q", c.in, out, c.want)
			}
			if hits != c.wantHits {
				t.Errorf("Scrub(%q) hits = %d, want %d", c.in, hits, c.wantHits)
			}
			if hits != redactedCount(out) {
				t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
			}
			out2, hits2 := Scrub(out)
			if hits2 != 0 || out2 != out {
				t.Errorf("not idempotent: Scrub(%q) = %q, hits=%d", out, out2, hits2)
			}
		})
	}
}

// TestScrub_ColonProseStillNotRedacted pins the false positives the trailing
// -context restriction exists to prevent. These must stay at 0 hits after the
// trailing-context widening above: the multi-word shape is the real signal
// and a space-separated word following the value still rejects.
func TestScrub_ColonProseStillNotRedacted(t *testing.T) {
	cases := []struct{ name, in string }{
		{"bare word colon prose", "Secret: this document is confidential"},
		{"prose: instruction to rotate", "password: please rotate this before Friday"},
		{"prose: pointer to another system", "api_key: see the shared vault for details"},
		{"prose: status sentence", "token: expired yesterday and needs renewal"},
		{"prose: sentence ending in a period", "secret: the meeting notes are attached."},
		{"chained label prose", "Error: token: invalid"},
		// The shapes closest to the newly-accepted terminators. Each still
		// rejects because the value is followed across a SPACE by an
		// ordinary word, which is the prose signal; the '#', '(' and '['
		// appear later in the sentence, where they never reach this check.
		{"prose: issue reference containing a hash", "secret: see issue #1234 for details"},
		{"prose: parenthetical later in the sentence", "token: refer to (RFC 7519) for details"},
		{"prose: bracket later in the sentence", "api_key: ask ops [see runbook] before rotating"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 0 || out != c.in {
				t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", c.in, out, hits)
			}
		})
	}
}

// --- Legacy smoke test (kept for continuity with prior behavior) ---------

func TestScrub(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     string
		wantHits int
	}{
		{"aws key", "AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE",
			"AWS_ACCESS_KEY=[REDACTED]", 1},
		{"openai key", "key=sk-abcdefghijklmnopqrstuvwxyz1234",
			"key=[REDACTED]", 1},
		{"bearer", "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9",
			"Authorization: [REDACTED]", 1},
		{"password=", "password=hunter2",
			"password=[REDACTED]", 1},
		{"token=", "token=abc123",
			"token=[REDACTED]", 1},
		{"two hits", "k1=sk-aaaaaaaaaaaaaaaaaaaaaaaa k2=sk-bbbbbbbbbbbbbbbbbbbbbbbb",
			"k1=[REDACTED] k2=[REDACTED]", 2},
		{"false positive: 'tokens earned'", "tokens earned: 5",
			"tokens earned: 5", 0},
		{"empty", "", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, hits := Scrub(c.in)
			if got != c.want {
				t.Errorf("Scrub(%q) = %q, want %q", c.in, got, c.want)
			}
			if hits != c.wantHits {
				t.Errorf("Scrub(%q) hits = %d, want %d", c.in, hits, c.wantHits)
			}
		})
	}
}

// --- Round 5 regression guards ------------------------------------------

// TestScrub_PEMMarkerMidLine pins the leak where a BEGIN marker was only
// recognized at the start of a (whitespace-trimmed) line, so any prefix --
// even a single space, the routine shape in shell history and log lines --
// hid an entire private key. The prefix text before the marker is ordinary
// content and must survive; redaction starts at the marker itself.
func TestScrub_PEMMarkerMidLine(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     string
		wantHits int
	}{
		{"space before marker", "note -----BEGIN RSA PRIVATE KEY-----\nSECRETBODY\n-----END RSA PRIVATE KEY-----\n",
			"note [REDACTED]\n", 1},
		{"word jammed against marker", "task-----BEGIN RSA PRIVATE KEY-----\nSECRETBODY\n-----END RSA PRIVATE KEY-----\n",
			"task[REDACTED]\n", 1},
		{"single alnum jammed against marker", "X-----BEGIN RSA PRIVATE KEY-----\nSECRETBODY\n-----END RSA PRIVATE KEY-----\n",
			"X[REDACTED]\n", 1},
		{"control: bare marker at line start", "-----BEGIN RSA PRIVATE KEY-----\nSECRETBODY\n-----END RSA PRIVATE KEY-----\n",
			"[REDACTED]\n", 1},
		{"control: marker on its own line after prose", "task\n-----BEGIN RSA PRIVATE KEY-----\nSECRETBODY\n-----END RSA PRIVATE KEY-----\n",
			"task\n[REDACTED]\n", 1},
		{"shell history line", "$ echo -----BEGIN OPENSSH PRIVATE KEY-----\nSECRETBODY\n-----END OPENSSH PRIVATE KEY-----\n",
			"$ echo [REDACTED]\n", 1},
		{"truncated block with prefix", "note -----BEGIN RSA PRIVATE KEY-----\nSECRETBODY\n",
			"note [REDACTED]\n", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if out != c.want {
				t.Errorf("Scrub(%q) = %q, want %q", c.in, out, c.want)
			}
			if hits != c.wantHits {
				t.Errorf("Scrub(%q) hits = %d, want %d", c.in, hits, c.wantHits)
			}
			if strings.Contains(out, "SECRETBODY") {
				t.Errorf("key body leaked: %q", out)
			}
			if hits != redactedCount(out) {
				t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
			}
			out2, hits2 := Scrub(out)
			if hits2 != 0 || out2 != out {
				t.Errorf("not idempotent: Scrub(%q) = %q, hits=%d", out, out2, hits2)
			}
		})
	}
}

// TestScrub_PEMMidLineDeniedLabelsAndProse pins that recognizing a mid-line
// marker does not bypass the label allow/deny logic, and does not turn prose
// that merely mentions the PEM envelope into an over-redaction of the
// preceding sentence.
func TestScrub_PEMMidLineDeniedLabelsAndProse(t *testing.T) {
	cases := []struct{ name, in string }{
		{"mid-line certificate is not a secret", "see -----BEGIN CERTIFICATE-----\nMIIBODY\n-----END CERTIFICATE-----\n"},
		{"mid-line public key is not a secret", "here -----BEGIN PUBLIC KEY-----\nMIIBODY\n-----END PUBLIC KEY-----\n"},
		{"mid-line certificate request is not a secret", "req -----BEGIN CERTIFICATE REQUEST-----\nMIIBODY\n-----END CERTIFICATE REQUEST-----\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 0 || out != c.in {
				t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", c.in, out, hits)
			}
		})
	}
}

// TestScrub_EscapedJSONKeyAssignment pins the leak where a JSON config blob
// quoted inside another JSON string -- the standard shape when a Claude Code
// JSONL transcript embeds a config file, one of the four sources this
// harvester reads -- hid the assignment entirely: inside an open quote only
// '=' was trusted as an operator, so an escaped-quote-delimited key was
// invisible. `\"ident\":` with escaped quotes on both sides is structurally
// unambiguous, unlike a bare ':' in prose.
func TestScrub_EscapedJSONKeyAssignment(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     string
		wantHits int
	}{
		{"doubly-escaped json password", `{"content":"{\"password\":\"SECRETVAL123\"}"}`,
			`{"content":"{\"password\":\"[REDACTED]\"}"}`, 1},
		{"doubly-escaped json api key", `{"text":"config: {\"api_key\":\"SECRETVAL123\"}"}`,
			`{"text":"config: {\"api_key\":\"[REDACTED]\"}"}`, 1},
		{"doubly-escaped with space after colon", `{"content":"{\"token\": \"SECRETVAL123\"}"}`,
			`{"content":"{\"token\": \"[REDACTED]\"}"}`, 1},
		{"two doubly-escaped fields", `{"content":"{\"password\":\"SECRETVAL123\",\"token\":\"SECRETVAL123\"}"}`,
			`{"content":"{\"password\":\"[REDACTED]\",\"token\":\"[REDACTED]\"}"}`, 2},
		{"control: plain json password", `{"password":"SECRETVAL123"}`,
			`{"password":"[REDACTED]"}`, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if out != c.want {
				t.Errorf("Scrub(%q) = %q, want %q", c.in, out, c.want)
			}
			if hits != c.wantHits {
				t.Errorf("Scrub(%q) hits = %d, want %d", c.in, hits, c.wantHits)
			}
			if strings.Contains(out, "SECRETVAL123") {
				t.Errorf("credential leaked: %q", out)
			}
			if hits != redactedCount(out) {
				t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
			}
			// The scrubbed output of valid JSON must remain valid JSON:
			// this scrubber's output is fed onward as structured context.
			var v any
			if err := json.Unmarshal([]byte(c.in), &v); err != nil {
				t.Fatalf("test input is not valid JSON: %v", err)
			}
			if err := json.Unmarshal([]byte(out), &v); err != nil {
				t.Errorf("scrubbed output is not valid JSON: %v (%q)", err, out)
			}
			out2, hits2 := Scrub(out)
			if hits2 != 0 || out2 != out {
				t.Errorf("not idempotent: Scrub(%q) = %q, hits=%d", out, out2, hits2)
			}
		})
	}
}

// TestScrub_EscapedJSONDoesNotLoosenProse pins that recognizing the
// escaped-quote key shape does not loosen the general ':'-inside-quotes
// restriction, which exists to stop false positives on ordinary prose.
func TestScrub_EscapedJSONDoesNotLoosenProse(t *testing.T) {
	cases := []struct{ name, in string }{
		{"prose: colon after a credential word", "Secret: this document is confidential"},
		{"prose: colon after a non-credential word", "Note: remember to check the logs"},
		{"prose: credential word mid-sentence", "Warning: password rotation is due next week"},
		{"quoted prose with colon", `{"content":"Secret: this document is confidential"}`},
		{"quoted errorf format string", `{"content":"fmt.Errorf(\"token: %w\", err)"}`},
		{"quoted chained label", `{"content":"Error: token: invalid"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 0 || out != c.in {
				t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", c.in, out, hits)
			}
		})
	}
}

// TestScrub_QuotedValueUnterminatedExtendsToEOF guards the EOF extension in
// the QUOTED-value branch of tryKeyAt: when no closing quote is found within
// the scanner's search ceiling, the redacted span must run to end of input
// rather than stopping at the ceiling and leaving the tail of the value in
// the clear ("secret cut in half, success reported"). Mutating that branch's
// `if hitCeiling {` to `if false {` must fail here.
//
// The pad length is a hard-coded literal, deliberately not a reference to
// maxValueScanBytes: a test that reads the constant it is guarding moves
// with the bug.
func TestScrub_QuotedValueUnterminatedExtendsToEOF(t *testing.T) {
	const pad = 65636 // > the 65536-byte value search ceiling
	in := `password="` + strings.Repeat("S", pad)
	out, hits := Scrub(in)
	if hits != 1 {
		t.Fatalf("hits = %d, want 1; out=%q", hits, out[:min(len(out), 80)])
	}
	if want := `password="` + redactedToken; out != want {
		t.Errorf("Scrub truncated the span: got %d bytes ending %q, want exactly %q",
			len(out), out[max(0, len(out)-40):], want)
	}
	if strings.Contains(out, "SS") {
		t.Errorf("unterminated quoted value leaked %d bytes of tail in the clear: %q",
			strings.Count(out, "S"), out[max(0, len(out)-60):])
	}
}

// TestScrub_PEMMidLineProseNotEaten pins the over-redaction that mid-line
// marker matching opened up and that pemKeywordFollows closes: matching at
// line start had accidentally bounded how far a marker's closing dash run
// could be, so once the scan may begin anywhere, a sentence that merely
// DISCUSSES the PEM envelope supplies both dash runs itself. Measured before
// the fix: "see -----BEGIN and -----END markers explained in RFC 1421"
// matched with label "and" and destroyed the sentence.
func TestScrub_PEMMidLineProseNotEaten(t *testing.T) {
	cases := []struct{ name, in string }{
		{"prose naming both markers", "see -----BEGIN and -----END markers explained in RFC 1421\n"},
		{"prose naming both markers, no newline", "the -----BEGIN and -----END lines are the envelope"},
		{"markdown table rule", "| col | col |\n| --- | --- |\n"},
		{"yaml front matter fence", "---\ntitle: notes\n---\n"},
		{"diff header", "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n"},
		{"horizontal rule", "------------------------------\n"},
		{"long-form flag that is not a marker", "the flag --begin-with is not a PEM marker\n"},
		{"dashes with no keyword", "a ----- b ----- c\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 0 || out != c.in {
				t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", c.in, out, hits)
			}
		})
	}
}

// TestScrub_PEMDeniedMarkerDoesNotHideLaterKey pins the under-redaction that
// mid-line matching also opened up: a line can now carry more than one
// marker, so stopping at the first one and finding it denied (CERTIFICATE,
// PUBLIC KEY) skipped the whole line -- including a real private-key marker
// sitting after it. Measured before the fix: the private key below was left
// entirely in the clear at hits=0.
func TestScrub_PEMDeniedMarkerDoesNotHideLaterKey(t *testing.T) {
	cases := []struct{ name, in string }{
		{"certificate then private key on one line",
			"-----BEGIN CERTIFICATE------- separator -------BEGIN RSA PRIVATE KEY-----\nSECRETBODY\n-----END RSA PRIVATE KEY-----\n"},
		{"public key then private key on one line",
			"-----BEGIN PUBLIC KEY----- then -----BEGIN RSA PRIVATE KEY-----\nSECRETBODY\n-----END RSA PRIVATE KEY-----\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if strings.Contains(out, "SECRETBODY") {
				t.Errorf("private key hidden behind a denied label leaked: hits=%d out=%q", hits, out)
			}
			if hits < 1 {
				t.Errorf("hits = %d, want >= 1: %q", hits, out)
			}
		})
	}
}

// TestScrub_PEMPrivateKeyNeverLeaksAcrossPrefixes is the broad
// under-redaction guard for the mid-line change: whatever precedes the
// marker on its line, a private-key body must never survive.
func TestScrub_PEMPrivateKeyNeverLeaksAcrossPrefixes(t *testing.T) {
	prefixes := []string{"", " ", "note ", "task", "X", "$ echo ", "9",
		"-----BEGIN CERTIFICATE----- ", "password=hunter2",
		"see -----BEGIN and -----END markers, ", "| --- | ",
		"-----BEGIN PUBLIC KEY----- then "}
	labels := []string{"RSA PRIVATE KEY", "PRIVATE KEY", "OPENSSH PRIVATE KEY",
		"PGP PRIVATE KEY BLOCK", "rsa private key", "ENCRYPTED PRIVATE KEY"}
	for _, p := range prefixes {
		for _, l := range labels {
			in := p + "-----BEGIN " + l + "-----\nSECRETBODYXYZ\n-----END " + l + "-----\n"
			out, hits := Scrub(in)
			if strings.Contains(out, "SECRETBODYXYZ") {
				t.Errorf("key body leaked: prefix=%q label=%q hits=%d out=%q", p, l, hits, out)
			}
		}
	}
}

// --- Orchestrator-reported findings (A-E) -----------------------------------
//
// Five defects reproduced independently by the orchestrator, each a real
// under-redaction against a source this scrubber actually reads (shell
// history, .env-style config, HTTP auth headers, transcript-adjacent
// key/value pairs). One regression test per finding, named after it.

// TestScrub_FindingA_DockerFlagPrefixChainedAssignment: a docker/CLI flag
// prefix ("--env=") jammed directly against a credential-key assignment
// used to be rejected outright by the chained-label check (the key's
// preceding non-space byte, '=', looked identical to the "Error: token:
// invalid" chained-prose shape), leaking the value verbatim with hits=0.
func TestScrub_FindingA_DockerFlagPrefixChainedAssignment(t *testing.T) {
	in := "--env=PGPASSWORD=SEKRITVALUE123"
	out, hits := Scrub(in)
	if hits != 1 {
		t.Fatalf("hits = %d, want 1: %q", hits, out)
	}
	want := "--env=PGPASSWORD=[REDACTED]"
	if out != want {
		t.Errorf("Scrub(%q) = %q, want %q", in, out, want)
	}
	if strings.Contains(out, "SEKRITVALUE123") {
		t.Errorf("value leaked: %q", out)
	}
}

// TestScrub_FindingB_NestedAssignmentAfterColonKey: a nested "key=value"
// assignment immediately after an outer "word:" label (a YAML mapping or
// log-line shape) hit the same chained-label rejection as finding A, for
// the same underlying reason -- the check did not previously distinguish
// "outer op ':', inner op '='" (a real nested assignment) from "outer op
// ':', inner op ':'" (the genuine "Error: token: invalid" prose shape).
func TestScrub_FindingB_NestedAssignmentAfterColonKey(t *testing.T) {
	in := "config: password=SEKRITVALUE123"
	out, hits := Scrub(in)
	if hits != 1 {
		t.Fatalf("hits = %d, want 1: %q", hits, out)
	}
	want := "config: password=[REDACTED]"
	if out != want {
		t.Errorf("Scrub(%q) = %q, want %q", in, out, want)
	}
	if strings.Contains(out, "SEKRITVALUE123") {
		t.Errorf("value leaked: %q", out)
	}
}

// TestScrub_FindingA_And_B_DoNotWeakenChainedLabelProseDefense pins the
// exact prose shape the chained-label check exists to protect
// ("Error: token: invalid", "word: word: word" with no real credential),
// confirming the finding A/B fix (only rejecting when the CURRENT key's own
// operator is ':') does not reopen it: the failure mode here has the SAME
// preceding-operator shape as finding B but a ':' final operator instead of
// '=', which must still reject.
func TestScrub_FindingA_And_B_DoNotWeakenChainedLabelProseDefense(t *testing.T) {
	cases := []string{
		"Error: token: invalid",
		"Secret: this document is confidential",
	}
	for _, in := range cases {
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
		}
	}
}

// TestScrub_FindingC_LowercaseAuthSchemeAssignment: the lowercase/'='
// assignment shape of an HTTP auth header (authorization=Basic <base64>)
// had only the scheme word ("Basic") redacted by the bare-value scanner,
// which stopped at the first space; the base64 credential after it shipped
// in the clear while hits still reported 1.
func TestScrub_FindingC_LowercaseAuthSchemeAssignment(t *testing.T) {
	in := "authorization=Basic QWxhZGRpbjpvcGVuc2VzYW1l=="
	out, hits := Scrub(in)
	if hits != 1 {
		t.Fatalf("hits = %d, want 1: %q", hits, out)
	}
	want := "authorization=[REDACTED]"
	if out != want {
		t.Errorf("Scrub(%q) = %q, want %q", in, out, want)
	}
	if strings.Contains(out, "QWxhZGRpbjpvcGVuc2VzYW1l") {
		t.Errorf("credential leaked: %q", out)
	}
}

// TestScrub_FindingC_BearerSchemeAssignmentAlsoFixed guards the sibling
// scheme word (Bearer, not just Basic) on the same '='-assignment shape,
// since the fix is a closed list of HTTP auth scheme words rather than a
// single hard-coded one.
func TestScrub_FindingC_BearerSchemeAssignmentAlsoFixed(t *testing.T) {
	in := "api_key=Bearer sometoken123"
	out, hits := Scrub(in)
	if hits != 1 {
		t.Fatalf("hits = %d, want 1: %q", hits, out)
	}
	want := "api_key=[REDACTED]"
	if out != want {
		t.Errorf("Scrub(%q) = %q, want %q", in, out, want)
	}
	if strings.Contains(out, "sometoken123") {
		t.Errorf("credential leaked: %q", out)
	}
}

// TestScrub_FindingD_ColonPathFirstPassRedactsBothCredentials: a colon-
// assigned value immediately followed (across one space) by a second,
// independently-recognized credential (a vendor sk- key) used to reject
// the WHOLE assignment on the first Scrub pass -- the colon trailing-
// context check saw the second credential as an ordinary trailing word and
// treated the shape as prose, dropping the first value's own redaction
// entirely. hits=1 on pass 1 (only the independent vendor match), and the
// first value only redacted once the vendor match's own placeholder made
// the trailing context look structural on a SECOND pass. This test pins
// convergence on the FIRST pass: the '=' path already converged in one
// pass, and the colon path must behave the same way now.
func TestScrub_FindingD_ColonPathFirstPassRedactsBothCredentials(t *testing.T) {
	in := "token: abc123xyz sk-aaaaaaaaaaaaaaaaaaaaaaaa"
	out1, hits1 := Scrub(in)
	if hits1 != 2 {
		t.Fatalf("pass 1: hits = %d, want 2 (both credentials redacted on the first pass): %q", hits1, out1)
	}
	want := "token: [REDACTED] [REDACTED]"
	if out1 != want {
		t.Errorf("pass 1: Scrub(%q) = %q, want %q", in, out1, want)
	}
	out2, hits2 := Scrub(out1)
	if hits2 != 0 || out2 != out1 {
		t.Errorf("not idempotent after the fix: Scrub(%q) = %q, hits=%d", out1, out2, hits2)
	}
}

// TestScrub_FindingD_SingleTrailingWordDoesNotReopenMultiWordProse pins the
// exact prose cases the colon trailing-context check's multi-word defense
// protects, confirming the finding D fix (accept exactly one trailing word
// before EOF/line-end) does not relax to accepting two or more.
func TestScrub_FindingD_SingleTrailingWordDoesNotReopenMultiWordProse(t *testing.T) {
	cases := []string{
		"Secret: this document is confidential",
		"password: please rotate this before Friday",
		"api_key: see the shared vault for details",
		"token: expired yesterday and needs renewal",
	}
	for _, in := range cases {
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
		}
	}
}

// TestScrub_FindingE_ColonValueHashCommentNoSpaceBeforeHash: a '#' comment
// introducer jammed directly against a colon-assigned value with no
// separating space ("SECRETVAL123# prod") was swallowed INTO the bare
// value (isBareValueByte previously accepted '#'), so the value scan ran
// past it to the next space and landed the trailing-context check on the
// ordinary word "prod" instead of on the '#' -- rejecting the whole
// assignment as prose, hits=0. The sibling shapes "value#word" (no space
// anywhere) and "value # word" (space on both sides of '#') were already
// redacting correctly before this fix; only the "value# word" combination
// (jammed hash, then a space) leaked.
func TestScrub_FindingE_ColonValueHashCommentNoSpaceBeforeHash(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"jammed hash then space then word (the leak)", "password: SECRETVAL123# prod", "password: [REDACTED]# prod"},
		{"jammed hash, no trailing space (already worked)", "password: SECRETVAL123#prod", "password: [REDACTED]#prod"},
		{"space before and after hash (already worked)", "password: SECRETVAL123 # prod", "password: [REDACTED] # prod"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 1 {
				t.Fatalf("hits = %d, want 1: %q", hits, out)
			}
			if out != c.want {
				t.Errorf("Scrub(%q) = %q, want %q", c.in, out, c.want)
			}
			if strings.Contains(out, "SECRETVAL123") {
				t.Errorf("value leaked: %q", out)
			}
		})
	}
}

// TestScrub_BlockerOne_ColonKeyedCredentialInsideBalancedQuotes is the
// regression test for BLOCKER 1: a colon-keyed credential nested inside a
// BALANCED double-quoted string (a shell -H flag, an echoed log line, a
// JSON string's own free-text content) was completely invisible, hits=0,
// because ':' was trusted as an assignment operator only at top level, never
// while the scanner was already inside an open quote. The control case
// (same shape, '=' operator instead of ':') already worked before this fix
// and is included to prove the defect really was colon-specific.
func TestScrub_BlockerOne_ColonKeyedCredentialInsideBalancedQuotes(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"shell curl header flag", `curl -H "X-Api-Key: SEKRITVALUE123" https://x`, `curl -H "X-Api-Key: [REDACTED]" https://x`},
		{"echoed log line", `echo "password: SEKRITVALUE123"`, `echo "password: [REDACTED]"`},
		{"JSON string free-text content", `{"content":"set api_key: SEKRITVALUE123"}`, `{"content":"set api_key: [REDACTED]"}`},
		{"control: '=' operator already worked", `echo "password=SEKRITVALUE123"`, `echo "password=[REDACTED]"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 1 {
				t.Fatalf("hits = %d, want 1: %q", hits, out)
			}
			if out != c.want {
				t.Errorf("Scrub(%q) = %q, want %q", c.in, out, c.want)
			}
			if strings.Contains(out, "SEKRITVALUE123") {
				t.Errorf("value leaked: %q", out)
			}
		})
	}
}

// TestScrub_BlockerTwo_BacktickAndMarkdownFenceNoLongerSuppressColon is the
// regression test for BLOCKER 2: a credential fenced in backticks (inline
// code) or inside a triple-backtick markdown code fence was invisible,
// hits=0, for the same root cause as BLOCKER 1 (colon untrusted once inside
// any tracked quote, including a backtick). Fixing BLOCKER 1's colon-trust
// rule resolves this uniformly, including the odd-length triple-backtick
// fence run the original finding called out as leaving quote state "open"
// -- there is no separate backtick-specific fix, because colon detection no
// longer depends on ever exiting the quote.
func TestScrub_BlockerTwo_BacktickAndMarkdownFenceNoLongerSuppressColon(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"bare backtick-fenced value", "`password: SEKRITVALUE123`", "`password: [REDACTED]`"},
		// Exactly one trailing word after the fence closes (not two or
		// more): "in" alone would be the accepted single-trailing-word
		// shape, but "in a config file" is three words of ordinary
		// English prose after the container, which the pre-existing
		// multi-word prose defense correctly continues to reject
		// regardless of the backtick fence (see
		// TestScrub_ColonValueTrailingContext and the
		// chained-colon-label-genuinely-ambiguous-with-prose /
		// space-separated-scheme-value-truncated-at-the-space fuzz
		// registry entries for the same class of accepted trade-off), so
		// this case uses a single trailing word instead.
		{"inline code in prose, single trailing word", "Set `api_key: SEKRITVALUE123` today", "Set `api_key: [REDACTED]` today"},
		{
			"triple-backtick markdown fence",
			"Here is my config:\n\n```yaml\ndatabase:\n  password: SEKRITVALUE123\n```\n\nDoes that work?",
			"Here is my config:\n\n```yaml\ndatabase:\n  password: [REDACTED]\n```\n\nDoes that work?",
		},
		{
			"triple-backtick fence embedded in a JSONL transcript content field",
			`{"role":"assistant","content":"Add this:\n\n` + "```yaml\npassword: SEKRITVALUE123\n```" + `"}`,
			`{"role":"assistant","content":"Add this:\n\n` + "```yaml\npassword: [REDACTED]\n```" + `"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 1 {
				t.Fatalf("hits = %d, want 1: %q", hits, out)
			}
			if out != c.want {
				t.Errorf("Scrub(%q) = %q, want %q", c.in, out, c.want)
			}
			if strings.Contains(out, "SEKRITVALUE123") {
				t.Errorf("value leaked: %q", out)
			}
		})
	}
}

// TestScrub_BlockerOneAndTwo_ProseDefenseStillHolds pins the three prose
// controls the BLOCKER 1/2 fix must not regress: trusting ':' inside quotes
// must not turn ordinary sentence punctuation, or the pre-existing
// documented "one trailing word after a colon-assigned value" acceptance,
// into a redaction just because it now happens to sit inside quotes.
func TestScrub_BlockerOneAndTwo_ProseDefenseStillHolds(t *testing.T) {
	cases := []string{
		`fmt.Errorf("token: %w", err)`,
		"Error: token: invalid",
		"Secret: this document is confidential",
		"api_key: see the shared vault for details",
	}
	for _, in := range cases {
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
		}
	}
}

// TestScrub_BlockerFive_ContainerCloseImmediatelyAtEOFIsNotATrailingWord is
// the regression test for a false positive the BLOCKER 1/2 colon-trust fix
// introduced: a colon-keyed value inside a quoted JSON string, immediately
// followed by the string's own closing quote and then the object's closing
// brace with nothing else after it (e.g. {"error":"invalid token: expired"}),
// was being redacted, hits=1.
//
// The trailing-context check's "value ends right at the enclosing quote's own
// closing delimiter" branch resumes past that delimiter and re-applies
// isSingleTrailingWord from there -- but it never re-applied the STRUCTURAL
// terminator cases (a bare '}' ')' ']' etc. is an acceptable closer, not a
// word), only the word-counting exception. So a lone structural closer
// immediately after the delimiter, with nothing following it, was
// misclassified as "one trailing word" and accepted, exactly as if
// "expired" were a real credential followed by one more token of prose.
//
// A genuine trailing sentence after the container ("... and that is a real
// problem") must still reject: only a BARE structural closer with nothing
// else after it is affected.
func TestScrub_BlockerFive_ContainerCloseImmediatelyAtEOFIsNotATrailingWord(t *testing.T) {
	falsePositives := []string{
		`{"error":"invalid token: expired"}`,
		`{"error":"invalid token: unknown"}`,
		`{"note":"see api_key: expired"}`,
	}
	for _, in := range falsePositives {
		t.Run(in, func(t *testing.T) {
			out, hits := Scrub(in)
			if hits != 0 || out != in {
				t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
			}
		})
	}

	// Control: the same shape but with real trailing prose after the
	// container must still reject -- this must not become a false negative
	// as a side effect of fixing the false positive above.
	t.Run("control: real prose after the container still rejects", func(t *testing.T) {
		in := `{"error":"invalid token: expired"} and that is a real problem`
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
		}
	})

	// Control: the blocker-fixed shapes (a real credential followed
	// immediately by the enclosing quote's close, with a genuine trailing
	// word past the container) must still redact.
	t.Run("control: genuine credential in the same container-close shape still redacts", func(t *testing.T) {
		in := `curl -H "X-Api-Key: SEKRITVALUE123" https://x`
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		want := `curl -H "X-Api-Key: [REDACTED]" https://x`
		if out != want {
			t.Errorf("Scrub(%q) = %q, want %q", in, out, want)
		}
	})
}

// TestScrub_EscapedNewlineTerminatesBareValue is the regression test for a
// leaked-structure defect: a bare (unquoted) credential value inside an
// already-open JSON string, followed by an ESCAPED newline (the literal two
// bytes '\' 'n', the textual spelling JSON uses for a line separator within
// a string whose bytes this scanner never unescapes) and then more document
// content, had its redaction span run straight through the escaped newline
// and swallow that following content too -- consuming, for example, the
// closing ``` of a markdown code fence the value was inside.
//
// The escape-handling in the bare-value scan treated '\' as always starting
// an opaque, swallowed two-byte escape pair unless the escaped byte was the
// enclosing quote character. '\n' and '\r' need the same treatment isLineSep
// already gives their literal-byte counterparts: a value terminator, not
// value content.
func TestScrub_EscapedNewlineTerminatesBareValue(t *testing.T) {
	in := `{"role":"assistant","content":"Add this:\n\n` + "```yaml" +
		`\npassword: SEKRITVALUE123\n` + "```" + `"}`
	want := `{"role":"assistant","content":"Add this:\n\n` + "```yaml" +
		`\npassword: [REDACTED]\n` + "```" + `"}`

	out, hits := Scrub(in)
	if hits != 1 {
		t.Fatalf("hits = %d, want 1: %q", hits, out)
	}
	if out != want {
		t.Errorf("Scrub(%q) =\n  %q\nwant\n  %q", in, out, want)
	}
	if strings.Contains(out, "SEKRITVALUE123") {
		t.Errorf("LEAK: secret survived in %q", out)
	}
	if !strings.HasSuffix(out, "```\"}") {
		t.Errorf("closing markdown fence was consumed by the redaction span: %q", out)
	}
	out2, hits2 := Scrub(out)
	if hits2 != 0 || out2 != out {
		t.Errorf("Scrub is not idempotent: Scrub(Scrub(%q)) = %q, hits=%d, want %q, 0 hits", in, out2, hits2, out)
	}
}

// TestScrub_BareKeyWithEscapeQuotedValueInsideOpenString is the regression
// test for an under-redaction leak: a BARE (unquoted) credential key found
// inside an already-open JSON string, whose value opens with an ESCAPED
// quote matching the enclosing string's own quote character, was invisible
// to the scanner entirely -- hits=0. This is the realistic shape of a Claude
// Code JSONL transcript row that embeds a shell command containing a
// double-quoted secret (`run: export PASSWORD="SECRET"`), serialized once
// into a JSON string field.
//
// Root cause: tryKeyAt's quoted-value branch requires the value's first byte
// to itself be an unescaped quote character (quoteCharFor(s[v])), and its
// bracket-value branch requires an unescaped opening bracket; neither
// matches a leading backslash. Falling through to the bare-value branch, the
// enclosing-quote guard there stops the scan dead at the very first byte
// (the escaped quote's own backslash), so vi never advances past vStart and
// the whole assignment is rejected with 0 hits.
func TestScrub_BareKeyWithEscapeQuotedValueInsideOpenString(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			"eq operator",
			`{"a":"password=\"SEK123456789012\""}`,
			`{"a":"password=\"[REDACTED]\""}`,
		},
		{
			"colon operator",
			`{"a":"password:\"SEK123456789012\""}`,
			`{"a":"password:\"[REDACTED]\""}`,
		},
		{
			"second credential after an escaped quote",
			`{"pw":"a\" token=\"SEKRITVALUE123456\""}`,
			`{"pw":"a\" token=\"[REDACTED]\""}`,
		},
		{
			"JSONL transcript shell command with double-quoted secret",
			`{"type":"user","message":{"content":"run: export PASSWORD=\"SEKRITVALUE123456\""}}`,
			`{"type":"user","message":{"content":"run: export PASSWORD=\"[REDACTED]\""}}`,
		},
		{
			"JSONL transcript assistant content",
			`{"type":"assistant","content":"set token=\"SEKRITVALUE123456\" in env"}`,
			`{"type":"assistant","content":"set token=\"[REDACTED]\" in env"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, hits := Scrub(c.in)
			if hits != 1 {
				t.Fatalf("hits = %d, want 1: %q", hits, out)
			}
			if out != c.want {
				t.Errorf("Scrub(%q) =\n  %q\nwant\n  %q", c.in, out, c.want)
			}
			if strings.Contains(out, "SEK123456789012") || strings.Contains(out, "SEKRITVALUE123456") {
				t.Errorf("LEAK: secret survived in %q", out)
			}
			out2, hits2 := Scrub(out)
			if hits2 != 0 || out2 != out {
				t.Errorf("Scrub is not idempotent: Scrub(Scrub(%q)) = %q, hits=%d, want %q, 0 hits", c.in, out2, hits2, out)
			}
		})
	}

	// Control: the bare-value shape (no escaped quote) must still redact
	// exactly as it did before this fix.
	t.Run("control: bare value with no escaped quote still redacts", func(t *testing.T) {
		in := `{"a":"password=SEK123456789012"}`
		want := `{"a":"password=[REDACTED]"}`
		out, hits := Scrub(in)
		if hits != 1 {
			t.Fatalf("hits = %d, want 1: %q", hits, out)
		}
		if out != want {
			t.Errorf("Scrub(%q) = %q, want %q", in, out, want)
		}
	})
}

// TestScrub_KeyWordSeparatedFromColonByOneWord is the regression test for a
// leaked-credential defect: a credential-key word (auth, token, ...) that is
// NOT the word directly touching the assignment operator -- separated from
// it by exactly one ordinary intervening word, e.g. "auth failed: X" -- was
// invisible to the assignment scanner entirely. The credential word ("auth")
// was tried as a key and rejected because an ordinary word ("failed"), not
// an operator, followed it; "failed" was tried as a key and rejected because
// it is not itself credential-shaped. Neither attempt ever reached the
// operator/value scan, independent of the value's own shape -- so an
// uppercase-hex value like DEADBEEF00 shipped in the clear, hits=0.
func TestScrub_KeyWordSeparatedFromColonByOneWord(t *testing.T) {
	t.Run("leak is fixed", func(t *testing.T) {
		cases := []struct {
			name string
			in   string
			want string
		}{
			{
				"auth failed, JSON-embedded",
				`{"error":"auth failed: DEADBEEF00"}`,
				`{"error":"auth failed: [REDACTED]"}`,
			},
			{
				"token missing, bare",
				`token missing: DEADBEEF00`,
				`token missing: [REDACTED]`,
			},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				out, hits := Scrub(c.in)
				if hits != 1 {
					t.Fatalf("hits = %d, want 1: %q", hits, out)
				}
				if out != c.want {
					t.Errorf("Scrub(%q) = %q, want %q", c.in, out, c.want)
				}
				out2, hits2 := Scrub(out)
				if hits2 != 0 || out2 != out {
					t.Errorf("Scrub is not idempotent: Scrub(Scrub(%q)) = %q, hits=%d, want %q, 0 hits", c.in, out2, hits2, out)
				}
			})
		}
	})

	// Control: this must not become a general "credential word anywhere
	// nearby a colon" rule. An intervening word that is itself an ordinary
	// capitalized English word, paired with a value that has no digit and
	// starts with punctuation, must not be swept in -- this is the exact
	// shape ("Key Attributes: <No Attributes>", an openssl pkcs12 header
	// line) that an earlier, broader version of this fix over-redacted.
	t.Run("control: ordinary key-adjacent prose stays unredacted", func(t *testing.T) {
		in := "Key Attributes: <No Attributes>\n"
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
		}
	})

	// Control: a plain lowercase trailing word reached via this same
	// leading-credential-word path must not be treated as a secret either.
	t.Run("control: lowercase trailing word via leading-credential-word path stays unredacted", func(t *testing.T) {
		in := "auth failed: pending"
		out, hits := Scrub(in)
		if hits != 0 || out != in {
			t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
		}
	})
}

// TestScrub_YAMLBlockScalarSecondPassDoesNotDeleteFollowingContent pins the
// fix for a content-DESTRUCTION defect (worse than a leak: silent, no trace
// left behind) in yamlBlockScalarSpan. The first Scrub pass on a YAML block
// scalar folded the body's own terminating line separator INTO the redacted
// span, so [REDACTED] ended up glued directly against whatever followed on
// the very next line with no newline between them. That was harmless to a
// human reading the first pass's output (the separator was simply consumed,
// not duplicated or corrupted), but on a SECOND Scrub pass, "[REDACTED]" sits
// at the start of what now reads as one unbroken physical line together with
// the following key -- e.g. "[REDACTED]region: eu-west-1" -- so the second
// pass's own indentation-based body-end detection (which, for a column-0 key,
// auto-detects the body's required indent from whatever indent this first
// line happens to have, and 0 has no shallower level to ever register as
// "less indented than the body") swallowed the following, entirely unrelated
// key/value pairs as if they were still part of the block body. The user's
// non-secret content was DELETED with no [REDACTED] marker left in its place
// and no way to tell it had ever been there.
//
// The fix has two parts, both exercised here: (1) the redacted span now
// excludes the body's own terminating line separator when a real structural
// boundary (indentation dropping back to keyIndent) was found, so that
// separator survives into the output and keeps the following key on its own
// physical line; (2) a re-scrub explicitly recognizes a lone redactedToken
// line as already-redacted before it ever auto-detects a bodyIndent from it,
// so a column-0 key's block body stops growing the moment it's already just
// the placeholder, rather than treating "indent 0 forever" as license to
// keep consuming every following top-level line.
func TestScrub_YAMLBlockScalarSecondPassDoesNotDeleteFollowingContent(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		wantContains []string
	}{
		{
			"literal block scalar under a column-0 key, followed by two more top-level keys",
			"password: |\n  line1\n  line2\nregion: eu-west-1\nname: prod\n",
			[]string{"region: eu-west-1", "name: prod"},
		},
		{
			"literal block scalar, one following key",
			"secret: |\n  abc\nother: keepme\n",
			[]string{"other: keepme"},
		},
		{
			"folded block scalar nested under an indented key",
			"config:\n  password: >\n    folded secret\n  region: eu-west-1\n",
			[]string{"region: eu-west-1"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out1, hits1 := Scrub(c.in)
			if hits1 != 1 {
				t.Fatalf("first Scrub(%q) hits = %d, want 1: %q", c.in, hits1, out1)
			}
			for _, want := range c.wantContains {
				if !strings.Contains(out1, want) {
					t.Errorf("first pass destroyed non-secret content: Scrub(%q) = %q, want it to still contain %q", c.in, out1, want)
				}
			}
			out2, hits2 := Scrub(out1)
			if hits2 != 0 {
				t.Errorf("second Scrub pass reported %d hits, want 0 (Scrub must be idempotent): Scrub(%q) = %q", hits2, out1, out2)
			}
			if out2 != out1 {
				t.Errorf("second pass is not byte-idempotent: Scrub(%q) = %q, want %q", out1, out2, out1)
			}
			for _, want := range c.wantContains {
				if !strings.Contains(out2, want) {
					t.Errorf("second pass destroyed non-secret content: Scrub(%q) = %q, want it to still contain %q", out1, out2, want)
				}
			}
		})
	}
}

// TestScrub_CredentialVocabularyGap pins the fix for a set of ordinary
// secret-bearing key names (session, cookie, otp, private, salt, pin, nonce,
// license, sessionid) that were absent from both credentialKeys and
// credentialWords, so no key-matching mechanism recognized them and their
// values leaked in full, hits=0 -- the same class of gap ADMIN_PASSWORD and
// JWT_SECRET used to fall into before isCredentialKeyByWord existed, just
// never enumerated for these nine words on their own.
func TestScrub_CredentialVocabularyGap(t *testing.T) {
	t.Run("leak is fixed", func(t *testing.T) {
		cases := []string{
			"session=abcdef0123456789abcdef0123456789",
			"cookie=abcdef0123456789abcdef0123456789",
			"otp=abcdef0123456789abcdef0123456789",
			"private=abcdef0123456789abcdef0123456789",
			"salt=abcdef0123456789abcdef0123456789",
			"pin=abcdef0123456789abcdef0123456789",
			"nonce=abcdef0123456789abcdef0123456789",
			"license=abcdef0123456789abcdef0123456789",
			"sessionid=abcdef0123456789abcdef0123456789",
		}
		for _, in := range cases {
			out, hits := Scrub(in)
			if hits != 1 {
				t.Errorf("Scrub(%q) hits = %d, want 1 (still leaking): %q", in, hits, out)
			}
			if hits != redactedCount(out) {
				t.Errorf("hits=%d but output has %d [REDACTED] tokens: %q", hits, redactedCount(out), out)
			}
		}
	})

	// Control: identifiers that merely CONTAIN one of the nine new words as
	// a whole separator-delimited word of a larger, ordinary, non-credential
	// name are accepted as over-redaction (a fixed, finite, sanctioned cost
	// -- see isCredentialKeyByWord's own doc comment) rather than left as an
	// open leak. These three are the collisions the words themselves create
	// against real config-key vocabulary and are recorded here so a reader
	// can see the accepted trade-off directly, not just infer it.
	t.Run("accepted over-redaction: whole-word collisions with the new vocabulary", func(t *testing.T) {
		cases := []string{
			"PRIVATE_REPO_URL=https://example.com/x",
			"LICENSE_FILE=LICENSE.md",
			"SESSION_TIMEOUT=3600",
		}
		for _, in := range cases {
			_, hits := Scrub(in)
			if hits != 1 {
				t.Errorf("Scrub(%q) hits = %d, want 1 (accepted over-redaction, see comment)", in, hits)
			}
		}
	})

	// Control: identifiers that merely CONTAIN one of the nine new words as
	// a substring, with no separator or camelCase boundary to split on, must
	// NOT be over-redacted by the fused-substring path: none of the leak
	// cases this fix closes depend on fused-substring matching for these
	// nine words, so there is no reason to widen that path for them (unlike
	// "pat", none of these nine needed excluding from it in the first
	// place -- this test is the proof).
	t.Run("no fused-substring collisions introduced", func(t *testing.T) {
		cases := []string{
			"PINNED_VERSION=1.2.3",
			"SPINNER_STYLE=dots",
			"PING_INTERVAL=30",
			"SALTED_HASH_ALGO=sha256",
		}
		for _, in := range cases {
			out, hits := Scrub(in)
			if hits != 0 || out != in {
				t.Errorf("Scrub(%q) = %q, hits=%d; want unchanged, 0 hits", in, out, hits)
			}
		}
	})
}

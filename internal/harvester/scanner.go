package harvester

import "strings"

// credentialKeys is the closed set of collapsed (lowercase, separators
// stripped) identifiers treated as credential-shaped. Membership is by
// whole-identifier equality only -- never substring or suffix -- so that
// words which merely contain a credential fragment (monkey, bypass, whisk,
// kiosk, risk, task, disk) never match.
//
// This closed set has no fallback of its own for an ordinary secret key
// name it does not enumerate (ADMIN_PASSWORD, JWT_SECRET, RAILS_MASTER_KEY,
// STRIPE_SECRET_KEY, and any other real-world name built by prefixing an
// ordinary credential word): those leaked in full, hits=0, until
// isCredentialKeyByWord (below) was added as a second, additive check. See
// that function's doc comment for how it stays inside the same
// whole-word discipline this set's own comment describes, rather than
// widening into substring matching (which would reopen the exact
// monkey/bypass/... false positives this paragraph warns about).
var credentialKeys = map[string]bool{
	"password":           true,
	"passwd":             true,
	"pass":               true,
	"token":              true,
	"secret":             true,
	"key":                true,
	"apikey":             true,
	"accesstoken":        true,
	"clientsecret":       true,
	"secretkey":          true,
	"refreshtoken":       true,
	"authtoken":          true,
	"pgpassword":         true,
	"anthropicapikey":    true,
	"privatekey":         true,
	"dbpassword":         true,
	"awssecretaccesskey": true,
	"passphrase":         true,
	"pwd":                true,
	"sessiontoken":       true,
	"apisecret":          true,
	"auth":               true,
	"credential":         true,
	"credentials":        true,
	"pat":                true,
	"signingkey":         true,
	"encryptionkey":      true,
	"serviceaccountkey":  true,
	"connectionstring":   true,
	"dsn":                true,
	"xapikey":            true,
	"bearer":             true,
	"authorization":      true,
	// oauth2_token collapses to "oauth2token": the "2" sits mid-identifier
	// (OAuth2 is the protocol name, not a version suffix), so trailing-
	// digit stripping in collapseKey does not reach it. Listed explicitly
	// rather than generalizing digit-stripping to the whole identifier,
	// which would risk the same suffix-matching trap credentialKeys is
	// designed to avoid.
	"oauth2token": true,
	// The following nine are ordinary secret-bearing key names absent from
	// this set even though the design already treats their SHAPE (a
	// credential-shaped key, unconditionally redacted regardless of value)
	// as covered: session=, cookie=, otp=, private=, salt=, pin=, nonce=,
	// license=, sessionid= all leaked in full, hits=0, for lack of a
	// vocabulary entry -- the identical gap ADMIN_PASSWORD/JWT_SECRET/etc.
	// closed for compounds, just never enumerated for these nine atomic
	// words on their own. Each is also a member of credentialWords below
	// (see that set's own doc comment for why atomic-word membership is a
	// separate list from this one), so isCredentialKeyByWord recognizes
	// them as a whole separator/camelCase-delimited word of a larger
	// identifier too (SESSION_TOKEN, USER_PRIVATE_KEY, LICENSE_KEY), not
	// only as an exact whole-identifier match.
	"session":   true,
	"cookie":    true,
	"otp":       true,
	"private":   true,
	"salt":      true,
	"pin":       true,
	"nonce":     true,
	"license":   true,
	"sessionid": true,
}

// isIdentByte reports whether b can be part of a credential-key identifier
// (letters, digits, underscore, hyphen).
func isIdentByte(b byte) bool {
	return b == '_' || b == '-' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// collapseKey lowercases s, strips '_' and '-' separators, and strips any
// run of trailing digits, so that PGPASSWORD, pg_password, pg-password,
// and api_key2 (a common versioned-secret naming convention) all normalize
// identically. Trailing-digit stripping stays inside the whole-identifier
// discipline: it changes what "the whole identifier" collapses to, but
// membership in credentialKeys is still exact equality against that
// result, never a substring or suffix match on the original.
func collapseKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || c == '-' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	out := b.String()
	end := len(out)
	for end > 0 && out[end-1] >= '0' && out[end-1] <= '9' {
		end--
	}
	if end == 0 {
		// The identifier was all digits; stripping everything would make
		// it match on an empty string, which is meaningless. Leave it
		// untouched (it will simply fail the credentialKeys lookup).
		return out
	}
	return out[:end]
}

// isCredentialKey reports whether ident (a run of isIdentByte characters)
// names a credential-shaped key.
func isCredentialKey(ident string) bool {
	return credentialKeys[collapseKey(ident)]
}

// credentialWords is the subset of credentialKeys whose members are a
// single atomic word rather than a multi-word compound (contrast
// "password" with "connectionstring" or "pgpassword"). It is the set
// isCredentialKeyByWord matches a WHOLE WORD of ident against -- see that
// function's doc comment for why the distinction between "atomic word" and
// "compound" matters and cannot simply reuse credentialKeys wholesale.
var credentialWords = map[string]bool{
	"password":      true,
	"passwd":        true,
	"pass":          true,
	"token":         true,
	"secret":        true,
	"key":           true,
	"apikey":        true,
	"privatekey":    true,
	"passphrase":    true,
	"pwd":           true,
	"auth":          true,
	"credential":    true,
	"credentials":   true,
	"pat":           true,
	"dsn":           true,
	"xapikey":       true,
	"bearer":        true,
	"authorization": true,
	// See credentialKeys' own comment on these same nine entries: added
	// together as one vocabulary-gap fix, and included here so each also
	// matches as a whole separator/camelCase-delimited word inside a larger
	// identifier (SESSION_TOKEN, USER_PRIVATE_KEY), not only via an exact
	// whole-identifier match.
	"session":   true,
	"cookie":    true,
	"otp":       true,
	"private":   true,
	"salt":      true,
	"pin":       true,
	"nonce":     true,
	"license":   true,
	"sessionid": true,
}

// splitIdentWords splits ident into its component words on '_'/'-'
// separators and camelCase boundaries (a lowercase-or-digit byte followed
// by an uppercase byte), lowercasing each word and stripping its trailing
// digit run the same way collapseKey does for the whole identifier. This
// is deliberately NOT the same operation as collapseKey: collapseKey fuses
// the whole identifier into one string for exact-equality membership
// (needed for compounds like "pg_password" -> "pgpassword", which is not
// two words glued from a credential vocabulary but one, PGPASSWORD, that
// only ever appears as a single field name); splitIdentWords instead keeps
// each word separate, which is what lets a compound identifier like
// RAILS_MASTER_KEY be recognized via its OWN "key" word without also
// recognizing "monkey" (one fused word, never split, with no separator or
// case boundary anywhere in it) via a bare substring match on "key".
func splitIdentWords(ident string) []string {
	var words []string
	start := 0
	flush := func(end int) {
		if end <= start {
			return
		}
		splitWordCamel(ident[start:end], &words)
	}
	for i := 0; i < len(ident); i++ {
		if ident[i] == '_' || ident[i] == '-' {
			flush(i)
			start = i + 1
		}
	}
	flush(len(ident))
	return words
}

// splitWordCamel further splits w (a run with no '_'/'-' separators) on
// camelCase boundaries, appends each resulting piece to *words, lowercased
// with its trailing digit run stripped (mirroring collapseKey's own
// trailing-digit handling, so "apiKey2" and "api_key2" normalize the same
// way at the word level too).
func splitWordCamel(w string, words *[]string) {
	start := 0
	for i := 1; i < len(w); i++ {
		prev, cur := w[i-1], w[i]
		prevLower := (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9')
		curUpper := cur >= 'A' && cur <= 'Z'
		if prevLower && curUpper {
			*words = append(*words, lowerTrimTrailingDigits(w[start:i]))
			start = i
		}
	}
	*words = append(*words, lowerTrimTrailingDigits(w[start:]))
}

// lowerTrimTrailingDigits lowercases w and strips any run of trailing
// digits, the per-word mirror of collapseKey's whole-identifier handling.
func lowerTrimTrailingDigits(w string) string {
	var b strings.Builder
	b.Grow(len(w))
	for i := 0; i < len(w); i++ {
		c := w[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	out := b.String()
	end := len(out)
	for end > 0 && out[end-1] >= '0' && out[end-1] <= '9' {
		end--
	}
	if end == 0 {
		return out
	}
	return out[:end]
}

// isCredentialKeyByWord reports whether ident, when split into its
// component words (splitIdentWords), contains a word that is itself a
// whole member of credentialWords -- e.g. RAILS_MASTER_KEY ("rails",
// "master", "key") matches on "key"; ADMIN_PASSWORD ("admin", "password")
// matches on "password".
//
// This is additive to, and deliberately narrower than, isCredentialKey:
// isCredentialKey's exact whole-identifier match already covers every
// multi-word COMPOUND in credentialKeys (connectionstring, pgpassword,
// secretkey, refreshtoken, and so on) that this per-word check cannot
// reach, since none of those compounds' individual words are themselves
// credential-shaped ("connection" and "string" alone are not). This check
// exists for the opposite, previously-uncovered gap: an ordinary
// underscore/hyphen/camelCase-separated identifier whose credential word is
// a real, separate word of the name rather than the WHOLE name --
// ADMIN_PASSWORD, JWT_SECRET, STRIPE_SECRET_KEY -- which used to leak in
// full because "adminpassword", "jwtsecret" and "stripesecretkey" are not,
// and never will be, members of a closed exact-match set enumerating every
// possible prefix.
//
// The whole-WORD discipline is what keeps this from re-opening the exact
// false-positive credentialKeys' own doc comment warns about: "monkey" has
// no separator and no camelCase boundary anywhere in it, so
// splitIdentWords never breaks it into anything smaller than the single
// word "monkey", which is not equal to "key" (only a substring of it would
// be, and this checks whole-word equality, never substring). The same
// holds for bypass, whisk, kiosk, risk, task, and disk.
func isCredentialKeyByWord(ident string) bool {
	for _, w := range splitIdentWords(ident) {
		if credentialWords[w] {
			return true
		}
	}
	return false
}

// fusedSubstringExcludedWords is the subset of credentialWords excluded from
// isCredentialKeyByFusedSubstring's any-position scan specifically, even
// though they remain fully active for isCredentialKey and
// isCredentialKeyByWord (an exact identifier match or a whole separator/
// camelCase-delimited word still recognizes PAT=x, GITHUB_PAT=x, my_pat=x
// exactly as before).
//
// "pat" is the sole member: at 3 bytes it is short enough to appear as an
// ordinary substring of common, non-credential single-fused-word identifiers
// -- GOPATH (Go's module search path environment variable) is the concrete,
// required-to-stay-clean case this scrubber's own tests pin (GOPATH contains
// "pat" with 3 bytes to spare, comfortably past fusedSubstringMinExtraLen).
// No leak case this fused-substring matcher exists to catch
// (PASSWORDADMIN, SECRETJWT, KEYSTORE, MYPASSWORDVALUE, APPSECRETSTORE,
// XPASS, TOKENVALUE, AUTHDATA) depends on "pat" at all, so excluding it here
// costs nothing on the leak side while removing the one real false-positive
// this matcher would otherwise introduce.
var fusedSubstringExcludedWords = map[string]bool{
	"pat": true,
}

// fusedSubstringMinExtraLen is the minimum number of bytes required outside
// the matched credentialWords substring in isCredentialKeyByFusedSubstring
// (split between whatever sits before and after it, in any proportion). It
// exists so the fused word must be a real compound built AROUND a credential
// word (DBPASS, JWTSECRET, PASSWORDADMIN, KEYSTORE, XPASS) rather than the
// bare credential word on its own with nothing else in the identifier, which
// isCredentialKey / isCredentialKeyByWord already recognize through their own
// paths -- 1 byte is enough to make that distinction (XPASS's single leading
// "x" is exactly this floor; ident having ZERO extra bytes means it IS the
// bare word, already covered elsewhere).
const fusedSubstringMinExtraLen = 1

// isCredentialKeyByFusedSubstring reports whether ident, when collapsed the
// same way collapseKey does (lowercased, separators stripped, trailing digits
// stripped), is a SINGLE fused word -- no '_'/'-' separator and no camelCase
// boundary anywhere in it, i.e. splitIdentWords(ident) yields exactly one
// word -- that CONTAINS a member of credentialWords anywhere in it (prefix,
// middle, or suffix), with at least fusedSubstringMinExtraLen bytes of other
// content somewhere in the identifier alongside that word.
//
// This replaces an earlier, narrower version of this check that matched only
// a credential word at the END of the fused word (isCredentialKeyByFusedSuffix,
// DBPASS, JWTSECRET). That caught only one of the three positions a
// credential word can occupy inside a fused compound: PASSWORDADMIN and
// SECRETJWT have the credential word at the START; KEYSTORE, MYPASSWORDVALUE
// and APPSECRETSTORE have it in the MIDDLE. Each of those leaked in full,
// hits=0, under the suffix-only version, for the identical reason
// ADMINPASSWORD and JWTSECRET used to leak before that version existed: the
// closed exact-match set has no entry for them and never will, since
// enumerating every possible position a credential word might sit at inside
// an arbitrary compound is an open set. Matching the substring in any
// position, rather than adding a second and third position-specific
// function, is the design this round converges on: a key containing a
// credential word ANYWHERE is a credential key, full stop -- see this file's
// package-level design note in the CLAUDE.md-referenced brief for why no
// value shape makes such a key safe to ship in the clear.
//
// The single-fused-word restriction is unchanged from the suffix-only
// version and is what keeps this from re-opening the false positive
// credentialKeys' and isCredentialKeyByWord's own doc comments warn about:
// MONKEY_BARS, WHISKEY_BRAND, and BYPASS_CACHE all carry a '_' separator, so
// splitIdentWords already breaks each of them into two SEPARATE words
// ("monkey"+"bars", "whiskey"+"brand", "bypass"+"cache") before this function
// is ever reached, and isCredentialKeyByWord's own whole-word check already
// (correctly) rejects all six of those words on its own. This function is
// only reached by ident forms with NO separator and NO camelCase boundary
// anywhere -- true single-word compounds like ADMINPASSWORD, PASSWORDADMIN,
// KEYSTORE, XPASS -- so it never has an opportunity to re-split MONKEY_BARS
// or WHISKEY_BRAND differently than isCredentialKeyByWord already does.
//
// Unlike the suffix-only version, a match here is treated exactly like an
// isCredentialKey exact match: redaction is unconditional, with no
// value-shape gate. See tryKeyAt's own doc comment for why gating this path
// specifically (rather than the atomic-key path) was backwards: the key
// evidence -- a credential word appearing literally inside the identifier --
// is not weaker just because of where in the fused word it sits, and no
// value shape makes ADMIN_PASSWORD= or PASSWORDADMIN= or KEYSTORE= safe to
// ship to a third party. The accepted cost is that KEYSTORE, BYPASS, XPASS
// and similar over-redact; that is the sanctioned direction, a fixed finite
// cost, not an open leak surface.
func isCredentialKeyByFusedSubstring(ident string) bool {
	words := splitIdentWords(ident)
	if len(words) != 1 {
		return false
	}
	w := words[0]
	for word := range credentialWords {
		if fusedSubstringExcludedWords[word] {
			continue
		}
		if len(w) < len(word)+fusedSubstringMinExtraLen {
			continue
		}
		if strings.Contains(w, word) {
			return true
		}
	}
	return false
}

// quoteState tracks which quote (if any) the scanner is currently inside.
type quoteState byte

const (
	quoteNone quoteState = iota
	quoteSingle
	quoteDouble
	quoteBacktick
)

func quoteCharFor(b byte) quoteState {
	switch b {
	case '\'':
		return quoteSingle
	case '"':
		return quoteDouble
	case '`':
		return quoteBacktick
	}
	return quoteNone
}

// topLevelQuoteOpenerFor is quoteCharFor restricted to the quote characters
// the OUTER scanner loop (scanCredentialAssignments, at quoteNone) may treat
// as opening tracked quote state. It deliberately excludes the single quote:
// unlike the double quote (the JSON/YAML key-quoting character) and the
// backtick (a value-only wrapper that never collides with ordinary prose),
// a single quote is routine English punctuation (apostrophes: "it's",
// "don't") with no structural role at top level. Before this exclusion, one
// prose apostrophe anywhere in the document opened quoteSingle and never
// found a matching close, leaving the scanner "inside a string" for the
// entire remainder of the input -- silently disabling colon-keyed
// assignment detection (only trusted at top level) and, worse, swallowing
// the double quote as an inert byte instead of the character that opens
// JSON key/value recognition, making an entire doubly-escaped JSONL
// transcript blob invisible.
//
// This does not weaken single-quoted VALUE detection: password='x' is
// still fully handled, because tryKeyAt's own value scan re-derives the
// quote character locally from quoteCharFor(s[v]) once it has already found
// the key and operator, independent of the outer q the top-level loop
// tracks. The outer loop never needed to open quoteSingle for that path to
// work; it only ever caused the poisoning above.
func topLevelQuoteOpenerFor(b byte) quoteState {
	if b == '\'' {
		return quoteNone
	}
	return quoteCharFor(b)
}

// bracketCloseFor reports the closing character for a value wrapped in
// braces, brackets, or parens (e.g. password={SECRET}, password=[SECRET],
// password=(SECRET)), and whether b opens one. These are structurally like
// quotes -- a delimited value whose interior should be redacted while the
// delimiters themselves are preserved -- but unlike quotes they are not
// tracked by the outer quote-state machine, since {, [, ( are also
// legitimate structural bytes (JSON/YAML containers, function calls)
// everywhere else in the scanner.
func bracketCloseFor(b byte) (close byte, ok bool) {
	switch b {
	case '{':
		return '}', true
	case '[':
		return ']', true
	case '(':
		return ')', true
	}
	return 0, false
}

// httpAuthSchemes is the closed set of HTTP authentication scheme words that
// may prefix a bare credential value, space-separated, in an assignment like
// authorization=Basic <base64> or api_key=Bearer <token>. Matching is
// case-sensitive against these exact spellings (RFC 7235's registered
// scheme names), which is deliberately narrower than case-insensitive: the
// goal is only to stop the ordinary single interior space of this one
// well-known shape from prematurely ending the bare-value scan, not to
// second-guess every space in an arbitrary value.
var httpAuthSchemes = []string{"Basic", "Bearer", "Digest", "Negotiate", "NTLM"}

// bareValueSchemeSpan reports whether s, starting at valueStart, begins with
// a recognized HTTP auth scheme word (httpAuthSchemes) followed by exactly
// one space and then at least one more bare-value byte, and if so returns
// the offset of that one space -- the position isBareValueByte's ordinary
// scan must be told to treat as still-part-of-the-value rather than a
// terminator.
//
// Without this, "authorization=Basic QWxhZGRpbjpvcGVuc2VzYW1l==" redacted
// only the scheme word: the bare-value scan (which stops at the first
// space) read "Basic" as the whole value and left the base64 credential
// after the space in the clear while hits still reported 1 -- the scheme
// word is not itself a secret, so redacting only it and shipping the real
// credential verbatim is the exact under-redaction this scrubber exists to
// prevent. The capitalized header form ("Authorization: Bearer ...") is
// already covered by an independent vendor regex in scrubber.go; this closes
// the gap for the lowercase/'='-assignment shape the credential-assignment
// scanner handles on its own.
//
// The match is intentionally narrow (a single interior space, one scheme
// word from a closed list) rather than a generic "allow any space in a bare
// value" relaxation, which would reopen the exact multi-word-prose false
// positive ("Secret: this document is confidential") the bare-value
// terminator rule exists to reject.
func bareValueSchemeSpan(s string, valueStart int) (spaceAt int, ok bool) {
	for _, scheme := range httpAuthSchemes {
		if !strings.HasPrefix(s[valueStart:], scheme) {
			continue
		}
		spacePos := valueStart + len(scheme)
		if spacePos >= len(s) || s[spacePos] != ' ' {
			continue
		}
		afterSpace := spacePos + 1
		if afterSpace < len(s) && isBareValueByte(s[afterSpace]) {
			return spacePos, true
		}
	}
	return 0, false
}

// isLineSep reports whether the byte at i starts a line separator, and
// returns the number of bytes the separator occupies (2 for CRLF, else 1).
// Recognized separators: CRLF, lone LF, lone CR, form feed (0x0C), vertical
// tab (0x0B).
func isLineSep(s string, i int) (width int, ok bool) {
	if i >= len(s) {
		return 0, false
	}
	switch s[i] {
	case '\n':
		return 1, true
	case '\r':
		if i+1 < len(s) && s[i+1] == '\n' {
			return 2, true
		}
		return 1, true
	case '\f', '\v':
		return 1, true
	}
	return 0, false
}

// isHSpace reports whether b is horizontal whitespace (space or tab), which
// is allowed around the assignment operator and does not cross a line.
func isHSpace(b byte) bool { return b == ' ' || b == '\t' }

// isChainedLabelByte reports whether b is an assignment operator. If the
// non-space byte immediately preceding a candidate key is one of these, the
// key MAY be a value fragment chained after an earlier bare "word:" or
// "word=" label on the same line (e.g. "Error: token: invalid", where
// "token" follows the already-terminated label "Error:") rather than a
// fresh assignment. Whether it actually is one is decided in tryKeyAt,
// which also looks at the CURRENT key's own operator: see the chained-label
// check there for why '=' is not credible sentence punctuation and is
// exempted when the current key's own operator is '='.
func isChainedLabelByte(b byte) bool {
	return b == ':' || b == '='
}

// looksLikeFreshKeyAhead reports whether s, starting at i, looks like the
// start of an unrelated fresh "key=" or "key:" assignment after skipping
// horizontal whitespace: an identifier run immediately followed (across
// more horizontal whitespace) by '=' or ':'. It is a narrow, bounded
// lookahead used only to break the tie in one genuinely ambiguous case: a
// quoted value that contains what its own escaping rules say is an escaped
// (non-terminating) quote, immediately followed by what looks like a
// second, distinct credential assignment (e.g. `password="a\" token="X"`,
// where continuing to treat the escaped quote as non-terminating would
// swallow the second field whole). It deliberately does not require the
// identifier to be credential-shaped: any key= shape immediately after is
// enough to prefer stopping the current value early (the safe,
// over-redaction direction) rather than risk consuming an unrelated field.
func looksLikeFreshKeyAhead(s string, i int) bool {
	for i < len(s) && isHSpace(s[i]) {
		i++
	}
	start := i
	for i < len(s) && isIdentByte(s[i]) {
		i++
	}
	if i == start {
		return false
	}
	for i < len(s) && isHSpace(s[i]) {
		i++
	}
	if i >= len(s) {
		return false
	}
	return s[i] == '=' || s[i] == ':'
}

// isSingleTrailingWord reports whether s, starting at i (already past the
// horizontal whitespace following a colon-assigned bare value), consists of
// exactly one run of non-space bytes and then nothing but EOF or a line
// separator -- the narrow exception to the colon trailing-context check in
// tryKeyAt that lets "token: abc123xyz sk-aaaa...aaaa" redact its first
// value on the very first Scrub pass instead of only converging after a
// second pass (see the call site for the full rationale).
//
// It deliberately does NOT accept a second word after more horizontal
// space: two or more trailing words is exactly the multi-word prose shape
// ("Secret: this document is confidential") the surrounding check exists to
// reject, and this exception must not reopen it.
func isSingleTrailingWord(s string, i int) bool {
	j := i
	for j < len(s) && !isHSpace(s[j]) {
		if _, lineOK := isLineSep(s, j); lineOK {
			break
		}
		j++
	}
	if j == i {
		return false
	}
	for j < len(s) && isHSpace(s[j]) {
		j++
	}
	if j >= len(s) {
		return true
	}
	_, lineOK := isLineSep(s, j)
	return lineOK
}

// isPlainLowercaseWord reports whether s[start:end] consists entirely of
// lowercase ASCII letters (a-z) and is non-empty -- the shape of an ordinary
// dictionary word ("expired", "unknown", "invalid", "missing",
// "confidential"), as opposed to a real credential value. Any digit,
// uppercase letter, or punctuation byte disqualifies it: a genuine secret is
// essentially never a single all-lowercase-letters dictionary word (random
// tokens, API keys, and passwords overwhelmingly carry digits, mixed case,
// or non-alphabetic characters; the vendor and PEM detectors already cover
// the vanishingly rare all-lowercase-letters secret shapes on their own
// terms). This is the value-shape signal used, in place of position or word
// count, to tell a real credential apart from a bare-value colon assignment
// whose "value" is actually the tail of an ordinary English sentence -- see
// the call site in tryKeyAt.
func isPlainLowercaseWord(s string, start, end int) bool {
	if start >= end {
		return false
	}
	for i := start; i < end; i++ {
		c := s[i]
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

// looksLikeCredentialValueMinLen is the length, in the absence of a digit or
// mixed case, above which a bare run of letters is treated as credential-
// shaped rather than an ordinary English word. It sits comfortably above
// every ordinary single word this gate must keep rejecting ("pending" (7),
// "Attributes" (10), "production" (10), "confidential" (12)) and comfortably
// below the all-one-case passphrases a password manager actually generates
// ("supersecretvaluehere" (20), "correcthorsebatterystaplewordsonly" (35)),
// so it separates the two without needing a digit from either.
const looksLikeCredentialValueMinLen = 16

// looksLikeCredentialValue reports whether s[start:end] has the shape of a
// genuine credential value rather than an ordinary English word or a
// structural fragment. It must start with a letter or digit (never
// punctuation, which rules out things like "<No", the opening fragment of an
// angle-bracketed placeholder such as "<No Attributes>"), and then satisfy at
// least one of three independent shape signals, each sufficient on its own:
// a digit anywhere in the run; mixed case (both an upper- and a lower-case
// letter somewhere in the run); or a run at least
// looksLikeCredentialValueMinLen bytes long.
//
// The three-way OR replaces an earlier, digit-only version of this gate that
// rejected exactly the strongest passwords a password manager generates: an
// all-letters Diceware-style passphrase ("correcthorsebatterystaplewordsonly")
// or an all-uppercase hex blob with no digit
// ("DEADBEEFCACEFEEDABCDEFABCDEFABCD") has neither a digit to find nor mixed
// case, and used to fail the gate outright, leaking the value in full even
// though a genuine credential-shaped key (ADMIN_PASSWORD, SESSION_SECRET) sat
// directly in front of it. Mixed case ("CorrectHorseBatteryStaple") and
// length (the all-one-case cases above) are the two shape signals that catch
// what a bare digit requirement missed, without reopening the false positive
// the digit requirement was originally added to close: "Key Attributes: <No
// Attributes>" still fails at the c0 check ("<No" starts with punctuation),
// and "pending"/"Attributes"/"production" are all short, single-case, digit-
// free words well under looksLikeCredentialValueMinLen.
//
// This is the value-shape gate for precededBySingleCredentialWord and
// viaWordFallback matches specifically -- a stricter cousin of
// isPlainLowercaseWord, needed because that check alone (reject only
// all-lowercase-letters) does not stop "Key Attributes: <No Attributes>"
// from matching: "<No" is neither all-lowercase nor a real secret, so
// isPlainLowercaseWord alone let it through. This path's only evidence is a
// WEAKER, one-word-removed (or one-word-of-a-compound) adjacency than the
// ordinary credential-word-touches-operator case, so it leans on the value's
// own shape being unambiguous instead.
func looksLikeCredentialValue(s string, start, end int) bool {
	if start >= end {
		return false
	}
	c0 := s[start]
	if !((c0 >= 'a' && c0 <= 'z') || (c0 >= 'A' && c0 <= 'Z') || (c0 >= '0' && c0 <= '9')) {
		return false
	}
	sawDigit := false
	sawUpper := false
	sawLower := false
	for i := start; i < end; i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			sawDigit = true
		case c >= 'A' && c <= 'Z':
			sawUpper = true
		case c >= 'a' && c <= 'z':
			sawLower = true
		}
	}
	return sawDigit || (sawUpper && sawLower) || (end-start) >= looksLikeCredentialValueMinLen
}

// precededBySingleCredentialWord reports whether the word immediately before
// wordStart -- separated from it by exactly one ASCII space and nothing else
// -- collapses to a credential-key identifier. It is the narrow extension
// that lets a two-word label whose credential-bearing word is NOT the one
// directly touching the operator still be recognized: "auth failed:
// DEADBEEF00" has "failed" (not a credential key) immediately before the
// colon, with the actual credential word "auth" one word further back.
// Without this, that shape was invisible to tryKeyAt entirely -- "auth" was
// tried as a key and rejected outright because "failed" (not an operator)
// followed it, and "failed" was tried as a key and rejected outright because
// it is not in credentialKeys -- so the assignment scanner never even
// attempted the operator/value scan, independent of the value's own shape.
//
// Deliberately narrow: exactly one interior space (never a tab, which the
// rest of this file already treats as a column/field separator rather than
// prose spacing), and the preceding run must itself be a whole,
// isIdentByte-delimited identifier (so "x-auth failed:" does not credit
// "auth" out of a larger token). This mirrors isSingleTrailingWord's
// "exactly one word, nothing more" discipline on the opposite side of the
// key.
func precededBySingleCredentialWord(s string, wordStart int) bool {
	if wordStart == 0 || s[wordStart-1] != ' ' {
		return false
	}
	end := wordStart - 1
	start := end
	for start > 0 && isIdentByte(s[start-1]) {
		start--
	}
	if start == end {
		return false
	}
	// The byte before the candidate leading word (if any) must not itself be
	// an identifier byte, or the "word" found above is really the tail of a
	// longer token that merely happens to end where a space was expected.
	if start > 0 && isIdentByte(s[start-1]) {
		return false
	}
	return isCredentialKey(s[start:end])
}

// span is a half-open byte range [Start, End) within the scanned input that
// should be redacted.
type span struct {
	Start, End int
}

// scanCredentialAssignments walks s once, tracking quote/escape state, and
// returns the value-only spans of every recognized credential assignment.
// The key, operator, surrounding structure, and (for quoted values) the
// closing quote are left untouched; only the value span is reported.
//
// Keys are recognized in two shapes: bare identifiers outside any quote
// (env/shell style, `PASSWORD=x`) and quoted identifiers whose closing
// quote is immediately followed by the operator (JSON/YAML style,
// `"password":"x"`). Both funnel through tryKeyAt, which applies the
// chained-label check, the credential-key-set check, and the
// operator/value scan uniformly.
func scanCredentialAssignments(s string) []span {
	var spans []span
	var q quoteState
	escaped := false
	quoteStart := 0 // index just past the currently-open quote character

	i := 0
	for i < len(s) {
		c := s[i]

		if q != quoteNone {
			if escaped {
				escaped = false
				i++
				continue
			}
			if c == '\\' {
				escaped = true
				i++
				continue
			}
			if quoteCharFor(c) == q {
				// Closing quote: if the quoted content is a plain
				// identifier, it may be a JSON/YAML-style key
				// (`"password":"x"`). The operator search resumes just
				// past this closing quote, where the scanner is back at
				// quoteNone, so ':' is allowed here exactly as it is for
				// any other top-level key.
				keyStart, keyEnd := quoteStart, i
				q = quoteNone
				i++
				if isPlainIdent(s, keyStart, keyEnd) {
					if next, ok := tryKeyAt(s, keyStart, keyEnd, i, true, quoteNone, &spans); ok {
						i = next
						continue
					}
				}
				continue
			}
			if isIdentStart(c) {
				// A credential-shaped word inside an already-open quote
				// (e.g. a JSONL transcript's "content" field, a shell
				// heredoc, or ordinary prose fenced in double quotes:
				// `curl -H "X-Api-Key: SECRET" https://x`, `echo "password:
				// SECRET"`). ':' IS trusted as an operator here, exactly as
				// it is at top level: tryKeyAt's own colon trailing-context
				// check (the prose defense that already protects `Error:
				// token: invalid` and `fmt.Errorf("token: %w", err)` at top
				// level -- rejecting the match unless a single trailing word,
				// a structural terminator, or EOF/line-end follows) applies
				// identically here, since it operates on the bytes AFTER the
				// value regardless of quote state. Declining to trust ':'
				// inside a quote at all left "key: value" -- the single most
				// common shape in shell-history and JSONL-transcript free
				// text -- completely invisible whenever it happened to sit
				// inside a BALANCED pair of double quotes. The enclosing
				// quote (q) is passed through so a bare value can never
				// consume the character that closes the surrounding string.
				start := i
				j := i
				for j < len(s) && isIdentByte(s[j]) {
					j++
				}
				// The one colon-keyed shape that IS trusted inside an open
				// quote: an identifier fenced by escaped quotes on both
				// sides, `\"password\":\"x\"`. See
				// escapedQuotedAssignmentSpan.
				if next, ok := escapedQuotedAssignmentSpan(s, start, j, q, &spans); ok {
					i = next
					continue
				}
				if next, ok := tryKeyAt(s, start, j, j, true, q, &spans); ok {
					i = next
					continue
				}
				i = j
				continue
			}
			i++
			continue
		}

		// Outside any quote.
		if w, ok := isLineSep(s, i); ok {
			i += w
			continue
		}
		if qs := topLevelQuoteOpenerFor(c); qs != quoteNone {
			q = qs
			quoteStart = i + 1
			i++
			continue
		}

		if isIdentStart(c) {
			start := i
			j := i
			for j < len(s) && isIdentByte(s[j]) {
				j++
			}
			if next, ok := tryKeyAt(s, start, j, j, true, quoteNone, &spans); ok {
				i = next
				continue
			}
			i = j
			continue
		}

		i++
	}

	return spans
}

// isPlainIdent reports whether s[start:end] is entirely identifier bytes
// (used to test whether quoted content, e.g. "password", is key-shaped).
func isPlainIdent(s string, start, end int) bool {
	if start >= end {
		return false
	}
	for k := start; k < end; k++ {
		if !isIdentByte(s[k]) {
			return false
		}
	}
	return true
}

// maxValueScanBytes is the hard ceiling on how far the scanner will search
// for a value's terminator, independent of how long the eventual redacted
// span turns out to be. It exists purely to bound truly pathological
// input (e.g. an unclosed quote with megabytes of text and no closing
// quote anywhere): a real credential of any realistic length -- including
// a full PEM-sized base64 blob pasted as a bare value -- is found and
// fully redacted long before this is ever reached.
//
// Critically, this bound must NOT cap where the redacted span ends: it
// only bounds where the scanner SEARCHES for a terminator. If a value
// runs all the way to this ceiling without a real terminator being found,
// the redacted span still extends all the way to EOF (see the hitCeiling
// handling in each of the bare/quoted/bracket branches below, and the
// analogous handling in yamlBlockScalarSpan and scanPEMBlocks) -- it
// never stops at the ceiling and leaves the remainder exposed. Two
// distinct, previously-shipped designs got this wrong in two different
// ways, both mutation-confirmed:
//
//  1. Capping the redacted span itself at a short length (the original
//     maxValueSpan=512 design): every byte past position 512 of an
//     unterminated value survived in the clear while hits still reported
//     1 -- "secret cut in half, success reported," the same failure class
//     an earlier regex-based round of this work was rejected for.
//  2. Gating the looksLikeFreshKeyAhead second-credential-detection
//     lookahead by a short length threshold (a later maxValueSpan=512
//     variant, scoped only to that one heuristic): past the threshold,
//     an odd-length backslash run before a second field's opening quote
//     read as one long "escaped" string again, swallowing the second
//     credential whole. Mutation testing proved this precisely: setting
//     that gate's threshold to 1<<30 (never gating) fixed the leak with
//     no regressions, which is exactly why the gate was removed rather
//     than widened -- widening a magic number never actually justifies
//     its own existence, and here it demonstrably didn't have one.
const maxValueScanBytes = 65536

// tryKeyAt attempts to treat s[keyStart:keyEnd] as a credential-key
// identifier immediately followed (across horizontal whitespace only, once
// the search resumes at opSearchStart) by an assignment operator and a
// value. opSearchStart differs from keyEnd when the key was quoted:
// keyEnd marks the identifier's own end (before the closing quote) while
// opSearchStart is the position just past that closing quote.
//
// colonAllowed gates whether ':' is accepted as the operator. It is true
// for keys found at top level (quoteNone), where ':' reliably marks a
// structural key/value pair (JSON, YAML, flow mappings). It is false for
// keys found while already inside an open quote (e.g. a JSONL transcript's
// free-text "content" field): there, ':' is at least as likely to be
// sentence punctuation ("token: %w", "Error: token: invalid") as a real
// assignment, so only '=' -- which is not credible sentence punctuation --
// is trusted in that context.
//
// enclosingQuote is the quote character (if any) the scanner was already
// inside when it found this key -- e.g. quoteDouble for a key found inside
// a JSON string's free-text content. A bare (unquoted) value must never
// consume that enclosing quote's unescaped closing character: doing so
// would swallow the delimiter that terminates the surrounding string,
// corrupting its structure. It is quoteNone for keys found at top level.
//
// On success tryKeyAt appends the value's span to *spans and returns the
// index to resume scanning from. On failure it returns ok=false and the
// caller continues past the key unchanged.
func tryKeyAt(s string, keyStart, keyEnd, opSearchStart int, colonAllowed bool, enclosingQuote quoteState, spans *[]span) (next int, ok bool) {
	ident := s[keyStart:keyEnd]

	// Chained-label preceding byte, captured before the credential-key and
	// operator checks below so it can be combined with the CURRENT key's own
	// operator once that is known (see the chained-label check further
	// down): p-1 is the last non-space byte before the key, or keyStart
	// itself (never an operator) when nothing precedes it.
	precedingByte := byte(0)
	if keyStart > 0 {
		p := keyStart
		for p > 0 && isHSpace(s[p-1]) {
			p--
		}
		if p > 0 {
			precedingByte = s[p-1]
		}
	}

	// isKeyMatch is true whenever ident ITSELF is recognized as a credential
	// key -- whether by exact whole-identifier membership (isCredentialKey),
	// by containing a credential word as one of its own separator/camelCase
	// words (isCredentialKeyByWord), or by containing one fused anywhere
	// inside a single-word compound with no separator at all
	// (isCredentialKeyByFusedSubstring). All three are the SAME strength of
	// evidence: the key itself names a credential, full stop. There is no
	// value shape that makes ADMIN_PASSWORD=, RAILS_MASTER_KEY=,
	// ADMINPASSWORD=, PASSWORDADMIN=, or KEYSTORE= safe to ship to a
	// third-party API, so none of these three mechanisms is gated by
	// looksLikeCredentialValue below -- unlike precededBySingleCredentialWord
	// (viaLeadingCredentialWord), where the credential word never touches the
	// operator at all and the evidence really is weaker. Gating only the
	// fallback mechanisms while leaving isCredentialKey's exact-match path
	// ungated was the design defect this round of work replaces: it applied
	// the STRICTER test to the keys the scrubber was LESS certain about,
	// which is backwards, and is why every new key-matching mechanism added
	// under that design shipped with a fresh leak attached (DB_PASSWORD
	// redacted unconditionally while the byte-identical value under
	// ADMIN_PASSWORD did not).
	//
	// Accepted cost: KEYSTORE, BYPASS, XPASS, COMPASS and similar keys whose
	// entire content happens to be (or contain) an ordinary English word that
	// also fuses a credential word now redact unconditionally too. That is
	// the sanctioned direction -- a fixed, finite over-redaction cost -- not
	// an open leak surface.
	viaLeadingCredentialWord := false
	isKeyMatch := isCredentialKey(ident) || isCredentialKeyByWord(ident) || isCredentialKeyByFusedSubstring(ident)
	if !isKeyMatch {
		if precededBySingleCredentialWord(s, keyStart) {
			viaLeadingCredentialWord = true
		} else {
			return 0, false
		}
	}

	// Look for the operator, honoring horizontal whitespace only.
	k := opSearchStart
	for k < len(s) && isHSpace(s[k]) {
		k++
	}
	if k >= len(s) {
		return 0, false
	}
	op := s[k]
	if op != '=' && op != ':' {
		return 0, false
	}
	if op == ':' && !colonAllowed {
		return 0, false
	}
	opEnd := k + 1
	if op == '=' && opEnd < len(s) && s[opEnd] == '=' {
		// "==" is a comparison, not an assignment.
		return 0, false
	}

	// Chained-label check: reject a key whose immediately preceding
	// non-space byte is itself an assignment operator AND whose own
	// operator (op, just determined above) is ':'. That combination is the
	// actual prose shape this check exists to reject ("Error: token:
	// invalid", "word: word: ..."): a chain of ':'-joined labels ending in
	// an ordinary trailing word, never a real credential.
	//
	// The combination is checked precisely because ONLY it is prose. A key
	// preceded by an operator but itself using '=' is a nested assignment
	// after an outer one -- a routine shape in shell/CLI flags
	// ("--env=PGPASSWORD=x", key preceded by '=', own op '=') and
	// colon-prefixed config/log lines ("config: password=x", key preceded
	// by ':', own op '='). Rejecting those outright (the previous behavior,
	// which looked only at the preceding byte) dropped the entire
	// assignment and shipped the value in the clear -- the dangerous
	// direction; over-redacting a rare "word=word=word" case is not.
	if isChainedLabelByte(precedingByte) && op == ':' {
		return 0, false
	}

	// Skip horizontal whitespace after the operator.
	v := opEnd
	for v < len(s) && isHSpace(s[v]) {
		v++
	}
	if v >= len(s) {
		return 0, false
	}

	if op == ':' {
		if blockEnd, ok := yamlBlockScalarSpan(s, keyStart, v, spans); ok {
			return blockEnd, true
		}
	}

	if w, lineOK := isLineSep(s, v); lineOK {
		// A value starting on the next line is only accepted when it's
		// quoted: {"password":\n"x"} is common pretty-printed JSON, a
		// purely structural shape with no ambiguity. A *bare* value
		// starting on the next line is left rejected, same as before,
		// since that shape is indistinguishable from a label followed by
		// unrelated prose on a new line.
		afterLine := v + w
		for afterLine < len(s) && isHSpace(s[afterLine]) {
			afterLine++
		}
		if afterLine >= len(s) || quoteCharFor(s[afterLine]) == quoteNone {
			return 0, false
		}
		v = afterLine
	}

	if qs := quoteCharFor(s[v]); qs != quoteNone {
		// Quoted value: scan to the matching close, honoring escapes. An
		// escaped quote does not end the string -- except when what
		// follows it looks like a fresh, unrelated "key=" or "key:"
		// assignment (looksLikeFreshKeyAhead): a genuine credential value
		// is never itself followed by another assignment immediately
		// after an "escaped" quote, so treating that quote as the real
		// terminator instead is the safe, bounded direction. Without this,
		// an odd-length run of backslashes before a second field's opening
		// quote (e.g. `password="a\" token="X"`) reads as one long
		// "escaped" string that swallows the second field whole.
		vStart := v + 1
		vi := vStart
		vEscaped := false
		closed := false
		hitCeiling := true
		for vi < len(s) && vi-vStart < maxValueScanBytes {
			vc := s[vi]
			if vEscaped {
				vEscaped = false
				vi++
				continue
			}
			if vc == '\\' {
				// The looksLikeFreshKeyAhead lookahead runs regardless of
				// how far into the value it is (no maxValueSpan gate): a
				// second, unrelated credential bleeding into this one via
				// an odd-length backslash run is exactly as real a risk at
				// byte 10,000 as at byte 10 -- gating it by length only
				// let the leak resurface past the gate's own threshold
				// (mutation-confirmed: an earlier version of this check
				// required vi-vStart < maxValueSpan, and every pad value
				// at or past 512 leaked the second credential in the
				// clear while hits still reported 1).
				if vi+1 < len(s) && quoteCharFor(s[vi+1]) == qs && looksLikeFreshKeyAhead(s, vi+2) {
					// Stop the value right before this backslash, leaving
					// both it and the quote it introduces untouched for
					// the outer scanner: closed stays false, so the
					// caller resumes scanning at vi (not vi+1) in its
					// normal, non-quoted state, and will open a fresh
					// quote context at the quote right after this
					// backslash, exactly as if this value had ended here.
					hitCeiling = false
					break
				}
				vEscaped = true
				vi++
				continue
			}
			if quoteCharFor(vc) == qs {
				closed = true
				hitCeiling = false
				break
			}
			if _, lineOK := isLineSep(s, vi); lineOK {
				hitCeiling = false
				break
			}
			vi++
		}
		if hitCeiling && enclosingQuote != quoteNone && qs == enclosingQuote {
			// The byte at v -- treated above as OPENING a nested quoted
			// value -- is the same character as the ENCLOSING string's own
			// quote, and no second occurrence of it was found anywhere
			// before EOF (hitCeiling). That combination means v was never a
			// value opener at all: it was the enclosing string's own
			// closing delimiter, and the assignment has no value in
			// between (e.g. `{"note":"see api_key="}`, where the operator's
			// very next byte is the JSON string's own terminating '"').
			// Genuine nested values never use the enclosing quote's raw,
			// unescaped character -- JSON requires \" there, handled
			// separately by escapedQuotedAssignmentSpan -- so failing to
			// find a second occurrence is proof there was no value, not
			// proof of an unusually long one. Without this, the fallback
			// below extended the span all the way to EOF, consuming the
			// enclosing string's real closing quote and whatever structure
			// followed it (e.g. the object's closing brace) even though
			// there was no secret anywhere in the input.
			//
			// This is deliberately narrower than rejecting on
			// s[v]==enclosingQuote alone: when a second, genuine occurrence
			// of that same character IS found before EOF (closed==true,
			// hitCeiling==false), the value is a well-formed nested
			// assignment that merely happens to reuse the enclosing quote's
			// character as its own delimiter (e.g. a top-level
			// `password="a\" token="X"` resuming as a fresh top-level quote
			// per the backslash-run handling above), and must still be
			// redacted normally -- that path is untouched here.
			return 0, false
		}
		if hitCeiling {
			// No real terminator was found within maxValueScanBytes: this
			// is unterminated/pathological input (an unclosed quote with
			// no closing character anywhere nearby), not a legitimate
			// long-but-bounded secret. Extend the redacted span to EOF
			// rather than truncating it at the ceiling -- see the
			// matching comment on the bare-value branch below for the
			// full rationale (the same "secret cut in half, success
			// reported" failure class, now fixed for the quoted-value
			// form too).
			vi = len(s)
		}
		if s[vStart:vi] == redactedToken {
			// Already redacted: re-scrubbing must be a no-op, not a fresh
			// hit against the placeholder text itself.
			if closed {
				return vi + 1, false
			}
			return vi, false
		}
		*spans = append(*spans, span{Start: vStart, End: vi})
		if closed {
			return vi + 1, true
		}
		return vi, true
	}

	if enclosingQuote != quoteNone && s[v] == '\\' && v+1 < len(s) && quoteCharFor(s[v+1]) == enclosingQuote {
		// Escape-quoted value nested inside an already-open string:
		// `\"password\":\"SECRET\"` found via a BARE key (unlike
		// escapedQuotedAssignmentSpan, which requires the key itself to be
		// escape-fenced on both sides), e.g. a JSONL transcript's free-text
		// "content" field containing a shell command with a double-quoted
		// secret: `run: export PASSWORD=\"SECRET\"`. s[v] is a backslash and
		// s[v+1] is the SAME quote character enclosing the surrounding
		// string, so `\"` here is the value's own opening delimiter, not the
		// enclosing string's escaped-but-non-terminating quote: from the
		// scanner's position at v, nothing has consumed this pair yet, and
		// treating it as a value-opening quote (scanning for its matching
		// `\"` close, mirroring escapedQuotedAssignmentSpan's value scan) is
		// what the bare-value branch below cannot do -- its own enclosing-
		// quote guard stops dead at the very first byte of the value
		// (v+1, the escaped quote's closing partner as far as that branch's
		// escape handling can tell) and rejects the whole assignment.
		vStart := v + 2
		vi := vStart
		closed := false
		hitCeiling := true
		for vi < len(s) && vi-vStart < maxValueScanBytes {
			if s[vi] == '\\' && vi+1 < len(s) && quoteCharFor(s[vi+1]) == enclosingQuote {
				closed = true
				hitCeiling = false
				break
			}
			if _, lineOK := isLineSep(s, vi); lineOK {
				hitCeiling = false
				break
			}
			// An unescaped occurrence of the enclosing quote closes the
			// surrounding string outright: the value cannot run past it.
			if quoteCharFor(s[vi]) == enclosingQuote {
				hitCeiling = false
				break
			}
			vi++
		}
		if hitCeiling {
			vi = len(s)
		}
		if s[vStart:vi] == redactedToken {
			// Already redacted: re-scrubbing must be a no-op, not a fresh
			// hit against the placeholder text itself.
			if closed {
				return vi + 2, false
			}
			return vi, false
		}
		*spans = append(*spans, span{Start: vStart, End: vi})
		if closed {
			return vi + 2, true
		}
		return vi, true
	}

	if bc, ok := bracketCloseFor(s[v]); ok {
		// Bracket-delimited value: password={SECRET}, password=[SECRET],
		// password=(SECRET). Structurally like a quoted value -- redact
		// only the interior, preserve the delimiters -- but matched
		// against the first occurrence of the specific closing character,
		// with no nesting or escape handling: credential values in this
		// shape are not themselves nested structures.
		//
		// Already-redacted check comes first and handles two distinct
		// shapes, both compared as literal prefixes rather than via the
		// nesting-unaware bracket scan below (which would misparse both):
		//
		//  1. The value IS the bare placeholder itself, unwrapped
		//     (password=[REDACTED], the normal output of a previous bare
		//     '[' /(none)-terminated redaction pass -- redactedToken's own
		//     leading '[' looks exactly like an opening bracket). Detected
		//     by s[v:] starting with redactedToken directly, i.e. before
		//     even consuming s[v] as "our" opening bracket.
		//  2. The value was wrapped in brackets we added ourselves last
		//     pass (password=[[REDACTED]], from redacting password=[SECRET]).
		//     Detected by s[v+1:] starting with redactedToken followed by
		//     the matching close.
		//
		// Without case 1, redactedToken's own ']' would be mistaken for
		// the close of a *fresh* bracket value starting at its '[',
		// extracting "REDACTED" and redacting it again into
		// "[[REDACTED]]". Without case 2, the same double-wrap can recur
		// one layer further out.
		if strings.HasPrefix(s[v:], redactedToken) {
			return v + len(redactedToken), false
		}
		vStart := v + 1
		if strings.HasPrefix(s[vStart:], redactedToken) {
			afterToken := vStart + len(redactedToken)
			if afterToken < len(s) && s[afterToken] == bc {
				return afterToken + 1, false
			}
			return afterToken, false
		}

		// Escape handling here exists solely to protect the *enclosing*
		// quote (e.g. this bracket value sitting inside a JSON string's
		// free-text content), mirroring the bare-value branch above: an
		// occurrence of the enclosing quote character always terminates
		// this value, even when backslash-escaped, and a backslash whose
		// escaped character is that enclosing quote must not itself be
		// consumed into the span (the outer scanner needs both bytes
		// intact to keep treating it as a non-terminating escape for the
		// *enclosing* string). This is deliberately separate from bc
		// (this bracket's own closing character), which is still matched
		// with no escape/nesting awareness, per the design above.
		vi := vStart
		vEscapedOuter := false
		closed := false
		hitCeiling := true
		for vi < len(s) && vi-vStart < maxValueScanBytes {
			vc := s[vi]
			if enclosingQuote != quoteNone && quoteCharFor(vc) == enclosingQuote {
				hitCeiling = false
				break
			}
			if vEscapedOuter {
				vEscapedOuter = false
				vi++
				continue
			}
			if vc == '\\' {
				if enclosingQuote != quoteNone && vi+1 < len(s) && quoteCharFor(s[vi+1]) == enclosingQuote {
					hitCeiling = false
					break
				}
				vEscapedOuter = true
				vi++
				continue
			}
			if vc == bc {
				closed = true
				hitCeiling = false
				break
			}
			if _, lineOK := isLineSep(s, vi); lineOK {
				hitCeiling = false
				break
			}
			vi++
		}
		if vi == vStart {
			return 0, false
		}
		if hitCeiling {
			// No real terminator (this bracket's own closing character,
			// or the enclosing quote) was found within maxValueScanBytes:
			// unterminated/pathological input, not a legitimate long
			// secret. Extend the span to EOF rather than truncating it at
			// the ceiling -- see the matching comment on the bare-value
			// branch below for the full rationale.
			vi = len(s)
		}
		*spans = append(*spans, span{Start: vStart, End: vi})
		if closed {
			return vi + 1, true
		}
		return vi, true
	}

	// Bare value: read the token, then validate its context. If this key
	// was found nested inside an already-open quote, ANY occurrence of
	// that quote's character terminates the value -- escaped or not. It
	// is the delimiter that will eventually close the surrounding string
	// (e.g. the nested, backslash-escaped quote in a JSON string like
	// "he said \"my password=X\" loudly"); from the value's own
	// perspective it is a literal quote character sitting in the decoded
	// text, and letting the value swallow it past that point corrupts the
	// surrounding structure. This check therefore comes before the escape
	// branch, not after: escape state governs whether a quote ends the
	// *enclosing* string (the outer scanner's concern), not whether one
	// terminates *this* value.
	// The search runs to maxValueScanBytes: every check in this loop
	// (enclosing-quote termination, escaped-backslash handling,
	// isBareValueByte) is a length-independent structural rule, applied
	// uniformly regardless of how long the value has already run. The
	// value is redacted in full regardless of how long it turns out to
	// be -- see hitCeiling below.
	vStart := v
	vi := v
	vEscapedOuter := false
	hitCeiling := true
	// schemeSpaceAt is the offset of the one interior space that
	// bareValueSchemeSpan has already confirmed belongs to a recognized
	// "Scheme <credential>" value (authorization=Basic <base64>), computed
	// once against vStart rather than re-checked every loop iteration: the
	// scheme word can only ever start at the value's own beginning.
	schemeSpaceAt, hasSchemeSpace := bareValueSchemeSpan(s, vStart)
	for vi < len(s) && vi-vStart < maxValueScanBytes {
		vc := s[vi]
		// Enclosing-quote check comes first, deliberately ignoring
		// vEscapedOuter: a backslash-escaped occurrence of the enclosing
		// quote character is still that literal character in the decoded
		// text and must still end the value.
		if enclosingQuote != quoteNone && quoteCharFor(vc) == enclosingQuote {
			hitCeiling = false
			break
		}
		if vEscapedOuter {
			vEscapedOuter = false
			vi++
			continue
		}
		if vc == '\\' {
			// A backslash whose escaped character is the enclosing
			// quote must not itself be swallowed into the value: stop
			// right before it so that character (and the backslash
			// introducing it) are left for the outer scanner, which
			// still correctly treats it as a non-terminating escape
			// for the *enclosing* string.
			if enclosingQuote != quoteNone && vi+1 < len(s) && quoteCharFor(s[vi+1]) == enclosingQuote {
				hitCeiling = false
				break
			}
			// A backslash immediately followed by 'n' or 'r' is the
			// JSON/textual escape sequence for a line separator (LF, CR) --
			// the two-byte SPELLING of a newline within a string whose own
			// literal bytes have not been unescaped, exactly as this scanner
			// never unescapes the strings it walks. It must terminate the
			// value the same way an actual literal '\n'/'\r' byte already
			// does (see isLineSep and isBareValueByte's own exclusion of
			// them), not be swallowed as an opaque escaped pair the way
			// "\"" or "\\" legitimately are. Without this,
			// `password: SECRET\n\`\`\`` (a value followed by an escaped
			// newline and a closing markdown code fence, inside a JSON
			// string) had its span run straight through both: the escaped
			// newline and the fence were consumed into the redaction, so
			// the fence's closing backticks -- structure the surrounding
			// document still needs -- were deleted along with the secret.
			if vi+1 < len(s) && (s[vi+1] == 'n' || s[vi+1] == 'r') {
				hitCeiling = false
				break
			}
			vEscapedOuter = true
			vi++
			continue
		}
		if hasSchemeSpace && vi == schemeSpaceAt {
			// The one space that separates a recognized auth scheme word
			// from its credential (see bareValueSchemeSpan): part of this
			// value, not a terminator, so the scan continues right through
			// it instead of stopping here.
			vi++
			continue
		}
		if !isBareValueByte(vc) {
			hitCeiling = false
			break
		}
		vi++
	}
	if vi == vStart {
		return 0, false
	}
	if hitCeiling {
		// No real terminator (enclosing quote, non-bare-value byte) was
		// found within maxValueScanBytes: this is unterminated/pathological
		// input, not a legitimate long-but-bounded secret. Extend the
		// redacted span all the way to EOF rather than truncating it at
		// the ceiling -- truncating here left the entire remainder of an
		// unterminated value in the clear while still reporting hits=1,
		// the same "secret cut in half, success reported" failure class
		// fixed once already for maxValueSpan. The ceiling still bounds
		// how far the scanner SEARCHES for a genuine terminator; it must
		// not silently become where the value's exposure ends.
		vi = len(s)
	}
	// A value immediately followed by '(' is a function call (e.g.
	// "getToken()"), not a literal secret.
	if vi < len(s) && s[vi] == '(' {
		return 0, false
	}
	// Note: no "already redacted" literal-equality guard is needed here.
	// redactedToken ("[REDACTED]") always starts with '[', and any value
	// starting with '[' is intercepted by the bracket-value branch above
	// before control ever reaches this bare-value code -- so a bare value
	// can never equal redactedToken in the first place. Re-scrubbing a
	// previously bare-redacted value (password=hunter2 ->
	// password=[REDACTED]) is idempotent via that bracket branch's own
	// "unwrapped placeholder" check, not via anything here.
	// Trailing context (':' only): skip horizontal whitespace, then require
	// end-of-input, a line separator, a structural delimiter, a comment
	// introducer, or exactly one trailing word with nothing (other than
	// EOF/line-end) after it. If two OR MORE ordinary words follow across a
	// space instead, this is prose ("Secret: this document is confidential")
	// and must not fire -- the multi-word shape is the real signal, and that
	// is what this check exists to reject. '=' is exempted entirely: it is
	// not credible sentence punctuation the way ':' is, so a shell/env value
	// followed by more free text (e.g. embedded in transcript prose, "my
	// password=hunter2secret please store it") still counts.
	//
	// The accepted set deliberately covers more than closers. Restricting
	// it to EOF, a line separator, and & | ; , } ) ] leaked in the clear,
	// hits=0, on three ordinary config shapes that are not prose at all:
	//
	//	password: SECRETVAL123 # prod      trailing comment (.env, YAML, TOML, INI, shell)
	//	api_key: SECRETVAL123 (prod)       parenthetical annotation
	//	password: SECRETVAL123<TAB>note    tab-separated column
	//
	// Dropping the whole assignment on any of those is the dangerous
	// direction: the value is a real credential and it shipped verbatim.
	// So openers ({ [ () join their matching closers, '#' is accepted as a
	// comment introducer, and a tab counts as a separator in its own right.
	// None of the three is how an English sentence continues after a word,
	// which is why the prose defense survives intact: "Secret: this
	// document is confidential" reaches 'd' of "document" across a space
	// and still rejects (two further words, "document" and "is", follow).
	//
	// The single-trailing-word exception exists because dropping the WHOLE
	// assignment when exactly one word follows is itself an under-redaction
	// bug, distinct from (and in addition to) the prose defense: e.g.
	// "token: abc123xyz sk-aaaa...aaaa" (a genuine second credential
	// appended after the first, a shape a value-adjacency fuzzer produces
	// routinely) rejected the WHOLE match on the first Scrub pass because
	// "sk-aaaa...aaaa" is an ordinary trailing word by this check's own
	// rule, leaving "abc123xyz" -- the first credential -- unredacted even
	// though a completely independent vendor pattern still caught the
	// second one on the same pass. Only on a SECOND pass, once the second
	// credential was already replaced with the placeholder (whose leading
	// '[' this check already accepts as a structural opener), did the first
	// credential finally redact -- convergence at pass 2, not pass 1, on a
	// security boundary. A real sentence never ends after exactly one more
	// word with nothing else following (not even a period, which itself
	// would be a second "word" here since it does not match any terminator
	// byte below), so "one trailing word, then EOF/line-end" is safe to
	// accept without reopening the multi-word prose false positive.
	if op == ':' {
		t := vi
		sawTab := false
		for t < len(s) && isHSpace(s[t]) {
			if s[t] == '\t' {
				// A tab is a column separator, not the spacing between
				// words of a sentence; whatever follows it is a separate
				// field, not a continuation of prose.
				sawTab = true
			}
			t++
		}
		if t < len(s) && !sawTab {
			if _, lineOK := isLineSep(s, t); !lineOK {
				switch {
				case s[t] == '&', s[t] == '|', s[t] == ';', s[t] == ',', s[t] == '}', s[t] == ')', s[t] == ']':
					// acceptable terminator: a structural closer
				case s[t] == '{', s[t] == '[', s[t] == '(', s[t] == '#':
					// acceptable terminator: a structural opener, or a
					// comment introducer
				case enclosingQuote != quoteNone && quoteCharFor(s[t]) == enclosingQuote:
					// The byte immediately after the value (no intervening
					// word) is the ENCLOSING string's own closing delimiter
					// -- e.g. the value ends right at the `"` of
					// `"X-Api-Key: SECRET" https://x`. That single character
					// is not itself a word of prose, so it must not be
					// counted as the "one trailing word" isSingleTrailingWord
					// looks for below: doing so read the bare quote character
					// as a fake first word and then rejected the match the
					// instant a SECOND, genuine word followed it outside the
					// container (here, "https"), even though nothing about
					// that outer word says anything about the credential's
					// own value. The check instead resumes one byte further,
					// past the delimiter, and re-applies the identical
					// single-trailing-word rule from there -- so a real
					// sentence continuing after the container ("token: %w",
					// followed by more of an actual English sentence) is
					// still correctly rejected, exactly as it would be were
					// the whole thing unquoted.
					afterDelim := t + 1
					for afterDelim < len(s) && isHSpace(s[afterDelim]) {
						afterDelim++
					}
					if afterDelim < len(s) {
						if _, lineOK := isLineSep(s, afterDelim); !lineOK && !isSingleTrailingWord(s, afterDelim) {
							return 0, false
						}
					}
					if isPlainLowercaseWord(s, vStart, vi) {
						// The value itself is an ordinary lowercase dictionary
						// word ("expired", "unknown", "invalid", "missing"),
						// not a real credential. This shape is reached only
						// when nothing but the container's own closing
						// delimiter (and, per the check just above, at most one
						// further word or nothing) follows the value -- e.g.
						// {"error":"invalid token: expired"}, where "expired"
						// is the value and `"}` ends the document. There is no
						// second word left to apply the ordinary multi-word
						// prose defense to, so the value's own shape is the
						// only remaining signal: a genuine secret is
						// essentially never a single all-lowercase-letters
						// word (random tokens, API keys and passwords
						// overwhelmingly carry digits, mixed case, or
						// punctuation). Without this, "invalid token: expired"
						// sitting inside a JSON string with nothing after the
						// closing brace redacted "expired" as though it were a
						// credential, hits=1.
						return 0, false
					}
				default:
					if !isSingleTrailingWord(s, t) {
						return 0, false
					}
				}
			}
		}
	}
	if viaLeadingCredentialWord && !looksLikeCredentialValue(s, vStart, vi) {
		// The key evidence for this match is weaker than every other case
		// reaching this point: the credential word ("auth" in "auth failed:
		// X") never actually touches the operator at all -- a word that is
		// NOT itself credential-shaped ("failed") does. isKeyMatch's three
		// mechanisms (exact, by-word, fused-substring) all recognize the
		// operator-adjacent identifier ITSELF as a credential key and are
		// deliberately left ungated above (see isKeyMatch's doc comment): a
		// key match is unconditional. Only this leading-word path, where the
		// credential word is one word further back and something else sits
		// directly against the operator, has no direct key/operator adjacency
		// left to lean on, so it leans on the value's own shape being
		// unambiguous instead (see looksLikeCredentialValue's doc comment for
		// why isPlainLowercaseWord alone is not strict enough here -- it let
		// "Key Attributes: <No Attributes>" over-redact "<No").
		return 0, false
	}
	*spans = append(*spans, span{Start: vStart, End: vi})
	return vi, true
}

// escapedQuotedAssignmentSpan recognizes a credential assignment written
// with backslash-escaped quotes inside an already-open string:
// `\"password\":\"SECRETVAL\"`. It is the standard shape whenever a JSON
// config blob is quoted inside another JSON string -- e.g. a Claude Code
// JSONL transcript embedding a config file, one of the four sources this
// harvester reads -- and it was completely invisible before: inside an open
// quote only '=' is trusted as an operator (a bare ':' there is at least as
// likely to be sentence punctuation), and the key's own operator is
// separated from it by the escaped closing quote, so tryKeyAt rejected it
// twice over.
//
// The general ':'-inside-quotes restriction is deliberately NOT loosened
// here, because this shape does not need it: the identifier must be fenced
// by an escaped quote on BOTH sides, matching the enclosing string's own
// quote character. That is structurally unambiguous -- prose does not fence
// a word in escaped quotes and then follow it with a colon -- so "Secret:
// this document is confidential" and friends still reject, as they must.
//
// s[keyStart:keyEnd] is the identifier run the caller already scanned;
// enclosingQuote is the quote character of the string the scanner is inside.
// On a match the value's interior span (the escaped quotes themselves
// preserved, so the surrounding JSON stays structurally valid) is appended
// to *spans and the index to resume scanning from is returned.
func escapedQuotedAssignmentSpan(s string, keyStart, keyEnd int, enclosingQuote quoteState, spans *[]span) (next int, ok bool) {
	if enclosingQuote == quoteNone {
		return 0, false
	}
	// Opening fence: the two bytes before the identifier must be a backslash
	// followed by the enclosing quote character.
	if keyStart < 2 || s[keyStart-2] != '\\' || quoteCharFor(s[keyStart-1]) != enclosingQuote {
		return 0, false
	}
	ident := s[keyStart:keyEnd]
	if !isCredentialKey(ident) && !isCredentialKeyByWord(ident) && !isCredentialKeyByFusedSubstring(ident) {
		return 0, false
	}
	// Closing fence, then the operator.
	k := keyEnd
	if k+1 >= len(s) || s[k] != '\\' || quoteCharFor(s[k+1]) != enclosingQuote {
		return 0, false
	}
	k += 2
	for k < len(s) && isHSpace(s[k]) {
		k++
	}
	if k >= len(s) || s[k] != ':' {
		return 0, false
	}
	k++
	for k < len(s) && isHSpace(s[k]) {
		k++
	}
	// The value must be escape-quoted too. A bare value here is left to the
	// existing paths rather than handled speculatively: only the fully
	// fenced shape is unambiguous enough to justify trusting ':' inside a
	// quote.
	if k+1 >= len(s) || s[k] != '\\' || quoteCharFor(s[k+1]) != enclosingQuote {
		return 0, false
	}
	vStart := k + 2
	// Scan to the closing escaped quote. The search is bounded by
	// maxValueScanBytes exactly as every other value form is; if no
	// terminator is found within it the span extends to EOF rather than
	// truncating and leaving the tail of the value in the clear.
	vi := vStart
	closed := false
	hitCeiling := true
	for vi < len(s) && vi-vStart < maxValueScanBytes {
		if s[vi] == '\\' && vi+1 < len(s) && quoteCharFor(s[vi+1]) == enclosingQuote {
			closed = true
			hitCeiling = false
			break
		}
		if _, lineOK := isLineSep(s, vi); lineOK {
			hitCeiling = false
			break
		}
		// An unescaped occurrence of the enclosing quote closes the
		// surrounding string: the value cannot run past it.
		if quoteCharFor(s[vi]) == enclosingQuote {
			hitCeiling = false
			break
		}
		vi++
	}
	if hitCeiling {
		vi = len(s)
	}
	if s[vStart:vi] == redactedToken {
		// Already redacted: re-scrubbing is a no-op, not a fresh hit against
		// the placeholder text.
		if closed {
			return vi + 2, false
		}
		return vi, false
	}
	if vi == vStart {
		return 0, false
	}
	// No value-shape gate here: ident is a credential key by one of the three
	// isKeyMatch mechanisms (see tryKeyAt's doc comment for why none of them
	// is gated), and the fencing itself (escaped quotes on both sides,
	// matching the enclosing quote, followed by ':') is already structurally
	// unambiguous -- prose does not fence a word in escaped quotes and follow
	// it with a colon.
	*spans = append(*spans, span{Start: vStart, End: vi})
	if closed {
		return vi + 2, true
	}
	return vi, true
}

// yamlLineIndent returns the number of leading space/tab bytes on the
// physical line containing position i.
func yamlLineIndent(s string, i int) int {
	start := i
	for start > 0 {
		if _, ok := isLineSep(s, start-1); ok {
			break
		}
		start--
	}
	n := 0
	for start+n < len(s) && isHSpace(s[start+n]) {
		n++
	}
	return n
}

// yamlBlockScalarSpan recognizes a YAML block scalar value -- "key: |" or
// "key: >" (a literal or folded block), with an optional chomping
// indicator ('-' or '+') and/or explicit indentation indicator (a single
// digit), in either order -- immediately at s[v], with nothing but
// horizontal whitespace or a comment before the end of that line. On a
// match it returns the byte offset just past the entire block (the marker
// line's own newline plus every subsequent line that is blank or indented
// more than the key itself, per the standard YAML block-scalar rule) and
// appends the block body's span to *spans, redacting the body while
// preserving the "key: |" header. keyStart locates the key's own line so
// its indentation can be used as the "more indented than this" baseline.
//
// Without this, a block scalar value is invisible to the rest of
// tryKeyAt: the marker sits at end-of-line (rejected by the ordinary
// "value must not start with a line separator" check) and, if that were
// relaxed generically, a bare per-line scan would treat only the marker
// itself as the value and leave the entire indented body -- the actual
// secret -- untouched while still reporting a hit, which is worse than no
// coverage at all because it signals false confidence.
func yamlBlockScalarSpan(s string, keyStart, v int, spans *[]span) (next int, ok bool) {
	if s[v] != '|' && s[v] != '>' {
		return 0, false
	}
	i := v + 1
	sawIndicator := true
	for sawIndicator && i < len(s) {
		switch {
		case s[i] == '-' || s[i] == '+':
			i++
		case s[i] >= '1' && s[i] <= '9':
			i++
		default:
			sawIndicator = false
		}
	}
	// Nothing but horizontal whitespace (and an optional '#' comment to
	// end of line) may follow on the marker's own line.
	t := i
	for t < len(s) && isHSpace(s[t]) {
		t++
	}
	if t < len(s) && s[t] == '#' {
		for t < len(s) {
			if _, lineOK := isLineSep(s, t); lineOK {
				break
			}
			t++
		}
	}
	w, lineOK := isLineSep(s, t)
	if !lineOK && t != len(s) {
		// Trailing content on the marker line that isn't whitespace or a
		// comment: not a well-formed block-scalar header.
		return 0, false
	}

	keyIndent := yamlLineIndent(s, keyStart)
	bodyStart := t + w
	bodyEnd := bodyStart
	pos := bodyStart
	// The whole block body is the value here (there is no separate
	// terminator byte to search for the way a quote or bracket has one:
	// the block's own end is "indentation drops back to keyIndent" or
	// EOF), so the entire body must always be redacted -- but the search
	// for where that end is still needs a hard ceiling, the same as every
	// other value form, or one pathologically long single line runs
	// unbounded. The cap is checked on every iteration of BOTH the outer
	// (line-by-line) loop and the inner (within-a-line) loop below, not
	// just between lines: checking only between lines lets one very long
	// line blow through the cap entirely, since the inner loop would
	// already have consumed it in full before the outer loop's check ever
	// runs again.
	// bodyIndent is the indentation level the block body must meet, set by
	// the first non-blank body line and then held fixed for every
	// subsequent line. Ordinarily that first line must be indented more
	// than the key itself (indent > keyIndent), matching standard YAML
	// block-scalar auto-detection. But when the key sits at column 0
	// (keyIndent == 0, a top-level document key), YAML still permits an
	// unindented body -- there is no shallower level left to distinguish
	// it from -- so the first line's indent (0) is accepted as-is rather
	// than requiring indent > 0, which no column-0 body could ever
	// satisfy. Without this, "password: |\nTOPLEVELSECRET\n" was
	// completely invisible to detection: keyIndent=0 and indent=0 fails
	// indent>keyIndent immediately, so the block was never recognized and
	// the secret shipped in full, unredacted.
	bodyIndent := -1
	hitOuterCeiling := true
	for pos < len(s) && pos-bodyStart < maxValueScanBytes {
		lineStart := pos
		lw, lok := isLineSep(s, lineStart)
		if lok {
			// Blank line: part of the block regardless of indentation.
			pos = lineStart + lw
			bodyEnd = pos
			continue
		}
		indent := 0
		for lineStart+indent < len(s) && isHSpace(s[lineStart+indent]) {
			indent++
		}
		if bodyIndent < 0 {
			if indent <= keyIndent && keyIndent > 0 {
				hitOuterCeiling = false
				break
			}
			// keyIndent == 0 is the one case where the branch just above
			// can never fire (indent <= 0 && keyIndent > 0 is never true),
			// so a column-0 key's body auto-detects bodyIndent from
			// whatever indent this very first line happens to have -- and
			// when that line IS the redactedToken placeholder itself
			// (re-scrubbing a previous pass's output, where the marker
			// carries none of the original body's own indentation), that
			// indent is 0 with nothing shallower for a genuinely fresh
			// top-level key to ever register as "less indented than the
			// body." Every following line at indent 0 -- including an
			// unrelated key like "region: eu-west-1" -- would otherwise
			// satisfy this auto-detected bodyIndent==0 forever, so the loop
			// could never find the real end of the (already-redacted, now
			// empty) block and silently kept consuming subsequent
			// top-level content as if it were still body. Recognizing the
			// placeholder explicitly, before it ever gets to set
			// bodyIndent, stops the scan right here instead: the same
			// "no-op on already-redacted input" contract every other value
			// form (quoted, bracketed) already honors via its own
			// redactedToken comparison, just reached one line earlier
			// because indentation alone cannot gate a column-0 body.
			if lineStart+len(redactedToken) <= len(s) && s[lineStart:lineStart+len(redactedToken)] == redactedToken {
				afterToken := lineStart + len(redactedToken)
				if _, tokLineOK := isLineSep(s, afterToken); tokLineOK || afterToken == len(s) {
					hitOuterCeiling = false
					break
				}
			}
			bodyIndent = indent
		} else if indent < bodyIndent {
			hitOuterCeiling = false
			break
		}
		// Consume the rest of this (indented, in-block) line, bounded by
		// maxValueScanBytes measured from THIS LINE's own start, not from
		// bodyStart: using bodyStart here would make the inner cap trip
		// spuriously on an ordinary short line simply because many prior
		// (fully consumed, unremarkable) lines had already accumulated
		// close to the overall ceiling, and treat that as "this one line
		// is pathologically long" when it manifestly is not. If this
		// single line is itself so long that it exhausts its own budget
		// before a line separator (or EOF) is found, there is no
		// legitimate "block ended here" signal at all -- unlike the outer
		// loop's indentation check, a mid-line stop is purely a budget
		// exhaustion, not a structural boundary. Extending to EOF in that
		// case avoids truncating the span mid-line and leaving the
		// remainder of that (still-in-block) line exposed in the clear.
		p := lineStart
		hitLineCeiling := true
		for p < len(s) && p-lineStart < maxValueScanBytes {
			if lw2, lok2 := isLineSep(s, p); lok2 {
				p += lw2
				hitLineCeiling = false
				break
			}
			p++
		}
		if hitLineCeiling {
			p = len(s)
		}
		pos = p
		bodyEnd = pos
	}
	if hitOuterCeiling && pos < len(s) {
		// The loop exited because pos-bodyStart reached maxValueScanBytes
		// while more of the document remained (pos < len(s)), not because
		// a real structural boundary (indentation dropping back to
		// keyIndent) or genuine end-of-input was found. Every completed
		// line up to this point was already folded into bodyEnd, but
		// everything after it -- still part of the same block, just past
		// the budget -- was not. Extending to EOF here matches the same
		// fix already applied to the bare/quoted/bracket value forms: the
		// ceiling bounds the SEARCH, never where the redaction stops.
		bodyEnd = len(s)
	}
	if bodyEnd == bodyStart {
		// No indented body followed the marker at all: nothing to redact,
		// but the header itself is still a recognized, structural match
		// (an empty block scalar is valid YAML), so resume right after it
		// without emitting a span.
		return bodyStart, true
	}
	if s[bodyStart:bodyEnd] == redactedToken+"\n" || s[bodyStart:bodyEnd] == redactedToken {
		// Already redacted (re-scrubbing a previous pass's output): no-op.
		return bodyEnd, true
	}

	// spanEnd is bodyEnd with the single line separator that TERMINATES the
	// body trimmed back off, when the loop above stopped because
	// indentation genuinely dropped back to keyIndent (hitOuterCeiling ==
	// false and pos < len(s): a real "next key starts here" boundary, not
	// the ceiling and not true EOF). That trailing separator is what
	// visually and structurally separates the block body from whatever
	// follows it on the next line, exactly the way a quoted value's closing
	// quote is structural and excluded from its own redacted span. Without
	// this, the separator was folded INTO the redacted span (it IS the
	// block body's own last line's terminator, so the "whole body must be
	// redacted" rule above included it), so [REDACTED] ended up glued
	// directly against the following key with no newline in between --
	// which is harmless on the FIRST Scrub pass (the output still reads
	// correctly as two lines to a human, because there IS a newline right
	// there) but is exactly what silently destroyed non-secret content on a
	// SECOND pass: yamlLineIndent measures indentation from the start of
	// the physical line, and with the separator gone, "[REDACTED]region:
	// eu-west-1" is one physical line starting at column 0, which then
	// satisfies this very same auto-detected bodyIndent==0 rule and gets
	// swallowed as more body -- silently deleting region: eu-west-1 with no
	// trace it was ever there. Preserving the separator keeps "next key
	// starts here" recognizable to a second pass exactly as it was to the
	// first.
	//
	// This is deliberately NOT applied when the block runs to true EOF or
	// when the ceiling was hit (bodyEnd forced to len(s) just above): there
	// is no following content in either case for the separator to
	// distinguish this body from, and doing so anyway would only diverge
	// from the pre-existing, still-correct column-zero-to-EOF behavior
	// (`"password: |\nTOPLEVELSECRET\n"` -> `"password: |\n[REDACTED]"`,
	// with no trailing newline) for no idempotence benefit: a body that
	// runs straight to EOF has nothing after it that a second pass could
	// ever misread as part of the block.
	spanEnd := bodyEnd
	if !hitOuterCeiling && pos < len(s) {
		if bodyEnd >= 2 && s[bodyEnd-2] == '\r' && s[bodyEnd-1] == '\n' {
			spanEnd = bodyEnd - 2
		} else if bodyEnd >= 1 && (s[bodyEnd-1] == '\n' || s[bodyEnd-1] == '\r' || s[bodyEnd-1] == '\f' || s[bodyEnd-1] == '\v') {
			spanEnd = bodyEnd - 1
		}
	}
	if spanEnd > bodyStart {
		*spans = append(*spans, span{Start: bodyStart, End: spanEnd})
	}
	return bodyEnd, true
}

// isIdentStart reports whether c can begin a credential-key identifier.
// Leading '_' or '-' are excluded since keys are words, not punctuation.
func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isBareValueByte reports whether b may be part of an unquoted assignment
// value. Whitespace and structural closers terminate the value. '#' also
// terminates it, exactly like the bracket openers `{[(` already do: it is
// the comment introducer the trailing-context check (below, in tryKeyAt)
// already accepts, but until now it was not itself a value terminator, so a
// '#' jammed directly against the value with no separating space (e.g.
// "password: SECRETVAL123# prod", a realistic .env/YAML/shell trailing
// comment with no space before the hash) was swallowed INTO the value. The
// value then stopped at the next space instead, landing the trailing-context
// check on the ordinary word after it ("prod") rather than on the '#' -- so
// the whole assignment was rejected as prose and the credential shipped in
// the clear, hits=0. Terminating on '#' here means the value now correctly
// stops at "SECRETVAL123", exactly matching the already-working
// "value # word" (space before '#') and "value#word" (no trailing space at
// all) shapes.
func isBareValueByte(b byte) bool {
	if isHSpace(b) || b == '\n' || b == '\r' || b == '\f' || b == '\v' {
		return false
	}
	switch b {
	case '&', '|', ';', ',', '}', ')', ']', '{', '[', '(', '#':
		return false
	}
	return true
}

// pemLine describes one physical line of the input for the purposes of PEM
// detection: its content with any single trailing line separator removed,
// and the byte offsets [Start,End) of that content (separator excluded)
// needed to reconstruct spans in the original string.
type pemLine struct {
	Text       string
	Start, End int
}

// splitPEMLines splits s into physical lines using the same separator
// classes as the rest of the scanner (CRLF, lone LF, lone CR, form feed,
// vertical tab), preserving byte offsets so callers can build spans.
func splitPEMLines(s string) []pemLine {
	var lines []pemLine
	start := 0
	i := 0
	for i < len(s) {
		if w, ok := isLineSep(s, i); ok {
			lines = append(lines, pemLine{Text: s[start:i], Start: start, End: i})
			i += w
			start = i
			continue
		}
		i++
	}
	if start < len(s) {
		lines = append(lines, pemLine{Text: s[start:], Start: start, End: len(s)})
	}
	return lines
}

// pemMaxScanLines bounds how many lines a truncated (no matching END) PEM
// block may consume before detection gives up. Without this bound, a
// header-shaped line inside ordinary prose could otherwise chain the
// "is this still PEM body" heuristic indefinitely and redact unrelated
// content far past the BEGIN marker.
//
// This bounds ONLY the truncated case. It deliberately does not bound how
// far the scan will look for a genuine matching END marker -- see
// pemMaxEndSearchLines for why conflating the two leaked realistic keys.
const pemMaxScanLines = 200

// pemMaxEndSearchLines bounds how many lines the scan will look ahead for a
// genuine matching END marker. It is separate from, and far larger than,
// pemMaxScanLines because the two bounds guard against opposite risks:
//
//   - pemMaxScanLines bounds OVER-redaction. A stray "-----BEGIN ...-----"
//     in prose or a log has no END marker, so whatever the heuristic
//     accepts as "still body-shaped" is what gets destroyed. That must stay
//     tight, and it does: this change leaves it at 200 lines.
//   - pemMaxEndSearchLines bounds only the SEARCH for a real terminator.
//     Finding a matching END marker is strong proof the block is a genuine
//     key, and the redacted span still stops exactly at that END marker --
//     so a generous budget here cannot over-redact unrelated content. The
//     only thing a small budget buys is UNDER-redaction, which is the
//     dangerous direction.
//
// Sharing one 200-line cap between the two is exactly what leaked: with
// conventional 64-byte PEM body lines the 200-line budget expires at ~13KB,
// so an ordinary passphrase-encrypted OpenSSH RSA 4096 key (~300 lines,
// ~19.5KB) never reached its own END marker. The span truncated at line 200
// and the remaining 100 body lines shipped in the clear while hits reported
// 1. For any conventional PEM line width (64-76 chars) the line cap always
// binds well before any byte-count concern, which is why the truncated
// outcome below extends straight to EOF on ranOutOfLineBudget alone rather
// than requiring a separate byte ceiling to also have been crossed -- see
// the ranOutOfLineBudget doc comment in scanPEMBlocks.
//
// 20000 lines is chosen with deliberate headroom over every realistic key
// shape, since under-redaction is the security defect and the cost of a
// large budget here is only scan time on input that is already known to
// start with a private-key marker:
//
//	RSA 4096 PEM                          ~50 lines    (~3.3KB)
//	OpenSSH ed25519                        ~7 lines    (~400B)
//	passphrase-encrypted OpenSSH RSA 4096 ~300 lines   (~19.5KB)
//	long certificate chains / PKCS#12     ~thousands
//
// That is ~40x the largest routine case and ~1.3MB of conventional-width
// body, while still being a hard bound: a stray BEGIN marker followed by
// megabytes of body-shaped content and no END marker still stops after
// pemMaxScanLines, never here, because the truncated path is what governs
// that input. Cost is bounded and linear: the look-ahead visits each line
// at most once per block and the scan resumes past the block, so total work
// stays O(len(input)) -- see the resume-index handling in scanPEMBlocks.
const pemMaxEndSearchLines = 20000

// pemMaxScanBytes bounds the total bytes a single truncated PEM block may
// consume, independent of pemMaxScanLines: a line-count cap alone is not
// enough, since pemBodyLineOK has no per-line length limit, so a body of
// FEWER lines than pemMaxScanLines can still consist of one (or a handful
// of) pathologically long body-shaped lines and destroy an unbounded amount
// of the document. This is what lets the extend-to-EOF fix below apply even
// when the block never runs out of LINE budget at all (e.g. a single
// 100000-byte body-shaped line, the whole "block" being only two lines
// long): a real PEM body line is conventionally 64-76 characters, so a
// single line comfortably past this ceiling is itself already proof there
// is no legitimate structural end in sight, the same proof
// ranOutOfLineBudget provides for the many-short-lines case.
const pemMaxScanBytes = 16384

// pemLabelIsDenied reports whether label names a marker type that must
// never be redacted even though it shares the "-----BEGIN ... -----"
// envelope: certificates and public keys are not secrets.
func pemLabelIsDenied(label string) bool {
	u := strings.ToUpper(strings.TrimSpace(label))
	return u == "CERTIFICATE" || strings.HasSuffix(u, "PUBLIC KEY") ||
		u == "TRUSTED CERTIFICATE" || u == "CERTIFICATE REQUEST" || u == "X509 CRL"
}

// pemBeginMarker reports whether line contains a "-----BEGIN <label>-----"
// marker (case-insensitive on BEGIN and the dashes, per the lowercase-marker
// requirement) and returns the label text between the dash runs along with
// the byte offset within line where the marker's opening dash run starts.
func pemBeginMarker(line string) (label string, at int, ok bool) {
	return pemMarker(line, 0, "BEGIN")
}

// pemBeginMarkerFrom is pemBeginMarker restricted to markers whose opening
// dash run starts at or after from, so a caller that rejects one marker on a
// line (e.g. a denied CERTIFICATE label) can resume the search past it
// instead of giving up on the whole line.
func pemBeginMarkerFrom(line string, from int) (label string, at int, ok bool) {
	return pemMarker(line, from, "BEGIN")
}

// pemEndMarker reports whether line contains an "-----END <label>-----"
// marker.
func pemEndMarker(line string) (label string, at int, ok bool) {
	return pemMarker(line, 0, "END")
}

// pemMarker matches <dashes><keyword> <label><dashes> anywhere on the line,
// where <dashes> is a run of three or more '-' characters (RFC 1421 uses
// five; some tools emit fewer). It returns the offset within line at which
// that opening dash run begins, so the caller can start the block at the
// marker itself rather than at the start of the line.
//
// Matching the marker only at (whitespace-trimmed) line start leaked whole
// private keys: any prefix at all -- a single space ("note -----BEGIN ..."),
// a shell prompt, or a word running straight into the dashes
// ("task-----BEGIN ...") -- hid the marker completely, and those shapes are
// routine in shell history and log lines, exactly the sources this scrubber
// reads. Scanning for the dash run anywhere on the line fixes that while
// preserving the text before it: the returned offset is where redaction
// begins, so the prefix is ordinary content and survives untouched.
//
// The closing dash run is located by scanning forward from the label (the
// first run of 3+ dashes after the label begins), not by trimming from the
// end of the line -- so a marker is still recognized even when key material
// runs directly into the marker's closing dashes with no separating newline
// (a realistic copy-paste-flattened or single-line-JSON-embedded shape). The
// caller treats the rest of the physical line as belonging to the block
// regardless of what follows the marker on it.
func pemMarker(line string, from int, keyword string) (label string, at int, ok bool) {
	if from < 0 {
		from = 0
	}
	// Try each run of 3+ dashes on the line as a candidate opening run: a
	// prefix may itself contain dashes (e.g. a diff marker or a hyphenated
	// word), so the first run found is not necessarily the marker's.
	for i := from; i < len(line); i++ {
		if line[i] != '-' {
			continue
		}
		runEnd := i
		for runEnd < len(line) && line[runEnd] == '-' {
			runEnd++
		}
		if runEnd-i < 3 {
			i = runEnd
			continue
		}
		if label, ok := pemMarkerAt(line, runEnd, keyword); ok {
			return label, i, true
		}
		i = runEnd - 1
	}
	return "", 0, false
}

// pemMarkerAt reports whether the keyword and label of a PEM marker follow
// at position i (just past an opening run of 3+ dashes), returning the label
// between that opening run and the next run of 3+ dashes.
func pemMarkerAt(line string, i int, keyword string) (label string, ok bool) {
	rest := line[i:]
	upperRest := strings.ToUpper(rest)
	if !strings.HasPrefix(upperRest, keyword) {
		return "", false
	}
	rest = rest[len(keyword):]
	rest = strings.TrimPrefix(rest, " ")
	// The label runs up to the first run of 3+ dashes found from here on.
	j := 0
	for j < len(rest) {
		if rest[j] != '-' {
			j++
			continue
		}
		k := j
		for k < len(rest) && rest[k] == '-' {
			k++
		}
		if k-j >= 3 {
			cand := strings.TrimSpace(rest[:j])
			if !pemLabelWellFormed(cand) || pemKeywordFollows(rest[k:]) {
				return "", false
			}
			return cand, true
		}
		j = k
	}
	// No closing dash run found anywhere on the line: not a well-formed
	// marker.
	return "", false
}

// pemKeywordFollows reports whether rest (the text immediately after a
// candidate marker's closing dash run) begins with BEGIN or END.
//
// That is the structural tell that the dash run just consumed as a "closing"
// run is really the OPENING run of a different marker, which means the text
// in between was never a label at all. It is what separates a genuine
// marker from a sentence discussing the PEM envelope:
//
//	-----BEGIN RSA PRIVATE KEY-----          closing run ends the line
//	see -----BEGIN and -----END markers ...  closing run opens "-----END"
//
// Before this check the second shape matched with label "and" and destroyed
// the sentence. Position cannot be the discriminator here (requiring the
// marker at line start is exactly the bug being fixed), so the structure of
// what follows the run is used instead. This only ever rejects candidates,
// and rejection does not abandon the line: pemMarker keeps scanning later
// dash runs, so the real "-----END ...-----" further along is still found on
// its own terms.
func pemKeywordFollows(rest string) bool {
	u := strings.ToUpper(rest)
	return strings.HasPrefix(u, "BEGIN") || strings.HasPrefix(u, "END")
}

// pemLabelMaxWords bounds how many space-separated words a PEM label may
// contain. The longest labels in real use are four words ("PGP PRIVATE KEY
// BLOCK", "ENCRYPTED PRIVATE KEY", "RSA PRIVATE KEY"); six leaves headroom
// for an unusual vendor label while still being far shorter than the
// sentence fragments this rejects.
const pemLabelMaxWords = 6

// pemLabelWellFormed reports whether label is plausibly a PEM label rather
// than an English sentence fragment that happens to sit between two dash
// runs.
//
// This check became necessary when marker detection stopped requiring the
// marker to sit at the start of its line (the fix for keys hidden behind any
// prefix). Matching at line start had accidentally bounded how far the
// closing dash run could be: once the scan may begin anywhere, a sentence
// that merely DISCUSSES the PEM envelope supplies both runs itself. Measured
// before this check: "see -----BEGIN and -----END markers explained in RFC
// 1421" matched with label "and" and destroyed the sentence -- a false
// positive on exactly the prose case the mid-line change had to stay clear
// of.
//
// The discriminator is the label text, not position, because position is
// what leaked. A real label (per RFC 7468 and every label in real use) is a
// short run of alphanumeric words joined by single spaces: "PRIVATE KEY",
// "RSA PRIVATE KEY", "PGP PRIVATE KEY BLOCK", and the lowercase spellings
// tools sometimes emit. Prose caught between two markers carries punctuation
// or runs long. Both bounds are deliberately loose: rejecting a genuine
// marker is the under-redaction direction, so anything label-shaped is
// accepted, and a rejected candidate does not abandon the line -- the caller
// keeps scanning later dash runs, so a real marker further along the same
// line is still found.
func pemLabelWellFormed(label string) bool {
	if label == "" {
		// "-----BEGIN-----" with no label at all: still an envelope, and
		// treating it as one is the over-redaction (safe) direction.
		return true
	}
	words := 1
	for k := 0; k < len(label); k++ {
		c := label[k]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == ' ':
			// Single spaces only; a double space is prose spacing or
			// column alignment, not a label.
			if k+1 >= len(label) || label[k+1] == ' ' {
				return false
			}
			words++
			if words > pemLabelMaxWords {
				return false
			}
		default:
			// Punctuation of any kind (including '.', ',', '"', '\\')
			// rules out a label.
			return false
		}
	}
	return true
}

// pemBodyLineOK reports whether line is consistent with being inside a PEM
// block: base64/base64url body content (possibly indented), an RFC 1421
// header line (Proc-Type:, DEK-Info:, Comment:, Version:), or blank (the
// separator between headers and the base64 body).
func pemBodyLineOK(line string) bool {
	t := strings.TrimRight(line, " \t")
	trimmed := strings.TrimLeft(t, " \t")
	if trimmed == "" {
		return true
	}
	for _, hdr := range []string{"Proc-Type:", "DEK-Info:", "Comment:", "Version:"} {
		if strings.HasPrefix(trimmed, hdr) {
			return true
		}
	}
	for k := 0; k < len(trimmed); k++ {
		c := trimmed[k]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '+', c == '/', c == '=', c == '-', c == '_':
			// base64 and base64url alphabets, plus '=' padding.
		default:
			return false
		}
	}
	return true
}

// scanPEMBlocks walks s line by line looking for "-----BEGIN ...-----"
// markers and returns the spans to redact. A complete block (a matching
// END marker for the same label found before pemMaxScanLines elapse) is
// redacted in full, markers included. A truncated block (no END found) is
// redacted from BEGIN through the last line that still looks like PEM
// header/body content, bounded by pemMaxScanLines so a stray marker
// followed by ordinary prose cannot consume the rest of the document.
// CERTIFICATE and PUBLIC KEY labels are never redacted.
func scanPEMBlocks(s string) []span {
	lines := splitPEMLines(s)
	// endMarkers lists every END-marker line (index and label), ascending, from
	// a single O(len(lines)) pre-pass, so a BEGIN marker can locate its
	// matching END directly instead of walking up to pemMaxEndSearchLines
	// lines to find out there isn't one.
	//
	// This became load-bearing when the non-body-line case stopped breaking
	// out of the search loop (see below): before that change a stray BEGIN
	// followed by prose gave up after one line; after it, every stray BEGIN
	// walked the full look-ahead budget. Measured on 20,000 stray BEGIN
	// markers each followed by one prose line (1.6MB total), that regressed
	// 87ms to 11.96s -- quadratic in the number of stray markers, at input
	// sizes (shell history, a JSONL transcript) this scrubber sees routinely.
	//
	// Matching here uses the identical strings.EqualFold comparison the
	// search loop uses, not a normalized map key: a normalization that
	// disagreed with EqualFold on any input would silently skip a search
	// that would have found a real END, trading under-redaction for speed,
	// which is the wrong direction for this file. Cost is O(number of END
	// markers) per BEGIN, and an input with many END markers is one where
	// the search terminates quickly anyway.
	type pemEndRef struct {
		line  int
		label string
	}
	var endMarkers []pemEndRef
	for n := range lines {
		if endLabel, _, ok := pemEndMarker(lines[n].Text); ok {
			endMarkers = append(endMarkers, pemEndRef{line: n, label: endLabel})
		}
	}
	// matchingEndAfter reports the index of the first END marker line for
	// label strictly after line n, or -1 if there is none.
	matchingEndAfter := func(label string, n int) int {
		lo, hi := 0, len(endMarkers)
		for lo < hi {
			mid := (lo + hi) / 2
			if endMarkers[mid].line <= n {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		for ; lo < len(endMarkers); lo++ {
			if strings.EqualFold(endMarkers[lo].label, label) {
				return endMarkers[lo].line
			}
		}
		return -1
	}
	var spans []span

	i := 0
	for i < len(lines) {
		// A line may carry more than one marker now that markers are found
		// mid-line, so a denied label (CERTIFICATE, PUBLIC KEY) must not
		// abandon the line: keep searching past it for a marker that is not
		// denied. Skipping the whole line under-redacted, measured:
		// "password=hunter2-----BEGIN CERTIFICATE------- separator
		// -------BEGIN RSA PRIVATE KEY-----" found only the CERTIFICATE,
		// was denied, and left the private-key marker after it unexamined.
		label, markerAt := "", 0
		ok := false
		for from := 0; from <= len(lines[i].Text); {
			l, at, found := pemBeginMarkerFrom(lines[i].Text, from)
			if !found {
				break
			}
			if !pemLabelIsDenied(l) {
				label, markerAt, ok = l, at, true
				break
			}
			from = at + 1
		}
		if !ok {
			i++
			continue
		}

		// Start the block at the marker itself, not at the start of its
		// line: whatever precedes the marker on that line ("note ", "$ echo ",
		// or a word jammed straight against the dashes) is ordinary content
		// and must survive, exactly as it already does when the marker sits
		// on its own line after prose.
		blockStart := lines[i].Start + markerAt
		// lastBodyEnd tracks the absolute end offset of the block so far.
		// It starts at the end of the BEGIN line (not just the marker
		// itself) so that key material running directly into the marker
		// with no separating newline is still covered even if no further
		// lines turn out to belong to the block.
		lastBodyEnd := lines[i].End
		end := -1
		// Two separate budgets, guarding opposite risks (see the constants):
		// limit bounds the search for a genuine matching END marker and is
		// generous, because finding one proves the block is real and the
		// span still stops at that END. truncatedLimit bounds how much a
		// stray, never-terminated marker may consume, and stays tight.
		// Sharing one budget between them is what leaked realistic keys.
		limit := i + 1 + pemMaxEndSearchLines
		if limit > len(lines) {
			limit = len(lines)
		}
		// unclampedTruncatedLimit is i+1+pemMaxScanLines BEFORE clamping to
		// len(lines): kept alongside truncatedLimit (the clamped value used
		// everywhere else) so the loop below can tell apart "gave up
		// because the line BUDGET ran out while more input remained" from
		// "reached this point because the document simply ended here" --
		// see ranOutOfLineBudget's own doc comment for why that
		// distinction is what decides whether extending to EOF is safe.
		unclampedTruncatedLimit := i + 1 + pemMaxScanLines
		truncatedLimit := unclampedTruncatedLimit
		if truncatedLimit > len(lines) {
			truncatedLimit = len(lines)
		}
		// Clamp the END search to where its matching END actually is (or to
		// the tight truncated budget if there is none within the look-ahead
		// budget). Semantically identical to searching the full budget:
		// every truncated-outcome variable in the loop below is already
		// gated on j < truncatedLimit, so lines past truncatedLimit only
		// ever mattered for finding the END this clamp already located.
		// Without it, a document full of stray BEGIN markers is quadratic.
		if ne := matchingEndAfter(label, i); ne >= 0 && ne < limit {
			limit = ne + 1
		} else if limit > truncatedLimit {
			limit = truncatedLimit
		}
		// pemMaxScanBytes does not break the loop early on its own: a
		// single body-shaped line crossing the byte total only proves the
		// block is long, not that it has ended, so scanning for
		// pemBodyLineOK/the END marker continues regardless of the byte
		// total, bounded only by the line budget (the j < limit loop
		// condition). hitByteCeiling just records whether that total was
		// ever crossed, for the extend-to-EOF decision below.
		//
		// ranOutOfLineBudget records a second, independent ambiguous
		// outcome: the loop stopping because j reached the truncated-block
		// budget while every line up to and including the last one checked
		// was still body-shaped and no END was found -- "gave up
		// searching," not "a real end was reached." Neither signal alone
		// covers every leaked shape: hitByteCeiling alone misses a
		// truncated block made of many CONVENTIONALLY SHORT lines (with
		// 64-byte lines the 200-line budget binds at ~13KB, under the
		// 16384-byte ceiling, so the byte ceiling never crosses for
		// exactly that realistic width); ranOutOfLineBudget alone misses a
		// truncated block that is only a HANDFUL of lines long but one of
		// them is pathologically long (the line-count budget is nowhere
		// close to exhausted, so ranOutOfLineBudget never sets true, even
		// though the byte ceiling proves the same "no structural end in
		// sight" fact just as well). Both are therefore checked, and
		// either one crossing is sufficient to extend lastBodyEnd through
		// EOF instead of stopping at the last line/byte examined -- see
		// the extend-to-EOF check below for the combined condition and the
		// one further refinement (unclampedTruncatedLimit) that keeps a
		// truncated block whose LINES simply run out at or before
		// pemMaxScanLines (no more input existed to examine in the first
		// place) from being swept into the same treatment as a block that
		// was cut off by the budget with more content still unexamined.
		//
		// If the loop instead stopped because a line genuinely stopped
		// looking body-shaped, lastBodyEnd already correctly points at the
		// real end of the block and must NOT be extended past it into
		// unrelated document content, regardless of either signal.
		//
		// lastBodyEnd and truncatedEnd describe the TRUNCATED outcome only,
		// so they stop accumulating at truncatedLimit even though the
		// END-marker search itself continues to limit. That separation is
		// the fix: the search may range far enough to find a real 300-line
		// key's END marker, while a stray marker that never terminates
		// still destroys no more of the document than pemMaxScanLines ever
		// allowed.
		hitByteCeiling := false
		ranOutOfLineBudget := true
		truncatedEnd := truncatedLimit
		// firstNonBody is the index of the first line that stopped looking
		// like PEM body/header content, or -1 if none has been seen.
		firstNonBody := -1
		// trailingRunStart is the line index where the CURRENT run of
		// consecutive non-body lines began, or -1 while on a body-OK line.
		// Combined with trailingRunLen it lets the loop, once it runs out
		// of input/budget, recognize a TRAILING run of exactly one
		// non-body line and extend the span through it -- see below.
		trailingRunStart := -1
		trailingRunLen := 0
		j := i + 1
		for ; j < limit; j++ {
			if j < truncatedLimit && lines[j].End-blockStart > pemMaxScanBytes {
				hitByteCeiling = true
			}
			if endLabel, _, endOK := pemEndMarker(lines[j].Text); endOK && strings.EqualFold(endLabel, label) {
				end = j
				ranOutOfLineBudget = false
				break
			}
			if !pemBodyLineOK(lines[j].Text) {
				// A line that is not body-shaped ends the block for the
				// TRUNCATED outcome only. It must not stop the search for a
				// genuine matching END marker, which is the same split
				// already drawn between limit and truncatedLimit above:
				// finding a matching END is proof the block is a real key,
				// and that proof outranks any guess made from the shape of
				// an intervening line.
				//
				// Breaking out of the whole loop here leaked verbatim key
				// material. pemBodyLineOK's allowlist covers only the RFC
				// 1421 headers (Proc-Type:, DEK-Info:, Comment:, Version:),
				// but real blocks carry other shapes: `openssl pkcs12
				// -nodes` emits "Key Attributes:" and "Bag Attributes:"
				// lines inside the block, and a terminal- or word-wrapped
				// paste puts a space, tab, period, percent or quote inside
				// a body line. Each of those froze the span at that line
				// while the matching END sat a line or two below, so the
				// BEGIN line alone was replaced, hits reported 1, and the
				// entire body shipped in the clear -- the same "secret cut
				// in half, success reported" class fixed elsewhere in this
				// file.
				if firstNonBody < 0 {
					firstNonBody = j
					if j < truncatedLimit {
						ranOutOfLineBudget = false
						truncatedEnd = j
					}
				}
				if trailingRunStart < 0 {
					trailingRunStart = j
				}
				trailingRunLen++
				continue
			}
			// Body-OK line: extend the truncated span through it
			// unconditionally, whether or not an earlier non-body line was
			// already seen. A truncated block (no END anywhere) has no
			// later body line to PROVE where the key actually ends, so
			// every body-shaped line found -- including one that follows an
			// intervening non-body-shaped line -- is genuine key material
			// and must be covered. Freezing the span at the first non-body
			// line (the previous behavior) left every body-OK line after an
			// intervening "Key Attributes:"-style header, or after a single
			// garbled body line with no separating newline, in the clear
			// while hits still reported 1 -- the same "secret cut in half,
			// success reported" class this whole block already guards
			// against, just one non-body line earlier than the case that
			// was fixed first.
			if j < truncatedLimit {
				lastBodyEnd = lines[j].End
				truncatedEnd = j + 1
			}
			trailingRunStart, trailingRunLen = -1, 0
		}
		// The loop ran out of input/budget while still inside a run of
		// non-body lines (trailingRunStart >= 0). If that run is exactly
		// ONE line long, extend the span through it regardless of whether
		// any body-OK line preceded it: real base64 body content commonly
		// ends with exactly one line corrupted by trailing paste artifacts
		// or closing punctuation (e.g. "...FuNfBj4==)"), and that one line
		// is the block's actual last content -- nothing follows to prove
		// otherwise. This covers both the original single-non-body-line
		// case (no body-OK line ever found: "-----BEGIN...-----\nAAAA...
		// production") and a corrupted FINAL line after otherwise-good
		// body content (e.g. a "Key Attributes:" line followed by more
		// body, whose own last line then picks up a trailing '|' from an
		// adjacent context).
		//
		// A run of TWO OR MORE trailing non-body lines is deliberately
		// left uncovered here, even when body-OK lines exist earlier in
		// the block: that shape is indistinguishable, using pemBodyLineOK
		// alone, from a genuine truncated block whose matching END has a
		// DIFFERENT label (so what follows the one real body line reads as
		// ordinary prose) -- an existing, deliberately bounded case that
		// must NOT have its trailing prose swept in. The narrower
		// multi-line variant of the leaked-tail defect this leaves
		// uncovered (no body-OK line ever found, AND two or more trailing
		// non-body lines) is tracked as an open finding in the fuzz
		// registry instead of "fixed" by a heuristic that would regress
		// that bound.
		if trailingRunStart >= 0 && trailingRunLen == 1 && trailingRunStart < truncatedLimit {
			lastBodyEnd = lines[trailingRunStart].End
			// Resume past this now-covered line, not at its start: leaving
			// truncatedEnd at trailingRunStart (the value set when this
			// line first failed pemBodyLineOK, before it was known to be
			// the confirmed trailing line) would re-examine text already
			// folded into the redacted span.
			truncatedEnd = trailingRunStart + 1
		}

		if end >= 0 {
			blockEnd := lines[end].End
			spans = append(spans, span{Start: blockStart, End: blockEnd})
			i = end + 1
			continue
		}

		// Truncated: redact through the last recognizable body/header
		// line (or, if nothing followed the BEGIN line, through the end
		// of the BEGIN line itself -- covering key material that runs
		// directly into the marker with no separating newline), bounded
		// by pemMaxScanLines regardless of how far the caller's input
		// continues.
		//
		// Extend to EOF when either hard signal fired:
		//
		//   - hitByteCeiling: the block (however few lines) already
		//     crossed pemMaxScanBytes, so its true extent is unknown
		//     regardless of the line count.
		//   - ranOutOfLineBudget, narrowed by unclampedTruncatedLimit <
		//     len(lines): the line BUDGET (not the document itself) is
		//     what actually stopped the loop -- i.e. more lines existed
		//     beyond pemMaxScanLines that were never even examined. When
		//     the document simply ends at or before that budget
		//     (unclampedTruncatedLimit >= len(lines)), the loop's own
		//     natural exit already IS the real end of the input, and
		//     lastBodyEnd already points at the last body-shaped line's
		//     own end -- one byte short of a trailing newline that carries
		//     no key material. Extending to EOF in that narrower case
		//     would only consume that harmless trailing newline, not fix
		//     any leak, so it is excluded (that is exactly what
		//     hitByteCeiling above is left to catch instead, when the
		//     single line itself is long enough to warrant it).
		if hitByteCeiling || (ranOutOfLineBudget && unclampedTruncatedLimit < len(lines)) {
			lastBodyEnd = len(s)
		}
		spans = append(spans, span{Start: blockStart, End: lastBodyEnd})
		// Resume scanning from the line after the last one consumed. This
		// uses the truncated budget's stopping point, not j: j may have run
		// far ahead searching (unsuccessfully) for an END marker, and
		// skipping the scanner past all those lines would blind it to any
		// further BEGIN markers among them, none of which were redacted.
		// (A `j < truncatedEnd` clamp used to sit here for the case where
		// the loop broke early; the loop no longer breaks on a non-body
		// line, so on this path j is always limit, which is >=
		// truncatedLimit >= truncatedEnd, and the clamp could never fire.)
		i = truncatedEnd
	}

	return spans
}

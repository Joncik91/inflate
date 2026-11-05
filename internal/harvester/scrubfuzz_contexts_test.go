package harvester

import (
	"encoding/json"
	"fmt"
	"strings"
)

// --- Differential fuzzer: contexts ------------------------------------------
//
// The context axis is where every real leak this scrubber has shipped
// actually lived. Not one of them was a missing vendor prefix: they were
// character adjacencies and enclosing structures nobody enumerated. A
// vendor key jammed against a word character. A colon-assigned value
// followed by " # comment". A PEM BEGIN marker that was not at line start.
// A JSON blob escaped inside another JSON string. So this axis is
// deliberately the larger of the two, and it enumerates adjacency
// explicitly (every prefix byte class, every quote character, every
// terminator) rather than sampling "realistic" documents.

// scrubContext wraps a secret shape in surrounding structure.
//
// Accepts lets a context decline shapes it cannot meaningfully embed (a
// multi-line PEM block inside a single-line .env value, for instance): the
// point of declining is that such a case tests nothing about the scrubber,
// not that its result would be acceptable.
type scrubContext struct {
	Name    string
	Accepts func(secretShape) bool
	Build   func(secretShape) scrubCase
}

// anyShape accepts every shape.
func anyShape(secretShape) bool { return true }

// singleLineOnly rejects shapes whose literal spans multiple lines (PEM),
// for contexts that are line-structured.
func singleLineOnly(s secretShape) bool { return !strings.Contains(s.Text, "\n") }

// selfContainedOnly accepts only shapes that are expected to be redacted on
// their own merit (vendor literals, PEM blocks), for contexts that embed
// the literal with no credential key attached to it.
func selfContainedOnly(s secretShape) bool { return s.SelfContained }

// selfContainedSingleLine is both of the above.
func selfContainedSingleLine(s secretShape) bool {
	return s.SelfContained && singleLineOnly(s)
}

// mkCase builds a scrubCase from a shape, a context name, and the fully
// assembled input.
func mkCase(s secretShape, ctxName, input string) scrubCase {
	return scrubCase{
		Name:            s.Name + "/" + ctxName,
		Input:           input,
		Secret:          s.Secret,
		Shape:           s.Name,
		Context:         ctxName,
		ExpectRedaction: true,
	}
}

// fuzzJamPrefixes are the byte classes that can immediately precede a
// credential with no separator. The word-character entries are the ones
// that matter: a leading `\b` in a vendor pattern asserts a word/non-word
// transition, so a key preceded by a letter, digit or underscore has no
// boundary to assert and the pattern does not match at all -- the key ships
// in the clear while hits reports 0. That shape is routine in the sources
// this scrubber reads (concatenated env dumps, log lines written without
// separators, flattened JSON), not exotic.
var fuzzJamPrefixes = []struct {
	Name string
	Text string
}{
	{"bare", ""},
	{"space", " "},
	{"tab", "\t"},
	{"newline", "\n"},
	{"jam-letter", "X"},
	{"jam-digit", "7"},
	{"jam-underscore", "_"},
	{"jam-word", "prefixword"},
	{"jam-hyphen", "-"},
	{"jam-equals", "="},
	{"jam-colon", ":"},
	{"jam-slash", "/"},
	{"jam-dot", "."},
	{"prose-then-space", "the value is "},
}

// fuzzTrailSuffixes are what can immediately follow a credential. The
// comment and parenthetical entries are the shapes that leaked: a
// colon-assigned value followed by " # prod", " (prod)" or a tab-separated
// column was dropped entirely by an over-strict "what may terminate a
// value" check that only accepted structural closers.
var fuzzTrailSuffixes = []struct {
	Name string
	Text string
}{
	{"eof", ""},
	{"newline", "\n"},
	{"crlf", "\r\n"},
	{"space-hash-comment", " # production credential"},
	{"hash-comment-no-space", "# production credential"},
	{"space-slashslash-comment", " // production credential"},
	{"space-semicolon-comment", " ; production credential"},
	{"paren-annotation", " (production)"},
	{"tab-column", "\tproduction"},
	// Still generated, deliberately. After a COLON-assigned unquoted value
	// this is the one shape scanner.go declines on purpose (it is prose,
	// "Secret: this document is confidential"), and the oracle recognizes
	// that structurally via isProsePolicyShape. With '=' or with the value
	// quoted the ambiguity is gone, so those combinations are still
	// expected to redact and this entry is what checks them.
	{"space-word", " production"},
	{"comma", ","},
	{"semicolon", ";"},
	{"close-brace", "}"},
	{"close-bracket", "]"},
	{"close-paren", ")"},
	{"ampersand", "&"},
	{"pipe", "|"},
	{"jam-letter", "X"},
}

// adjacencyContexts crosses jamming prefixes against trailing suffixes for
// self-contained (vendor / PEM) literals embedded with no assignment key.
// This is the axis that finds anchor defects: a pattern that only matches
// at a word boundary fails for every jam-* prefix, and a pattern anchored
// on the right fails for every jam-* suffix.
func adjacencyContexts() []scrubContext {
	var out []scrubContext
	for _, p := range fuzzJamPrefixes {
		for _, sfx := range fuzzTrailSuffixes {
			p, sfx := p, sfx
			out = append(out, scrubContext{
				Name:    "adjacency/" + p.Name + "+" + sfx.Name,
				Accepts: selfContainedOnly,
				Build: func(s secretShape) scrubCase {
					return mkCase(s, "adjacency/"+p.Name+"+"+sfx.Name, p.Text+s.Text+sfx.Text)
				},
			})
		}
	}
	return out
}

// fuzzAssignOps are the operator spellings an assignment can use,
// including the whitespace variants around them. A colon-assigned value is
// governed by a stricter trailing-context rule than an '='-assigned one, so
// both must be crossed against every terminator.
var fuzzAssignOps = []struct {
	Name string
	Text string
}{
	{"eq", "="},
	{"eq-spaced", " = "},
	{"eq-space-after", "= "},
	{"colon", ":"},
	{"colon-space", ": "},
	{"colon-spaced", " : "},
	{"colon-tab", ":\t"},
}

// fuzzValueWrappers are the delimiters a value can be wrapped in.
var fuzzValueWrappers = []struct {
	Name  string
	Open  string
	Close string
}{
	{"unquoted", "", ""},
	{"double-quote", `"`, `"`},
	{"single-quote", "'", "'"},
	{"backtick", "`", "`"},
	{"brace", "{", "}"},
	{"bracket", "[", "]"},
	{"paren", "(", ")"},
	{"unterminated-double-quote", `"`, ""},
	{"unterminated-single-quote", "'", ""},
	{"unterminated-brace", "{", ""},
	{"unterminated-bracket", "[", ""},
}

// fuzzAssignLeadIns are what can precede the credential KEY on the same
// line. These matter because scanner.go rejects a key whose immediately
// preceding non-space byte is itself an assignment operator (the
// "chained label" rule, written to reject prose like "Error: token:
// invalid"). That rule cannot distinguish prose from the very common
// shapes below, where the preceding operator belongs to an outer,
// unrelated assignment: `--env=PGPASSWORD=x`, `config: password=x`,
// `?token=x` in a query string.
var fuzzAssignLeadIns = []struct {
	Name string
	Text string
}{
	{"none", ""},
	{"line-start", "\n"},
	{"export", "export "},
	{"flag-eq", "--env="},
	{"outer-colon", "config: "},
	{"query-string", "https://api.example.com/v1?"},
	{"docker-e-flag", "docker run -e "},
	{"bracket-open", "args=["},
	// An unbalanced quote earlier in the document leaves the scanner's
	// quote-state machine "inside a string" for everything that follows,
	// including across line boundaries. Since a colon-keyed assignment is
	// only trusted at top level, one stray apostrophe or double quote can
	// disable colon-keyed detection for the entire remainder of the input.
	// Both triggers below are ordinary: an English possessive in a prose
	// line, and an unterminated quoted field in a log line.
	{"stray-apostrophe-earlier-line", "it's a config file\n"},
	{"unterminated-double-quote-earlier-line", "msg=\"unterminated\n"},
	{"unterminated-double-quote-same-line", "\""},
}

// assignmentContexts crosses lead-in, operator spelling, value wrapper, and
// trailing terminator for every shape (vendor literals get a credential key
// wrapped around them too, since that is how they appear in a real .env or
// config).
func assignmentContexts() []scrubContext {
	var out []scrubContext
	// Lead-ins are crossed against both operators, because the two defect
	// classes they expose split on exactly that: the chained-label rule
	// fires on '=' assignments preceded by an operator, while the
	// quote-state defect fires on ':' assignments after an unbalanced quote
	// (a colon-keyed assignment is only trusted at top level, so anything
	// that leaves the scanner "inside a string" disables it).
	for _, lead := range fuzzAssignLeadIns {
		for _, op := range []string{"=", ": "} {
			lead, op := lead, op
			if lead.Text == "" {
				continue // the no-lead-in case is the main loop below
			}
			opName := "eq"
			if op != "=" {
				opName = "colon"
			}
			name := "leadin/" + lead.Name + "+" + opName
			out = append(out, scrubContext{
				Name:    name,
				Accepts: singleLineOnly,
				Build: func(s secretShape) scrubCase {
					return mkCase(s, name, lead.Text+s.AssignmentKey+op+s.AssignmentValue)
				},
			})
		}
	}
	for _, op := range fuzzAssignOps {
		for _, w := range fuzzValueWrappers {
			for _, sfx := range fuzzTrailSuffixes {
				op, w, sfx := op, w, sfx
				name := "assign/" + op.Name + "+" + w.Name + "+" + sfx.Name
				// No exemption is applied here. The documented prose policy
				// (colon operator + unquoted value + an ordinary word after
				// a single space) is recognized structurally by
				// isProsePolicyShape in the oracle, so the table test and
				// the fuzz target -- whose mutator can build that shape from
				// affixes no context chose -- apply exactly the same rule
				// from one definition.
				out = append(out, scrubContext{
					Name:    name,
					Accepts: singleLineOnly,
					Build: func(s secretShape) scrubCase {
						in := s.AssignmentKey + op.Text + w.Open + s.AssignmentValue + w.Close + sfx.Text
						return mkCase(s, name, in)
					},
				})
			}
		}
	}
	return out
}

// jsonContexts embed the credential in JSON documents, including the
// doubly-escaped shape that a Claude Code JSONL transcript actually
// produces: a JSON string field whose CONTENT is itself a serialized JSON
// blob, so every quote in the inner document arrives as \" and the inner
// "password":"x" pair is invisible to a scanner that only understands one
// level of quoting.
func jsonContexts() []scrubContext {
	mustMarshal := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			panic(err) // generator bug, not a scrubber finding
		}
		return string(b)
	}
	return []scrubContext{
		{
			Name:    "json/flat-object",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				in := mustMarshal(map[string]string{s.AssignmentKey: s.AssignmentValue})
				return mkCase(s, "json/flat-object", in)
			},
		},
		{
			Name:    "json/nested-object",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				in := mustMarshal(map[string]any{
					"service": "api",
					"config":  map[string]string{s.AssignmentKey: s.AssignmentValue, "region": "eu-west-1"},
				})
				return mkCase(s, "json/nested-object", in)
			},
		},
		{
			Name:    "json/pretty-printed",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				b, err := json.MarshalIndent(map[string]string{s.AssignmentKey: s.AssignmentValue}, "", "  ")
				if err != nil {
					panic(err)
				}
				return mkCase(s, "json/pretty-printed", string(b))
			},
		},
		{
			Name:    "json/array-element",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				in := mustMarshal([]any{"first", map[string]string{s.AssignmentKey: s.AssignmentValue}})
				return mkCase(s, "json/array-element", in)
			},
		},
		{
			// The Claude Code JSONL transcript shape: an outer JSON object
			// whose "content" string holds a serialized inner JSON document.
			Name:    "json/doubly-escaped-transcript",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				inner := mustMarshal(map[string]string{s.AssignmentKey: s.AssignmentValue})
				in := mustMarshal(map[string]any{
					"type": "user", "content": inner,
				})
				return mkCase(s, "json/doubly-escaped-transcript", in)
			},
		},
		{
			// Same shape, hand-built rather than round-tripped through
			// encoding/json, so the exact byte sequence a transcript writer
			// emits (no key reordering, no whitespace) is exercised too.
			Name:    "json/doubly-escaped-literal",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				esc := strings.ReplaceAll(s.AssignmentValue, `\`, `\\`)
				esc = strings.ReplaceAll(esc, `"`, `\"`)
				in := fmt.Sprintf(`{"content":"{\"%s\":\"%s\"}"}`, s.AssignmentKey, esc)
				return mkCase(s, "json/doubly-escaped-literal", in)
			},
		},
		{
			// The transcript shape preceded by one line of ordinary English
			// prose containing a possessive apostrophe. That apostrophe is
			// an unbalanced quote, and it is the single most likely byte to
			// appear in a real JSONL transcript before a credential blob.
			Name:    "json/doubly-escaped-after-prose-apostrophe",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				esc := strings.ReplaceAll(s.AssignmentValue, `\`, `\\`)
				esc = strings.ReplaceAll(esc, `"`, `\"`)
				in := "it's the deploy config\n" +
					fmt.Sprintf(`{"content":"{\"%s\":\"%s\"}"}`, s.AssignmentKey, esc)
				return mkCase(s, "json/doubly-escaped-after-prose-apostrophe", in)
			},
		},
		{
			// Triply-escaped: a transcript quoting a transcript. One more
			// level than anything realistic, included because each level is
			// a separate chance for a one-level-deep quote state machine to
			// lose track of where the value ends.
			Name:    "json/triply-escaped",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				l1 := mustMarshal(map[string]string{s.AssignmentKey: s.AssignmentValue})
				l2 := mustMarshal(map[string]any{"content": l1})
				in := mustMarshal(map[string]any{"content": l2})
				return mkCase(s, "json/triply-escaped", in)
			},
		},
		{
			Name:    "json/value-in-free-text",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := fmt.Sprintf(`{"content":"here is the config %s=%s please store it"}`,
					s.AssignmentKey, s.AssignmentValue)
				return mkCase(s, "json/value-in-free-text", in)
			},
		},
		{
			// A JSON string whose free-text content contains an assignment
			// with a single-quoted value. The quote opens a value scan that
			// has no matching close inside the string, so the span runs to
			// EOF and takes the JSON's own closing delimiters with it. Well
			// formed JSON in, truncated garbage out.
			Name:    "json/single-quoted-value-inside-string",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				esc := strings.ReplaceAll(s.AssignmentValue, `\`, `\\`)
				esc = strings.ReplaceAll(esc, `"`, `\"`)
				in := fmt.Sprintf(`{"note":"%s='%s"}`, s.AssignmentKey, esc)
				return mkCase(s, "json/single-quoted-value-inside-string", in)
			},
		},
		{
			// A credential key whose '=' is the LAST content byte of a JSON
			// string, so the string's own closing quote is the next
			// non-space byte after the operator. The scanner reads that
			// closing quote as the value's OPENING quote, finds no match,
			// and runs to EOF -- emitting a placeholder past the end of the
			// document and destroying its structure.
			Name:    "json/credential-key-at-end-of-string",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := fmt.Sprintf(`{"note":"see %s="}`, s.AssignmentKey)
				// No secret is planted: this case exists for the JSON
				// preservation property, not the leak property.
				c := mkCase(s, "json/credential-key-at-end-of-string", in)
				c.Secret = ""
				return c
			},
		},
		{
			Name:    "json/unterminated-object",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := fmt.Sprintf(`{"%s":"%s`, s.AssignmentKey, s.AssignmentValue)
				return mkCase(s, "json/unterminated-object", in)
			},
		},
	}
}

// yamlContexts embed the credential in YAML at several indentation levels
// and in both inline and block-scalar value forms.
func yamlContexts() []scrubContext {
	return []scrubContext{
		{
			Name:    "yaml/column-zero",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "service: api\n" + s.AssignmentKey + ": " + s.AssignmentValue + "\nregion: eu-west-1\n"
				return mkCase(s, "yaml/column-zero", in)
			},
		},
		{
			Name:    "yaml/indented",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "config:\n  " + s.AssignmentKey + ": " + s.AssignmentValue + "\n  region: eu-west-1\n"
				return mkCase(s, "yaml/indented", in)
			},
		},
		{
			Name:    "yaml/deeply-indented",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "a:\n  b:\n    c:\n        " + s.AssignmentKey + ": " + s.AssignmentValue + "\n"
				return mkCase(s, "yaml/deeply-indented", in)
			},
		},
		{
			Name:    "yaml/list-item",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "envs:\n  - " + s.AssignmentKey + ": " + s.AssignmentValue + "\n"
				return mkCase(s, "yaml/list-item", in)
			},
		},
		{
			Name:    "yaml/quoted-key",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "\"" + s.AssignmentKey + "\": \"" + s.AssignmentValue + "\"\n"
				return mkCase(s, "yaml/quoted-key", in)
			},
		},
		{
			Name:    "yaml/block-scalar-literal",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				var b strings.Builder
				b.WriteString(s.AssignmentKey + ": |\n")
				for _, line := range strings.Split(s.AssignmentValue, "\n") {
					b.WriteString("  " + line + "\n")
				}
				b.WriteString("region: eu-west-1\n")
				return mkCase(s, "yaml/block-scalar-literal", b.String())
			},
		},
		{
			Name:    "yaml/block-scalar-folded-column-zero",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				var b strings.Builder
				b.WriteString(s.AssignmentKey + ": >\n")
				for _, line := range strings.Split(s.AssignmentValue, "\n") {
					b.WriteString(line + "\n")
				}
				return mkCase(s, "yaml/block-scalar-folded-column-zero", b.String())
			},
		},
		{
			Name:    "yaml/flow-mapping",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "config: {" + s.AssignmentKey + ": " + s.AssignmentValue + ", region: eu-west-1}\n"
				return mkCase(s, "yaml/flow-mapping", in)
			},
		},
	}
}

// envAndShellContexts embed the credential in the line-oriented formats
// this scrubber actually reads: .env files, exported shell variables,
// command lines, and shell history.
func envAndShellContexts() []scrubContext {
	return []scrubContext{
		{
			Name:    "env/plain-line",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "# config\n" + strings.ToUpper(strings.ReplaceAll(s.AssignmentKey, "-", "_")) +
					"=" + s.AssignmentValue + "\nDEBUG=1\n"
				return mkCase(s, "env/plain-line", in)
			},
		},
		{
			Name:    "env/export-prefixed",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "export " + strings.ToUpper(strings.ReplaceAll(s.AssignmentKey, "-", "_")) +
					"=" + s.AssignmentValue + "\n"
				return mkCase(s, "env/export-prefixed", in)
			},
		},
		{
			Name:    "env/no-trailing-newline",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "DEBUG=1\n" + strings.ToUpper(strings.ReplaceAll(s.AssignmentKey, "-", "_")) +
					"=" + s.AssignmentValue
				return mkCase(s, "env/no-trailing-newline", in)
			},
		},
		{
			Name:    "env/crlf-line-endings",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "DEBUG=1\r\n" + strings.ToUpper(strings.ReplaceAll(s.AssignmentKey, "-", "_")) +
					"=" + s.AssignmentValue + "\r\nOTHER=2\r\n"
				return mkCase(s, "env/crlf-line-endings", in)
			},
		},
		{
			Name:    "shell/inline-env-command",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := strings.ToUpper(strings.ReplaceAll(s.AssignmentKey, "-", "_")) +
					"=" + s.AssignmentValue + " ./deploy.sh --env prod\n"
				return mkCase(s, "shell/inline-env-command", in)
			},
		},
		{
			Name:    "shell/curl-header-flag",
			Accepts: selfContainedSingleLine,
			Build: func(s secretShape) scrubCase {
				in := `curl -H "X-Api-Key: ` + s.Text + `" https://api.example.com/v1/things` + "\n"
				return mkCase(s, "shell/curl-header-flag", in)
			},
		},
		{
			Name:    "shell/history-prompt-line",
			Accepts: selfContainedSingleLine,
			Build: func(s secretShape) scrubCase {
				in := "$ echo " + s.Text + " | pbcopy\n"
				return mkCase(s, "shell/history-prompt-line", in)
			},
		},
		{
			Name:    "shell/heredoc",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "cat <<EOF > /etc/app.conf\n" + s.AssignmentKey + "=" + s.AssignmentValue + "\nEOF\n"
				return mkCase(s, "shell/heredoc", in)
			},
		},
		{
			Name:    "shell/pipeline-and-redirect",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "printf '%s' " + s.AssignmentKey + "=" + s.AssignmentValue + " | tee out.txt && echo done\n"
				return mkCase(s, "shell/pipeline-and-redirect", in)
			},
		},
	}
}

// documentContexts place the credential inside larger, messier documents:
// log lines, markdown, unterminated structures, and files whose last byte
// is the last byte of the secret.
func documentContexts() []scrubContext {
	return []scrubContext{
		{
			Name:    "doc/log-line",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := "2026-08-01T10:00:00Z INFO starting worker " +
					s.AssignmentKey + "=" + s.AssignmentValue + " pid=4211\n"
				return mkCase(s, "doc/log-line", in)
			},
		},
		{
			Name:    "doc/markdown-fenced-block",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				in := "Set it like this:\n\n```\n" + s.AssignmentKey + "=" + s.AssignmentValue + "\n```\n"
				return mkCase(s, "doc/markdown-fenced-block", in)
			},
		},
		{
			Name:    "doc/markdown-inline-code",
			Accepts: selfContainedSingleLine,
			Build: func(s secretShape) scrubCase {
				in := "Use the key `" + s.Text + "` for staging.\n"
				return mkCase(s, "doc/markdown-inline-code", in)
			},
		},
		{
			Name:    "doc/eof-no-newline",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				return mkCase(s, "doc/eof-no-newline", "leading context line\n"+s.Text)
			},
		},
		{
			Name:    "doc/crlf-everywhere",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				body := strings.ReplaceAll(s.Text, "\n", "\r\n")
				return mkCase(s, "doc/crlf-everywhere", "header line\r\n"+body+"\r\ntrailer line\r\n")
			},
		},
		{
			Name:    "doc/surrounded-by-fillers",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				in := strings.Join(benignFillers[:4], "\n") + "\n" + s.Text + "\n" +
					strings.Join(benignFillers[4:], "\n") + "\n"
				return mkCase(s, "doc/surrounded-by-fillers", in)
			},
		},
		{
			Name:    "doc/repeated-twice",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				return mkCase(s, "doc/repeated-twice", s.Text+"\n"+s.Text+"\n")
			},
		},
		{
			Name:    "doc/prose-then-secret-same-line",
			Accepts: selfContainedSingleLine,
			Build: func(s secretShape) scrubCase {
				return mkCase(s, "doc/prose-then-secret-same-line",
					"note the credential is "+s.Text+" do not share it\n")
			},
		},
		{
			// A PEM marker that is not at line start. Anchoring marker
			// detection to (trimmed) line start hid the marker completely
			// behind any prefix at all, and the whole key shipped in the
			// clear. Shell history and log lines routinely produce exactly
			// this.
			Name:    "doc/pem-marker-not-at-line-start",
			Accepts: func(s secretShape) bool { return s.Kind == "pem" },
			Build: func(s secretShape) scrubCase {
				return mkCase(s, "doc/pem-marker-not-at-line-start", "note "+s.Text+"\n")
			},
		},
		{
			Name:    "doc/pem-marker-jammed-against-word",
			Accepts: func(s secretShape) bool { return s.Kind == "pem" },
			Build: func(s secretShape) scrubCase {
				return mkCase(s, "doc/pem-marker-jammed-against-word", "task"+s.Text+"\n")
			},
		},
		{
			Name:    "doc/pem-in-json-string",
			Accepts: func(s secretShape) bool { return s.Kind == "pem" },
			Build: func(s secretShape) scrubCase {
				b, err := json.Marshal(map[string]string{"private_key": s.Text})
				if err != nil {
					panic(err)
				}
				return mkCase(s, "doc/pem-in-json-string", string(b))
			},
		},
		{
			Name:    "doc/unterminated-quote-then-secret",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := `msg="unterminated and then ` + s.AssignmentKey + "=" + s.AssignmentValue + "\n"
				return mkCase(s, "doc/unterminated-quote-then-secret", in)
			},
		},
		{
			Name:    "doc/two-credentials-escaped-quote-between",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := `password="a\" ` + s.AssignmentKey + `="` + s.AssignmentValue + `"`
				return mkCase(s, "doc/two-credentials-escaped-quote-between", in)
			},
		},
		{
			// A bare (unquoted) credential key found inside an already-open
			// JSON string, whose value opens with an ESCAPED quote rather
			// than a bare byte: {"a":"KEY=\"VALUE\""}. Unlike
			// doc/two-credentials-escaped-quote-between (which fences the KEY
			// itself in escaped quotes, the shape escapedQuotedAssignmentSpan
			// covers) or the doubly-escaped JSON contexts above (which escape
			// BOTH key and value), this is the realistic JSONL-transcript
			// shape of a shell command embedding a double-quoted secret --
			// `run: export PASSWORD="SECRET"` -- serialized once into a JSON
			// string field. No context before this one reached a bare key
			// immediately followed by an escape-quoted value with the '='
			// operator.
			Name:    "doc/bare-key-escaped-quoted-value-eq",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := `{"a":"` + s.AssignmentKey + `=\"` + s.AssignmentValue + `\""}`
				return mkCase(s, "doc/bare-key-escaped-quoted-value-eq", in)
			},
		},
		{
			// Same shape as doc/bare-key-escaped-quoted-value-eq, with the
			// ':' operator instead of '=' -- the JSON/YAML-style spelling of
			// the same bare-key, escape-quoted-value assignment.
			Name:    "doc/bare-key-escaped-quoted-value-colon",
			Accepts: singleLineOnly,
			Build: func(s secretShape) scrubCase {
				in := `{"a":"` + s.AssignmentKey + `:\"` + s.AssignmentValue + `\""}`
				return mkCase(s, "doc/bare-key-escaped-quoted-value-colon", in)
			},
		},
		{
			// A value long enough to matter but well inside every scan
			// ceiling, padded before the credential so the credential sits
			// deep in the input rather than at offset 0.
			Name:    "doc/deep-offset-padding",
			Accepts: anyShape,
			Build: func(s secretShape) scrubCase {
				pad := strings.Repeat("filler line of ordinary text\n", 40)
				return mkCase(s, "doc/deep-offset-padding", pad+s.Text+"\n"+pad)
			},
		},
	}
}

// allContexts is the full context axis.
func allContexts() []scrubContext {
	var out []scrubContext
	out = append(out, adjacencyContexts()...)
	out = append(out, assignmentContexts()...)
	out = append(out, jsonContexts()...)
	out = append(out, yamlContexts()...)
	out = append(out, envAndShellContexts()...)
	out = append(out, documentContexts()...)
	return out
}

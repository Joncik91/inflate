package harvester

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	maxEvents     = 200
	maxUserMsgs   = 3
	maxAsstMsgs   = 3
	maxJSONLBytes = 4000 // approximate token cap
)

type jsonlEvent struct {
	Type    string `json:"type"`
	Message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	Timestamp string `json:"timestamp"`
}

var (
	filePathRE   = regexp.MustCompile(`[\w./\-]+\.(go|rs|py|ts|tsx|js|jsx|toml|yaml|yml|md|json|sh|sql|c|h|cpp|java|kt|swift)\b(:\d+)?`)
	stackTraceRE = regexp.MustCompile(`(?m)^[^a-z\n]*\b(panicked|Traceback|Error|Exception|panic:|fatal)\b.*$`)
)

// CollectJSONL tails the most-recently-modified *.jsonl file in dir and
// returns a compact context block. ok=false means no JSONL files exist.
func CollectJSONL(dir string) (string, bool) {
	path, ok := newestJSONL(dir)
	if !ok {
		return "", false
	}
	events := readLastEvents(path, maxEvents)
	if len(events) == 0 {
		return "", false
	}

	var sb strings.Builder
	files := map[string]struct{}{}
	traces := []string{}
	users := []string{}
	asst := []string{}

	for _, e := range events {
		text := contentToText(e.Message.Content)
		if text == "" {
			continue
		}
		for _, m := range filePathRE.FindAllString(text, -1) {
			files[m] = struct{}{}
		}
		for _, m := range stackTraceRE.FindAllString(text, -1) {
			traces = append(traces, m)
		}
		switch e.Message.Role {
		case "user":
			users = append(users, text)
		case "assistant":
			asst = append(asst, text)
		}
	}

	if len(files) > 0 {
		fmt.Fprintln(&sb, "files referenced:")
		for f := range files {
			fmt.Fprintf(&sb, "  %s\n", f)
		}
	}
	if len(traces) > 0 {
		fmt.Fprintln(&sb, "errors / stack traces:")
		for _, t := range traces {
			fmt.Fprintf(&sb, "  %s\n", t)
		}
	}
	if u := lastN(users, maxUserMsgs); len(u) > 0 {
		fmt.Fprintln(&sb, "recent user prompts:")
		for _, m := range u {
			fmt.Fprintf(&sb, "  - %s\n", truncate(m, 200))
		}
	}
	if a := lastN(asst, maxAsstMsgs); len(a) > 0 {
		fmt.Fprintln(&sb, "recent assistant replies:")
		for _, m := range a {
			fmt.Fprintf(&sb, "  - %s\n", truncate(m, 200))
		}
	}

	out := sb.String()
	if len(out) > maxJSONLBytes {
		out = out[:maxJSONLBytes] + "\n…(truncated)\n"
	}
	return out, true
}

func newestJSONL(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	type ent struct {
		path  string
		mtime int64
	}
	var jsonls []ent
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		jsonls = append(jsonls, ent{filepath.Join(dir, e.Name()), info.ModTime().UnixNano()})
	}
	if len(jsonls) == 0 {
		return "", false
	}
	sort.Slice(jsonls, func(i, j int) bool { return jsonls[i].mtime > jsonls[j].mtime })
	return jsonls[0].path, true
}

func readLastEvents(path string, n int) []jsonlEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	ring := make([]jsonlEvent, 0, n)
	for scanner.Scan() {
		var e jsonlEvent
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // tolerate truncated last line / malformed
		}
		if len(ring) < n {
			ring = append(ring, e)
		} else {
			copy(ring, ring[1:])
			ring[n-1] = e
		}
	}
	return ring
}

func contentToText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Content can be either a plain string or an array of content blocks.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Type == "text" {
				sb.WriteString(b.Text)
				sb.WriteString("\n")
			}
		}
		return sb.String()
	}
	return ""
}

func lastN(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

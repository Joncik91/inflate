package harvester

import (
	"strings"
	"testing"
)

func TestDiagnoseJSONLNoFile(t *testing.T) {
	dir := t.TempDir()
	_, ok, err := DiagnoseJSONL(dir)
	if ok {
		t.Errorf("expected ok=false on empty dir")
	}
	if err == nil {
		t.Errorf("expected non-nil err when no jsonl files")
	}
}

func TestCollectJSONLNoFile(t *testing.T) {
	dir := t.TempDir()
	got, ok := CollectJSONL(dir)
	if ok {
		t.Errorf("expected ok=false on empty dir, got %q", got)
	}
}

func TestCollectJSONLFromFixture(t *testing.T) {
	got, ok := CollectJSONL("testdata")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(got, "src/foo.rs:47") {
		t.Errorf("missing file ref: %q", got)
	}
	if !strings.Contains(got, "edit src/main.go please") {
		t.Errorf("missing assistant reply content: %q", got)
	}
	if strings.Contains(got, "recent user prompts") {
		t.Errorf("user-prompts section should be dropped (issue #4 B): %q", got)
	}
	if strings.Contains(got, "now run the tests") {
		t.Errorf("user-prompt content should not appear in summary: %q", got)
	}
}

func TestCollectJSONLMalformedLineSkipped(t *testing.T) {
	dir := t.TempDir()
	// Mix in an assistant message so the surviving summary has something
	// to assert on (we no longer keep user prompts).
	body := `{"type":"user","message":{"role":"user","content":"first"},"timestamp":"2026-04-27T10:00:00Z"}` + "\n" +
		`{this is not json` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":"second-reply"},"timestamp":"2026-04-27T10:00:01Z"}` + "\n"
	if err := writeFile(t, dir, "x.jsonl", body); err != nil {
		t.Fatal(err)
	}
	got, ok := CollectJSONL(dir)
	if !ok {
		t.Fatal("expected ok=true despite malformed line")
	}
	if !strings.Contains(got, "second-reply") {
		t.Errorf("missing 'second-reply' assistant msg: %q", got)
	}
}

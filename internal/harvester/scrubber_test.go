package harvester

import "testing"

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
			"[REDACTED]", 1},
		{"token=", "token=abc123",
			"[REDACTED]", 1},
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

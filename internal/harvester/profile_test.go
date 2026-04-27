package harvester

import (
	"strings"
	"testing"

	"github.com/Joncik91/inflate/internal/config"
)

func TestCollectProfile(t *testing.T) {
	p := config.Profile{
		Identity: "senior backend engineer (Go)",
		Work:     "CLI tools",
		Style:    "terse",
	}
	got := CollectProfile(p)
	if !strings.Contains(got, "senior backend engineer (Go)") {
		t.Errorf("missing identity: %q", got)
	}
	if !strings.Contains(got, "terse") {
		t.Errorf("missing style: %q", got)
	}
}

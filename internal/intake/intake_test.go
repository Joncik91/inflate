package intake

import (
	"strings"
	"testing"

	"github.com/Joncik91/inflate/internal/config"
)

func TestRunFromReader(t *testing.T) {
	in := strings.NewReader("senior backend engineer\nAPI services\nterse\n")
	var out strings.Builder
	p, err := RunFromReader(in, &out)
	if err != nil {
		t.Fatal(err)
	}
	if p.Identity != "senior backend engineer" {
		t.Errorf("identity = %q", p.Identity)
	}
	if p.Style != "terse" {
		t.Errorf("style = %q", p.Style)
	}
}

func TestRunFromReaderNormalisesStyle(t *testing.T) {
	in := strings.NewReader("dev\nstuff\nGRANDMASTER\n")
	var out strings.Builder
	p, _ := RunFromReader(in, &out)
	if p.Style != "verbose" {
		t.Errorf("expected GRANDMASTER -> verbose, got %q", p.Style)
	}
	_ = config.Profile{} // import sanity
}

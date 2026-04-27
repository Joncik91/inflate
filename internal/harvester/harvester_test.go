package harvester

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Joncik91/inflate/internal/config"
)

func TestHarvesterPublishesInitialBundle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	h, err := New(Options{
		ProjectDir: dir,
		Profile:    config.Profile{Identity: "tester", Style: "terse"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Run(ctx)

	select {
	case b := <-h.Bundles():
		if !b.ProfileOK {
			t.Errorf("expected ProfileOK=true; got %s", b.FlagsString())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no bundle published within 2s")
	}
}

func TestHarvesterCachesAndForceRefreshes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	h, err := New(Options{
		ProjectDir: dir,
		Profile:    config.Profile{Identity: "tester"},
	})
	if err != nil {
		t.Fatal(err)
	}
	b1 := h.Latest()
	if b1.ProfileOK {
		t.Errorf("expected empty cached bundle before Run, got %s", b1.FlagsString())
	}
	h.collectOnce() // direct trigger
	b2 := h.Latest()
	if !b2.ProfileOK {
		t.Errorf("expected ProfileOK after collectOnce, got %s", b2.FlagsString())
	}
}

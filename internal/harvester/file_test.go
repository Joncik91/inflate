package harvester

import "testing"

func TestCollectFileNoLSOF(t *testing.T) {
	t.Setenv("PATH", "") // lsof not findable
	got, ok := CollectFile("/tmp")
	if ok {
		t.Errorf("expected ok=false when lsof missing, got %q", got)
	}
}

func TestEditorList(t *testing.T) {
	if len(editors) == 0 {
		t.Error("editors list is empty")
	}
}

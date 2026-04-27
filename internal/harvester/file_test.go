package harvester

import "testing"

func TestDiagnoseFileNoLSOF(t *testing.T) {
	t.Setenv("PATH", "") // lsof not findable
	_, ok, err := DiagnoseFile("/tmp")
	if ok {
		t.Errorf("expected ok=false when lsof missing")
	}
	if err == nil {
		t.Errorf("expected non-nil err when lsof missing")
	}
}

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

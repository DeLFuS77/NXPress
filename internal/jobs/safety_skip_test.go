package jobs

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// A new file already there without -w skips its file, as nsz does: the work run again over
// the same files ends well, and the file there stays.
func TestNewFileThereIsSkipped(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	nsp := testkit.TitleNsp(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	o := Options{Compress: true, Level: 3, Files: []string{nsp}, Multi: 1, Threads: 1}
	if failed := RunJobs(context.Background(), o, testkit.Quiet{}); failed != 0 {
		t.Fatalf("first run: %d failed", failed)
	}
	var text bytes.Buffer
	if failed := RunJobs(context.Background(), o, &Reporter{Out: &text}); failed != 0 {
		t.Fatalf("second run: %d failed: %s", failed, text.String())
	}
	if text.String() != "SKIPPED game.nsp (game.nsz is already there (use -w to replace it))\n" {
		t.Fatalf("%q", text.String())
	}

	packed := filepath.Join(folder, "pack.nsp")
	other := testkit.TitleNsp(t, filepath.Join(folder, "dlc.nsp"), testkit.DlcID, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
	pack := Options{Pack: packed, Files: []string{nsp, other}}
	for round := 0; round < 2; round++ {
		if failed := RunJobs(context.Background(), pack, testkit.Quiet{}); failed != 0 {
			t.Fatalf("pack, round %d: %d failed", round, failed)
		}
	}
}

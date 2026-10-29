package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// A new file already there without -w skips its file, as nsz does: the work run again over
// the same files ends well, and the file there stays.
func TestNewFileThereIsSkipped(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	nsp := safetyTitleNsp(t, filepath.Join(folder, "game.nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	o := options{compress: true, level: 3, files: []string{nsp}, multi: 1, threads: 1}
	if failed := runJobs(context.Background(), o, quiet{}); failed != 0 {
		t.Fatalf("first run: %d failed", failed)
	}
	var text bytes.Buffer
	if failed := runJobs(context.Background(), o, &reporter{out: &text}); failed != 0 {
		t.Fatalf("second run: %d failed: %s", failed, text.String())
	}
	if text.String() != "SKIPPED game.nsp (game.nsz is already there (use -w to replace it))\n" {
		t.Fatalf("%q", text.String())
	}

	packed := filepath.Join(folder, "pack.nsp")
	other := safetyTitleNsp(t, filepath.Join(folder, "dlc.nsp"), safetyDlcID, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
	pack := options{pack: packed, files: []string{nsp, other}}
	for round := 0; round < 2; round++ {
		if failed := runJobs(context.Background(), pack, quiet{}); failed != 0 {
			t.Fatalf("pack, round %d: %d failed", round, failed)
		}
	}
}

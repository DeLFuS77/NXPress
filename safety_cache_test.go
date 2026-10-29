package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// A file made whole from its parts is only compared with them: it is not kept as checked, and
// -V reads it. A file compressed, checked as a game, is kept.
func TestOnlyFilesCheckedAsGamesAreKeptAsChecked(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	parts := filepath.Join(folder, "Broken.nsp")
	os.Mkdir(parts, 0o755)
	os.WriteFile(filepath.Join(parts, "00"), []byte("not a game, only some bytes"), 0o644)
	os.WriteFile(filepath.Join(parts, "01"), []byte(" split in two parts"), 0o644)
	if failed := runJobs(context.Background(), options{fat32: true, files: []string{parts}, multi: 1, threads: 1}, quiet{}); failed != 0 {
		t.Fatalf("join: %d failed", failed)
	}
	if _, note, err := doJob(context.Background(), "verify", parts, options{}, 1, quiet{}); err == nil {
		t.Fatalf("-V of bytes joined from parts: %q", note)
	}

	nsp := safetyTitleNsp(t, filepath.Join(folder, "game.nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	if failed := runJobs(context.Background(), options{compress: true, level: 3, files: []string{nsp}, multi: 1, threads: 1}, quiet{}); failed != 0 {
		t.Fatalf("compress: %d failed", failed)
	}
	if _, note, err := doJob(context.Background(), "verify", filepath.Join(folder, "game.nsz"), options{}, 1, quiet{}); err != nil || !strings.HasPrefix(note, "checked on ") {
		t.Fatalf("-V of the compressed file: %q %v", note, err)
	}
}

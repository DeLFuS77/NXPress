package jobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// A file made whole from its parts is only compared with them: it is not kept as checked, and
// -V reads it. A file compressed, checked as a game, is kept.
func TestOnlyFilesCheckedAsGamesAreKeptAsChecked(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	parts := filepath.Join(folder, "Broken.nsp")
	os.Mkdir(parts, 0o755)
	os.WriteFile(filepath.Join(parts, "00"), []byte("not a game, only some bytes"), 0o644)
	os.WriteFile(filepath.Join(parts, "01"), []byte(" split in two parts"), 0o644)
	if failed := RunJobs(context.Background(), Options{Fat32: true, Files: []string{parts}, Multi: 1, Threads: 1}, testkit.Quiet{}); failed != 0 {
		t.Fatalf("join: %d failed", failed)
	}
	if _, note, err := doJob(context.Background(), "verify", parts, Options{}, 1, testkit.Quiet{}); err == nil {
		t.Fatalf("-V of bytes joined from parts: %q", note)
	}

	nsp := testkit.TitleNsp(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	if failed := RunJobs(context.Background(), Options{Compress: true, Level: 3, Files: []string{nsp}, Multi: 1, Threads: 1}, testkit.Quiet{}); failed != 0 {
		t.Fatalf("compress: %d failed", failed)
	}
	if _, note, err := doJob(context.Background(), "verify", filepath.Join(folder, "game.nsz"), Options{}, 1, testkit.Quiet{}); err != nil || !strings.HasPrefix(note, "checked on ") {
		t.Fatalf("-V of the compressed file: %q %v", note, err)
	}
}

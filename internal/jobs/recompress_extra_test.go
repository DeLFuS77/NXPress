package jobs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// Recompress with the extra compression goes through an NSP checked by its hashes (#92): the
// file is written again in place, its hidden copies are gone, and decompressed it is the game
// it was, byte for byte. A game that gains nothing from the extra compression stays a plain NSZ.
func TestRecompressWithExtraGoesThroughAnNsp(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	nsp := testkit.TitleNsp(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	original, err := os.ReadFile(nsp)
	if err != nil {
		t.Fatal(err)
	}
	if failed := RunJobs(context.Background(), Options{Compress: true, Level: 3, Files: []string{nsp}, Multi: 1, Threads: 1}, testkit.Quiet{}); failed != 0 {
		t.Fatalf("compress: %d failed", failed)
	}
	nsz := filepath.Join(folder, "game.nsz")
	os.Remove(nsp)

	results := &noted{}
	if failed := RunJobs(context.Background(), Options{Recompress: true, Sparse: true, Archive: true, Level: 5, Files: []string{nsz}, Multi: 1, Threads: 1}, results); failed != 0 {
		t.Fatalf("recompress with --extra: %d failed: %v", failed, results.err)
	}
	if !strings.Contains(results.note, "a plain NSZ again") {
		t.Errorf("note: %q", results.note)
	}
	if left := testkit.List(folder); len(left) != 1 || left[0] != "game.nsz" {
		t.Errorf("files left: %v", left)
	}

	back := filepath.Join(t.TempDir(), "game.nsp")
	if _, err := switchfs.DecompressAndCheck(context.Background(), nsz, back, nil); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(back); !bytes.Equal(again, original) {
		t.Error("decompressed, it is not the game it was")
	}
}

// Recompress asks for the keys only with the extra compression: the file then goes through an
// NSP (#92).
func TestRecompressNeedsKeysOnlyWithExtra(t *testing.T) {
	for _, o := range []struct {
		options Options
		want    bool
	}{
		{Options{Recompress: true}, false},
		{Options{Recompress: true, Sparse: true, Archive: true}, true},
		{Options{Recompress: true, Archive: true}, true},
		{Options{Fat32: true, Sparse: true}, false},
	} {
		if got := NeedsKeys(o.options); got != o.want {
			t.Errorf("%+v: %v", o.options, got)
		}
	}
}

// noted keeps the note and the error of the last file of a work.
type noted struct {
	testkit.Quiet
	mutex sync.Mutex
	note  string
	err   error
}

func (n *noted) Done(path string, output string, note string, err error) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.note, n.err = note, err
}

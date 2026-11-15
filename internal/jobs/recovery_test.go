package jobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/par2"
	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// With --par2 the new file gets its recovery data; the original deleted takes its own along;
// Verify with --repair rebuilds the file damaged on the disk, and without it tells it damaged; a
// file renamed takes its .par2 along.
func TestRecoveryData(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	nsp := testkit.TitleNsp(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	if err := par2.Create(context.Background(), nsp, par2.Path(nsp), 5, nil); err != nil {
		t.Fatal(err)
	}
	out := &noteCatcher{notes: map[string]string{}}
	if failed := RunJobs(context.Background(), Options{Compress: true, Level: 3, Par2: 10, RmSource: true, Files: []string{nsp}, Multi: 1, Threads: 1}, out); failed != 0 {
		t.Fatalf("compress: %d failed: %v", failed, out.notes)
	}
	nsz := filepath.Join(folder, "game.nsz")
	if left := testkit.List(folder); strings.Join(left, ",") != "game.nsz,game.nsz.par2" {
		t.Fatalf("left: %v", left)
	}
	if !strings.Contains(out.notes[nsp], "recovery data") {
		t.Fatalf("note %q", out.notes[nsp])
	}

	data, _ := os.ReadFile(nsz)
	original := append([]byte(nil), data...)
	data[len(data)/2] ^= 0xFF
	os.WriteFile(nsz, data, 0o644)
	if _, _, err := doJob(context.Background(), "verify", nsz, Options{NoCache: true}, 1, testkit.Quiet{}); err == nil {
		t.Fatal("a damaged file passes")
	}
	_, note, err := doJob(context.Background(), "verify", nsz, Options{NoCache: true, Repair: true}, 1, testkit.Quiet{})
	if err != nil || !strings.Contains(note, "repaired with its recovery data") {
		t.Fatalf("repair: %q %v", note, err)
	}
	if again, _ := os.ReadFile(nsz); string(again) != string(original) {
		t.Fatal("not rebuilt as it was")
	}

	renamed := filepath.Join(folder, "Game [0100000000010000][v0].nsz")
	os.Rename(nsz, renamed)
	moveRecovery(nsz, renamed)
	if left := testkit.List(folder); strings.Join(left, ",") != "Game [0100000000010000][v0].nsz,Game [0100000000010000][v0].nsz.par2" {
		t.Fatalf("after renaming: %v", left)
	}
}

// A file written again in place keeps recovery data that fits it: its .par2 is made again, as
// big as it was, also without --par2.
func TestRecoveryDataMadeAgainForAFileWrittenAgain(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	file := filepath.Join(folder, "game.nsz")
	os.WriteFile(file, []byte(strings.Repeat("old game ", 100000)), 0o644)
	if err := par2.Create(context.Background(), file, par2.Path(file), 10, nil); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(file, []byte(strings.Repeat("new game ", 120000)), 0o644)
	if note := recoveryAfter(context.Background(), Options{}, "recompress", file, testkit.Quiet{}); note == "" {
		t.Fatal("no recovery data made again")
	}
	result, err := par2.Verify(context.Background(), par2.Path(file), file, nil)
	if err != nil || !result.OK() || result.Recovery*10 < result.Slices {
		t.Fatalf("%+v %v", result, err)
	}
}

// noteCatcher keeps the note each file ended with
type noteCatcher struct {
	testkit.Quiet
	notes map[string]string
}

func (n *noteCatcher) Done(path string, output string, note string, err error) {
	if err != nil {
		note = "error: " + err.Error()
	}
	n.notes[path] = note
}

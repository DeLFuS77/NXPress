package jobs

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// sparseGame writes a game in sparse storage, its update and its DLC in folder.
func sparseGame(t *testing.T, folder string) (string, string, string) {
	t.Helper()
	program := testkit.ProgramNcaWith(testkit.GameData(0x10000, 5), func(header []byte) {
		// the table of a sparse section has a size
		header[0x400+0x150] = 0x10
		fsHash := sha256.Sum256(header[0x400:0x600])
		copy(header[0x280:], fsHash[:])
	})
	base := testkit.NspWith(t, filepath.Join(folder, "base.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, program, testkit.ContentName(program, ".nca"), nil)
	update := testkit.TitleNsp(t, filepath.Join(folder, "update.nsp"), testkit.UpdateID, 65536, switchfs.ContentMetaType_Patch, 2, nil)
	dlc := testkit.TitleNsp(t, filepath.Join(folder, "dlc.nsp"), testkit.DlcID, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
	return base, update, dlc
}

// With --skip-sparse-updates, the update of a game in sparse storage is left as it is with it,
// also when it comes first in the list; its DLC is compressed. Without it the update is compressed.
func TestSparseUpdateIsLeftWithItsGame(t *testing.T) {
	testkit.Isolate(t)
	for _, skip := range []bool{true, false} {
		folder := t.TempDir()
		base, update, dlc := sparseGame(t, folder)
		out := &testkit.Results{}
		o := Options{Compress: true, Level: 3, Threads: 1, Multi: 1, Files: []string{update, dlc, base}, SkipSparseUpdates: skip}
		if failed := RunJobs(context.Background(), o, out); failed != 0 {
			t.Fatalf("skip %v: %d failed: %v", skip, failed, out.Results)
		}
		if err := out.Results[base].Err; err == nil || err.Error() != sparseSkipped {
			t.Errorf("skip %v: the game: %v", skip, err)
		}
		if err := out.Results[dlc].Err; err != nil {
			t.Errorf("skip %v: the DLC: %v", skip, err)
		}
		err := out.Results[update].Err
		if skip && (err == nil || err.Error() != sparseUpdateSkipped) {
			t.Errorf("the update was not left with its game: %v", err)
		}
		if !skip && err != nil {
			t.Errorf("the update was not compressed without the option: %v", err)
		}
		if got := testkit.Exists(filepath.Join(folder, "update.nsz")); got == skip {
			t.Errorf("skip %v: update.nsz there: %v", skip, got)
		}
	}
}

// The option has nothing to do with the extra compression, which compresses the game too.
func TestSparsePairsNeedTheGameLeftAsItIs(t *testing.T) {
	testkit.Isolate(t)
	base, update, _ := sparseGame(t, t.TempDir())
	files := []string{base, update}
	if pairs := sparsePairs(context.Background(), Options{SkipSparseUpdates: true, Sparse: true}, "compress", files); len(pairs) != 0 {
		t.Errorf("with the extra compression: %v", pairs)
	}
	if pairs := sparsePairs(context.Background(), Options{SkipSparseUpdates: true}, "decompress", files); len(pairs) != 0 {
		t.Errorf("in another action: %v", pairs)
	}
	if pairs := sparsePairs(context.Background(), Options{SkipSparseUpdates: true}, "compress", []string{update}); len(pairs) != 0 {
		t.Errorf("without its game: %v", pairs)
	}
	if pairs := sparsePairs(context.Background(), Options{SkipSparseUpdates: true}, "compress", files); len(pairs) != 1 || pairs[update] == "" {
		t.Errorf("with its game: %v", pairs)
	}
}

// A file still waiting that is skipped by hand is left out when its turn comes; the rest is done.
func TestWaitingFileSkippedByHand(t *testing.T) {
	testkit.Isolate(t)
	base, update, dlc := testkit.Game(t, t.TempDir())
	skips := NewSkips()
	skips.Skip(update)
	out := &testkit.Results{}
	o := Options{Compress: true, Level: 3, Threads: 1, Multi: 1, Files: []string{base, update, dlc}}
	if failed := RunJobs(WithSkips(context.Background(), skips), o, out); failed != 0 {
		t.Fatalf("%d failed: %v", failed, out.Results)
	}
	if err := out.Results[update].Err; err == nil || err.Error() != SkippedByHand {
		t.Errorf("the update: %v", err)
	}
	for _, path := range []string{base, dlc} {
		if err := out.Results[path].Err; err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	if testkit.Exists(strings.TrimSuffix(update, ".nsp") + ".nsz") {
		t.Error("the update skipped was compressed")
	}
}

// skipAtWork skips each file as soon as it tells its progress.
type skipAtWork struct {
	testkit.Results
	skips *Skips
	once  sync.Map
}

func (s *skipAtWork) Progress(path string, step string) func(int64, int64) {
	return func(int64, int64) {
		if _, done := s.once.LoadOrStore(path, true); !done {
			s.skips.Skip(path)
		}
	}
}

// A file skipped by hand while it is at work stops, its half written file is removed, it is
// told as skipped and not failed, and the next file is done.
func TestFileAtWorkSkippedByHand(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base, update, _ := testkit.Game(t, folder)
	skips := NewSkips()
	out := &skipAtWork{skips: skips}
	o := Options{Compress: true, Level: 3, Threads: 1, Multi: 1, Files: []string{base}}
	if failed := RunJobs(WithSkips(context.Background(), skips), o, out); failed != 0 {
		t.Fatalf("%d failed: %v", failed, out.Results.Results)
	}
	if err := out.Results.Results[base].Err; err == nil || err.Error() != SkippedByHand {
		t.Errorf("the game: %v", err)
	}
	for _, name := range testkit.List(folder) {
		if strings.HasSuffix(name, ".nsz") || strings.Contains(name, ".tmp") || strings.Contains(name, "part") {
			t.Errorf("left behind: %s", name)
		}
	}
	// the next work, without skips, goes on as usual
	again := &testkit.Results{}
	o.Files = []string{update}
	if failed := RunJobs(WithSkips(context.Background(), NewSkips()), o, again); failed != 0 {
		t.Fatalf("the update: %v", again.Results)
	}
}

// In Recompress what Compress leaves as it is is left too (#132): a game in sparse storage kept in
// the extra compression stays as it is without it, its update stays with it with
// --skip-sparse-updates, the DLC stay as they are with --skip-dlc, and with --move-skipped the
// three go to its folder as their turn comes. Nothing of them is written again.
func TestRecompressLeavesWhatCompressLeaves(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base, update, dlc := testkit.Game(t, folder)
	if failed := RunJobs(context.Background(), Options{Compress: true, Level: 3, Threads: 1, Multi: 1, Files: []string{base, update, dlc}}, testkit.Quiet{}); failed != 0 {
		t.Fatalf("compress: %d failed", failed)
	}
	nsz := func(path string) string {
		os.Remove(path)
		return strings.TrimSuffix(path, ".nsp") + ".nsz"
	}
	base, update, dlc = nsz(base), nsz(update), nsz(dlc)
	// the game, as if it were in sparse storage kept in NCZSECT2
	real := describeFile
	describeFile = func(path string) (switchfs.FileFacts, error) {
		facts, err := real(path)
		if SameFile(path, base) {
			facts.Sparse = 1
		}
		return facts, err
	}
	t.Cleanup(func() { describeFile = real })

	if pairs := sparsePairs(context.Background(), Options{SkipSparseUpdates: true, Sparse: true}, "recompress", []string{update, base}); len(pairs) != 0 {
		t.Errorf("with the extra compression: %v", pairs)
	}
	aside := filepath.Join(t.TempDir(), "aside")
	out := &testkit.Results{}
	o := Options{Recompress: true, Level: 5, Threads: 1, Multi: 1, Files: []string{update, dlc, base}, SkipSparseUpdates: true, SkipDlc: true, MoveSkipped: aside}
	if failed := RunJobs(context.Background(), o, out); failed != 0 {
		t.Fatalf("recompress: %d failed: %v", failed, out.Results)
	}
	for path, reason := range map[string]string{base: sparseNoPlain, update: sparseUpdateSkipped, dlc: dlcSkipped} {
		err := out.Results[path].Err
		if err == nil || !IsSkipped(err) || !strings.HasPrefix(err.Error(), reason) {
			t.Errorf("%s: %v", filepath.Base(path), err)
		}
		if testkit.Exists(path) || !testkit.Exists(filepath.Join(aside, filepath.Base(path))) {
			t.Errorf("%s was not moved to its folder", filepath.Base(path))
		}
	}
}

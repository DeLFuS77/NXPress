package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/par2"
)

// With --skip-dlc the DLC are left as they are and the rest is compressed; without it they are
// compressed too.
func TestSkipDlcLeavesTheDlc(t *testing.T) {
	testkit.Isolate(t)
	for _, skip := range []bool{true, false} {
		folder := t.TempDir()
		base, update, dlc := testkit.Game(t, folder)
		out := &testkit.Results{}
		o := Options{Compress: true, Level: 3, Threads: 1, Multi: 1, Files: []string{base, update, dlc}, SkipDlc: skip}
		if failed := RunJobs(context.Background(), o, out); failed != 0 {
			t.Fatalf("skip %v: %d failed: %v", skip, failed, out.Results)
		}
		err := out.Results[dlc].Err
		if skip && (err == nil || err.Error() != dlcSkipped) {
			t.Errorf("the DLC was not left as it is: %v", err)
		}
		if !skip && err != nil {
			t.Errorf("the DLC was not compressed without the option: %v", err)
		}
		if got := testkit.Exists(filepath.Join(folder, "dlc.nsz")); got == skip {
			t.Errorf("skip %v: dlc.nsz there: %v", skip, got)
		}
		for _, path := range []string{base, update} {
			if err := out.Results[path].Err; err != nil {
				t.Errorf("skip %v: %s: %v", skip, path, err)
			}
		}
	}
}

// movedAtDone keeps, as each file is told done, whether it was in its new place already, in order.
type movedAtDone struct {
	testkit.Results
	folder string
	order  []string
}

func (m *movedAtDone) Done(path string, output string, note string, err error) {
	m.Results.Done(path, output, note, err)
	there := testkit.Exists(filepath.Join(m.folder, filepath.Base(path)))
	m.order = append(m.order, fmt.Sprintf("%s:%v:%v", filepath.Base(path), there, output == filepath.Join(m.folder, filepath.Base(path))))
}

// With --move-skipped what is left as it is goes to its folder as its turn comes in the work, one
// file after the other (asked by ItsmeAJ on GBAtemp): the game in sparse storage, its update with
// --skip-sparse-updates and the DLC with --skip-dlc, each with its name and its recovery data,
// told as skipped with where it went once it is there. Nothing of it is compressed.
func TestSkippedFilesAreMovedAsTheirTurnComes(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base, update, dlc := sparseGame(t, folder)
	sidecar := par2.Path(dlc)
	if err := os.WriteFile(sidecar, []byte("recovery"), 0o644); err != nil {
		t.Fatal(err)
	}
	aside := filepath.Join(t.TempDir(), "left as they are")
	out := &movedAtDone{folder: aside}
	o := Options{Compress: true, Level: 3, Threads: 1, Multi: 1, Files: []string{update, dlc, base}, SkipSparseUpdates: true, SkipDlc: true, MoveSkipped: aside}
	if failed := RunJobs(context.Background(), o, out); failed != 0 {
		t.Fatalf("%d failed: %v", failed, out.Results.Results)
	}
	if got := strings.Join(out.order, " "); got != "update.nsp:true:true dlc.nsp:true:true base.nsp:true:true" {
		t.Errorf("as each was told done: %s", got)
	}
	for path, reason := range map[string]string{base: sparseSkipped, update: sparseUpdateSkipped, dlc: dlcSkipped} {
		err := out.Results.Results[path].Err
		if !IsSkipped(err) || err.Error() != reason+"; moved to "+aside {
			t.Errorf("%s: %v", filepath.Base(path), err)
		}
		if testkit.Exists(path) || !testkit.Exists(filepath.Join(aside, filepath.Base(path))) {
			t.Errorf("%s was not moved", filepath.Base(path))
		}
	}
	if testkit.Exists(sidecar) || !testkit.Exists(par2.Path(filepath.Join(aside, "dlc.nsp"))) {
		t.Error("the recovery data did not go with its file")
	}
	for _, name := range append(testkit.List(folder), testkit.List(aside)...) {
		if strings.HasSuffix(name, ".nsz") || strings.Contains(name, ".nxpress") {
			t.Errorf("left behind: %s", name)
		}
	}
}

// Without --skip-dlc and --skip-sparse-updates only the game in sparse storage is moved: its
// update and its DLC are compressed where they are. With the extra compression the game is
// compressed too, and nothing is moved.
func TestOnlyWhatIsLeftIsMoved(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base, update, dlc := sparseGame(t, folder)
	aside := t.TempDir()
	out := &testkit.Results{}
	o := Options{Compress: true, Level: 3, Threads: 1, Multi: 1, Files: []string{base, update, dlc}, MoveSkipped: aside}
	if failed := RunJobs(context.Background(), o, out); failed != 0 {
		t.Fatalf("%d failed: %v", failed, out.Results)
	}
	if !testkit.Exists(filepath.Join(aside, "base.nsp")) || testkit.Exists(base) {
		t.Error("the game in sparse storage was not moved")
	}
	for _, path := range []string{update, dlc} {
		if err := out.Results[path].Err; err != nil {
			t.Errorf("%s: %v", filepath.Base(path), err)
		}
		if !testkit.Exists(path) || !testkit.Exists(strings.TrimSuffix(path, ".nsp")+".nsz") {
			t.Errorf("%s was not compressed where it is", filepath.Base(path))
		}
	}
}

// A file of DLC only is a DLC; a game, a game with its DLC, and a file that cannot be read are not.
func TestDlcOnly(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base, update, dlc := testkit.Game(t, folder)
	if !dlcOnly(dlc) || dlcOnly(base) || dlcOnly(update) || dlcOnly(filepath.Join(folder, "missing.nsp")) {
		t.Error("a DLC is not told apart")
	}
	pack := filepath.Join(folder, "pack.nsp")
	o := Options{Pack: pack, Files: []string{base, dlc}}
	if failed := RunJobs(context.Background(), o, testkit.Quiet{}); failed != 0 || dlcOnly(pack) {
		t.Error("a game with its DLC is taken for a DLC")
	}
}

// A file never replaces one of its name in the folder: it stays where it is, told as skipped with
// why, and the file there is as it was. A file in the folder already stays there.
func TestMovingNeverReplacesAFile(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base, update, dlc := testkit.Game(t, folder)
	aside := t.TempDir()
	there := filepath.Join(aside, "dlc.nsp")
	if err := os.WriteFile(there, []byte("another file"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := &testkit.Results{}
	o := Options{Compress: true, Level: 3, Threads: 1, Multi: 1, Files: []string{base, update, dlc}, SkipDlc: true, MoveSkipped: aside}
	if failed := RunJobs(context.Background(), o, out); failed != 0 {
		t.Fatalf("%d failed: %v", failed, out.Results)
	}
	if err := out.Results[dlc].Err; !IsSkipped(err) || err.Error() != dlcSkipped+"; it stays where it is: "+ErrMovedThere.Error() {
		t.Errorf("the DLC: %v", err)
	}
	if data, _ := os.ReadFile(there); string(data) != "another file" || !testkit.Exists(dlc) {
		t.Error("a file was replaced")
	}
	if testkit.Exists(filepath.Join(folder, "dlc.nsz")) {
		t.Error("the DLC was compressed")
	}

	// in the folder already: left there
	own := t.TempDir()
	_, _, dlc = testkit.Game(t, own)
	out = &testkit.Results{}
	o = Options{Compress: true, Level: 3, Threads: 1, Multi: 1, Files: []string{dlc}, SkipDlc: true, MoveSkipped: own}
	if failed := RunJobs(context.Background(), o, out); failed != 0 {
		t.Fatalf("%d failed: %v", failed, out.Results)
	}
	if err := out.Results[dlc].Err; !IsSkipped(err) || err.Error() != dlcSkipped || !testkit.Exists(dlc) {
		t.Errorf("a DLC in the folder already: %v", err)
	}
}

// On another drive a file is copied under a hidden name, read back and compared, and given its
// name; the copy never replaces a file, and leaves nothing behind when it cannot be made.
func TestCopyAcrossDrives(t *testing.T) {
	from := filepath.Join(t.TempDir(), "game.nsp")
	data := bytes.Repeat([]byte("NXPress"), 1<<20)
	if err := os.WriteFile(from, data, 0o644); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	to := filepath.Join(folder, "game.nsp")
	last := int64(0)
	if err := copyAcross(context.Background(), from, to, func(done int64, total int64) {
		if total != 2*int64(len(data)) || done < last {
			t.Errorf("progress %d of %d", done, total)
		}
		last = done
	}); err != nil {
		t.Fatal(err)
	}
	if copied, _ := os.ReadFile(to); !bytes.Equal(copied, data) || last != 2*int64(len(data)) {
		t.Fatal("the copy is not the file")
	}
	if !testkit.Exists(from) {
		t.Fatal("the copy took the original away")
	}
	if err := copyAcross(context.Background(), from, to, nil); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("a copy over a file: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := copyAcross(ctx, from, filepath.Join(folder, "other.nsp"), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	if names := testkit.List(folder); len(names) != 1 {
		t.Fatalf("left behind: %v", names)
	}
}

// The options of the command line: --skip-dlc and --move-skipped, told when they have no effect.
func TestSkipOptions(t *testing.T) {
	o, err := ParseOptions([]string{"-C", "--skip-dlc", "--move-skipped", "aside", "game.nsp"})
	if err != nil || !o.SkipDlc || o.MoveSkipped != "aside" {
		t.Fatalf("%+v, %v", o, err)
	}
	if err := o.Check(); err != nil || len(o.Notes) != 0 {
		t.Fatalf("%v, notes %v", err, o.Notes)
	}
	o, _ = ParseOptions([]string{"-D", "--skip-dlc", "--move-skipped=aside", "game.nsz"})
	if err := o.Check(); err != nil || len(o.Notes) != 2 {
		t.Fatalf("%v, notes %v", err, o.Notes)
	}
	if _, err := ParseOptions([]string{"-C", "--move-skipped"}); err == nil {
		t.Fatal("--move-skipped without a folder")
	}
}

// A file moved to another drive is copied, and the original goes only once the copy has its name,
// with its recovery data; when the original cannot go, the copy goes and the file stays whole
// where it was.
func TestMovingToAnotherDrive(t *testing.T) {
	across := errors.New("another drive")
	renamed, other := renameNew, otherDrive
	renameNew = func(string, string) error { return across }
	otherDrive = func(err error) bool { return errors.Is(err, across) }
	defer func() { renameNew, otherDrive = renamed, other }()

	from := filepath.Join(t.TempDir(), "dlc.nsp")
	if err := os.WriteFile(from, []byte("a DLC"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(par2.Path(from), []byte("recovery"), 0o644); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	to, err := moveInto(context.Background(), from, folder, testkit.Quiet{})
	if err != nil || to != filepath.Join(folder, "dlc.nsp") {
		t.Fatalf("%s, %v", to, err)
	}
	if data, _ := os.ReadFile(to); string(data) != "a DLC" || testkit.Exists(from) {
		t.Fatal("the file was not moved")
	}
	if data, _ := os.ReadFile(par2.Path(to)); string(data) != "recovery" || testkit.Exists(par2.Path(from)) {
		t.Fatal("its recovery data did not go with it")
	}

	// the original cannot be deleted: its folder takes no change
	if runtime.GOOS == "windows" {
		return
	}
	locked := t.TempDir()
	from = filepath.Join(locked, "update.nsp")
	if err := os.WriteFile(from, []byte("an update"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chmod(locked, 0o500)
	defer os.Chmod(locked, 0o700)
	if os.Remove(from) == nil {
		t.Skip("the folder takes changes all the same")
	}
	if _, err := moveInto(context.Background(), from, folder, testkit.Quiet{}); err == nil || !strings.Contains(err.Error(), "the original could not be deleted") {
		t.Fatalf("%v", err)
	}
	if testkit.Exists(filepath.Join(folder, "update.nsp")) || !testkit.Exists(from) {
		t.Fatal("the file is in two places, or in none")
	}
}

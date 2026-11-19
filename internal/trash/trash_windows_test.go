package trash

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// binItems are the items of the Recycle Bin of this user on the drive of folder that came from
// path: their $I file (what Restore reads) and their $R file or folder (what was moved).
func binItems(t *testing.T, path string) [][2]string {
	t.Helper()
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(driveOf(path), "$Recycle.Bin", user.User.Sid.String())
	long := longName(path)
	names, _ := filepath.Glob(filepath.Join(folder, "$I*"))
	var found [][2]string
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil || len(data) < 28 {
			continue
		}
		var original string
		switch binary.LittleEndian.Uint64(data) {
		case 2:
			length := int(binary.LittleEndian.Uint32(data[24:]))
			if len(data) < 28+2*length {
				continue
			}
			original = decode(data[28 : 28+2*length])
		case 1:
			original = decode(data[24:])
		}
		if strings.EqualFold(original, path) || strings.EqualFold(original, long) {
			found = append(found, [2]string{name, filepath.Join(folder, "$R"+strings.TrimPrefix(filepath.Base(name), "$I"))})
		}
	}
	return found
}

// longName is path with the long names of its folders: the Recycle Bin keeps the long one, and the
// temporary folder of some computers is given by its short name (C:\Users\RUNNER~1 on GitHub).
func longName(path string) string {
	short, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return path
	}
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetLongPathName(short, &buffer[0], uint32(len(buffer)))
	if err != nil || n == 0 || int(n) > len(buffer) {
		return path
	}
	return filepath.Join(windows.UTF16ToString(buffer[:n]), filepath.Base(path))
}

func decode(data []byte) string {
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	for i, unit := range units {
		if unit == 0 {
			units = units[:i]
			break
		}
	}
	return string(utf16.Decode(units))
}

// inBin tells how many items of the Recycle Bin came from path, and takes them out of it once the
// test is over: the tests leave nothing in the Recycle Bin of the person who runs them.
func inBin(t *testing.T, path string) int {
	t.Helper()
	items := binItems(t, path)
	t.Cleanup(func() {
		for _, item := range binItems(t, path) {
			os.RemoveAll(item[1])
			os.Remove(item[0])
		}
	})
	return len(items)
}

func write(t *testing.T, path string, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s is still there: %v", path, err)
	}
}

// A file goes to the Recycle Bin, where Restore finds it by its path; one of the same name goes
// there too, beside it; and so does a name in other alphabets.
func TestMoveToTheRecycleBin(t *testing.T) {
	folder := t.TempDir()
	for _, name := range []string{"Game [0100000000010000][v0].nsp", "ゲーム ñ € 게임.nsz"} {
		path := filepath.Join(folder, name)
		write(t, path, "first")
		if err := Check(path); err != nil {
			t.Skipf("the drive of the temporary folder has no Recycle Bin here: %v", err)
		}
		if err := Move(path); err != nil {
			t.Fatal(err)
		}
		gone(t, path)
		write(t, path, "second")
		if err := Move(path); err != nil {
			t.Fatal(err)
		}
		gone(t, path)
		if n := inBin(t, path); n != 2 {
			t.Errorf("%s: %d items in the Recycle Bin, not 2", name, n)
		}
	}
}

// A folder goes whole, as one item.
func TestMoveAFolderToTheRecycleBin(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "Game dump")
	write(t, filepath.Join(folder, "base", "exefs", "main"), "code")
	write(t, filepath.Join(folder, "base", "romfs", "data.bin"), "data")
	if err := Check(folder); err != nil {
		t.Skipf("no Recycle Bin here: %v", err)
	}
	if err := Move(folder); err != nil {
		t.Fatal(err)
	}
	gone(t, folder)
	if n := inBin(t, folder); n != 1 {
		t.Errorf("%d items in the Recycle Bin, not 1", n)
	}
}

// A path longer than the shell takes stays where it is, told: the shell deletes it for good
// without a word, even by its short name. So does a folder with such a path inside, which the
// shell would ask about deleting for good.
func TestALongPathStays(t *testing.T) {
	folder := t.TempDir()
	if err := Check(folder); err != nil {
		t.Skipf("no Recycle Bin here: %v", err)
	}
	top := filepath.Join(folder, "Dump")
	deep := top
	for len(deep) < 300 {
		deep = filepath.Join(deep, strings.TrimSpace(strings.Repeat("folder ", 6)))
	}
	path := filepath.Join(deep, "Game [0100000000010000][v0].nsp")
	write(t, path, "long")
	for _, given := range []string{path, top} {
		unavailable, ok := Why(Move(given))
		if !ok || unavailable.Reason != TooLong {
			t.Fatalf("%s: %+v", given, unavailable)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("a path the Recycle Bin does not take is gone")
	}
	// a folder whose paths would be too long only in the Recycle Bin, under its longer name
	near := filepath.Join(folder, "D")
	drive := driveOf(folder)
	if length(drive)+length(`$Recycle.Bin\`)+length(userSid())+length(`\$RAB12CD`) <= length(near) {
		return
	}
	inside := near
	for length(inside)+11 <= maxPath {
		inside = filepath.Join(inside, "abcdefghij")
	}
	// its deepest path as long as the shell takes where it is
	if room := maxPath - length(inside) - 1; room > 0 {
		inside = filepath.Join(inside, strings.Repeat("a", room))
	}
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if !tooLong(near, drive) {
		t.Error("a folder too deep for the Recycle Bin is taken")
	}
	short := filepath.Join(folder, "E")
	write(t, filepath.Join(short, "base", "romfs", "data.bin"), "data")
	if tooLong(short, drive) {
		t.Error("a folder that fits in the Recycle Bin is not taken")
	}
}

// A network share has no Recycle Bin: told without reaching the network.
func TestANetworkShareHasNoRecycleBin(t *testing.T) {
	for _, path := range []string{`\\nxpress-nowhere\games\Game.nsp`, `\\?\UNC\nxpress-nowhere\games\Game.nsp`} {
		unavailable, ok := Why(check(path))
		if !ok || unavailable.Reason != NoBin || unavailable.Drive != `\\nxpress-nowhere\games` {
			t.Errorf("%s: %+v", path, unavailable)
		}
	}
}

// A file bigger than the Recycle Bin of its drive holds is told, and stays: with no question
// asked, the shell would delete it for good.
func TestAFileBiggerThanTheRecycleBinStays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Game.nsp")
	write(t, path, strings.Repeat("x", 100))
	drive := driveOf(path)
	bins.Lock()
	bins.found[drive], bins.at[drive] = bin{capacity: 10}, time.Now()
	bins.Unlock()
	t.Cleanup(func() {
		bins.Lock()
		delete(bins.found, drive)
		bins.Unlock()
	})
	unavailable, ok := Why(Move(path))
	if !ok || unavailable.Reason != TooBig {
		t.Fatalf("%+v", unavailable)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("the file is gone")
	}
}

// The size Windows gives the Recycle Bin of a drive without settings: 10 % of the first 40 GB and
// 5 % of the rest (49534 MB for a drive of 949732 MB, as Windows wrote it here).
func TestDefaultCapacity(t *testing.T) {
	for megabytes, want := range map[int64]int64{949732: 49534, 20480: 2048, 40960: 4096} {
		if got := capacityFor(megabytes); got != want {
			t.Errorf("%d MB: %d, not %d", megabytes, got, want)
		}
	}
}

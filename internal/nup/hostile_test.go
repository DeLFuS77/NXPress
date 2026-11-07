package nup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// A NUP is opened from files of the Internet: one made to harm, with a table of contents of a few
// hundred kilobytes, must not make NXPress take gigabytes or hours, nor close it. The library
// app has the same tests (switchfs/nuphostile_test.go).

type craftedDir struct {
	nameAt, nameLen, first, count uint32
}

type craftedFile struct {
	nameAt, nameLen uint32
}

// craftNup writes a NUP as a crafted file has it: packages with the roots given (all of them
// games of different IDs), folders, files of no bytes, children and a pool of names, with a table
// of contents whose hash matches.
func craftNup(roots []uint32, dirs []craftedDir, files []craftedFile, children []uint32, pool []byte) []byte {
	le := binary.LittleEndian
	records := tocHeadSize + len(roots)*packageSize + len(dirs)*dirSize + len(files)*fileSize
	poolAt := records + 4*len(children)
	toc := make([]byte, poolAt+len(pool))
	copy(toc, tocMagic)
	le.PutUint32(toc[0x04:], uint32(len(roots)))
	le.PutUint64(toc[0x08:], uint64(len(dirs)))
	le.PutUint64(toc[0x10:], uint64(len(files)))
	le.PutUint64(toc[0x18:], uint64(poolAt))
	le.PutUint64(toc[0x20:], uint64(len(pool)))
	for i, root := range roots {
		record := toc[tocHeadSize+i*packageSize:]
		le.PutUint64(record[0x00:], 0x0100000000010000+uint64(i)<<16)
		le.PutUint32(record[0x08:], root)
		le.PutUint32(record[0x0C:], NoDir)
		le.PutUint32(record[0x10:], NoDir)
	}
	at := tocHeadSize + len(roots)*packageSize
	for _, dir := range dirs {
		le.PutUint32(toc[at:], dir.nameAt)
		le.PutUint32(toc[at+4:], dir.nameLen)
		le.PutUint32(toc[at+8:], dir.first)
		le.PutUint32(toc[at+12:], dir.count)
		at += dirSize
	}
	for _, file := range files {
		le.PutUint32(toc[at:], file.nameAt)
		le.PutUint32(toc[at+4:], file.nameLen)
		at += fileSize
	}
	for _, child := range children {
		le.PutUint32(toc[at:], child)
		at += 4
	}
	copy(toc[poolAt:], pool)
	blob := (headerSize + len(toc) + 15) &^ 15
	nup := make([]byte, blob)
	copy(nup, magic)
	le.PutUint16(nup[0x04:], headerSize)
	le.PutUint64(nup[0x08:], headerSize)
	le.PutUint64(nup[0x10:], uint64(len(toc)))
	sum := sha256.Sum256(toc)
	copy(nup[0x18:], sum[:])
	le.PutUint64(nup[0x38:], uint64(blob))
	le.PutUint64(nup[0x40:], uint64(blob))
	le.PutUint32(nup[0x48:], uint32(len(roots)))
	copy(nup[headerSize:], toc)
	return nup
}

func readCrafted(nup []byte) (*Container, error) {
	return Read(bytes.NewReader(nup), int64(len(nup)))
}

func TestReadRefusesNamesLongerThanThePathsOfTheConsole(t *testing.T) {
	pool := bytes.Repeat([]byte("a"), maxName+1)
	for _, length := range []uint32{maxName, maxName + 1} {
		_, err := readCrafted(craftNup([]uint32{0}, []craftedDir{{0, 0, 0, 1}}, []craftedFile{{0, length}}, []uint32{1}, pool))
		if length <= maxName && err != nil {
			t.Errorf("a name of %d bytes: %v", length, err)
		}
		if length > maxName && (err == nil || !strings.Contains(err.Error(), "too long")) {
			t.Errorf("a name of %d bytes: %v", length, err)
		}
	}
}

// Every record of a crafted NUP may name the same bytes of the pool: the names are not copied.
func TestReadDoesNotCopyTheNamesForEachRecord(t *testing.T) {
	const files = 6500
	records := make([]craftedFile, files)
	children := make([]uint32, files)
	for i := range records {
		records[i] = craftedFile{0, maxName}
		children[i] = uint32(1 + i)
	}
	nup := craftNup([]uint32{0}, []craftedDir{{0, 0, 0, files}}, records, children, bytes.Repeat([]byte("n"), maxName))
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	container, err := readCrafted(nup)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	// the table of contents, the records and little else: a copy of each name alone would be
	// 6500 × 768 bytes
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > uint64(4*len(nup)) {
		t.Errorf("reading a NUP of %d bytes allocated %d bytes", len(nup), allocated)
	}
	if container.Files[files-1].Name != strings.Repeat("n", maxName) {
		t.Errorf("name %q", container.Files[files-1].Name[:8])
	}
}

func TestWalkRefusesPathsLongerThanThePathsOfTheConsole(t *testing.T) {
	// root/a/b/file, each name of 0x300 bytes: past 0x400 at b
	pool := bytes.Repeat([]byte("p"), maxName)
	dirs := []craftedDir{{0, 0, 0, 1}, {0, maxName, 1, 1}, {0, maxName, 2, 1}}
	container, err := readCrafted(craftNup([]uint32{0}, dirs, []craftedFile{{0, 4}}, []uint32{1, 2, 3}, pool))
	if err != nil {
		t.Fatal(err)
	}
	if err := container.Walk(0, func(string, int) error { return nil }); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Errorf("walk: %v", err)
	}
	if err := container.walkFolders(0, func(string) error { return nil }); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Errorf("walk of the folders: %v", err)
	}
	// one level of them is a path of the console
	container, err = readCrafted(craftNup([]uint32{0}, dirs[:2], []craftedFile{{0, 4}}, []uint32{1, 2}, pool))
	if err != nil {
		t.Fatal(err)
	}
	walked := []string{}
	if err := container.Walk(0, func(path string, _ int) error { walked = append(walked, path); return nil }); err != nil || len(walked) != 1 {
		t.Errorf("walk %d files: %v", len(walked), err)
	}
}

// Folders whose ranges of children overlap: each walk goes through the shared children for each
// of them, and a crafted NUP of a hundred kilobytes keeps Details and Verify at it for hours. The
// packers of the format never write them, and its check refuses them.
func TestReadRefusesFoldersThatShareTheirChildren(t *testing.T) {
	const folders, files = 64, 100
	dirs := []craftedDir{{0, 0, 0, files}}
	for range folders {
		dirs = append(dirs, craftedDir{0, 1, 0, files})
	}
	records := make([]craftedFile, files)
	children := make([]uint32, files)
	for i := range children {
		records[i] = craftedFile{0, 1}
		children[i] = uint32(len(dirs) + i)
	}
	_, err := readCrafted(craftNup([]uint32{0}, dirs, records, children, []byte("x")))
	if err == nil || !strings.Contains(err.Error(), "share their children") {
		t.Errorf("read: %v", err)
	}
}

// Packages that all have the same tree: each of them walks it whole.
func TestReadRefusesPackagesThatShareATree(t *testing.T) {
	const files = 1000
	records := make([]craftedFile, files)
	children := make([]uint32, files)
	for i := range children {
		records[i] = craftedFile{0, 1}
		children[i] = uint32(1 + i)
	}
	for _, c := range []struct {
		packages int
		refused  bool
	}{{1, false}, {2, false}, {64, true}, {maxPackages, true}} {
		roots := make([]uint32, c.packages)
		_, err := readCrafted(craftNup(roots, []craftedDir{{0, 0, 0, files}}, records, children, []byte("x")))
		if refused := err != nil && strings.Contains(err.Error(), "share their folders"); refused != c.refused {
			t.Errorf("%d packages with one tree: %v", c.packages, err)
		}
	}
}

// A file record in two folders would be written twice, past what the card and the check of the
// free space tell: Unpack refuses it before it writes anything.
func TestUnpackRefusesAFileAtTwoPlaces(t *testing.T) {
	dirs := []craftedDir{{0, 0, 0, 2}, {0, 1, 2, 1}, {1, 1, 3, 1}}
	nup := craftNup([]uint32{0}, dirs, []craftedFile{{2, 1}}, []uint32{1, 2, 3, 3}, []byte("abf"))
	path := filepath.Join(t.TempDir(), "Game.nup")
	if err := os.WriteFile(path, nup, 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	_, err := Unpack(context.Background(), path, dest, 1, 1, func(int64, int64) {})
	if err == nil || !strings.Contains(err.Error(), "more than one place") {
		t.Errorf("unpack: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("the folder was made: %v", err)
	}
}

// A module whose header says its compressed text takes 4 GiB, made of zeros a NUP stores in a few
// kilobytes: no LZ4 data of 1 MiB takes so much, and the check tells it without gathering it.
func TestNsoCheckDoesNotGatherWhatLz4CannotMake(t *testing.T) {
	header := make([]byte, nsoHeaderSize)
	copy(header, nsoMagic)
	le := binary.LittleEndian
	le.PutUint32(header[0x0C:], 0b001001)
	le.PutUint32(header[0x10:], nsoHeaderSize)
	le.PutUint32(header[0x18:], 1<<20)
	le.PutUint32(header[0x60:], 0xFFFFFF00)
	check := newNsoCheck(nsoHeaderSize + 0xFFFFFF00)
	check.Write(header)
	zeros := make([]byte, 1<<20)
	for range 16 {
		check.Write(zeros)
	}
	if len(check.segments) != 0 {
		t.Errorf("%d segments read", len(check.segments))
	}
	if problem := check.result(); problem != "its text segment cannot decompress to the 1048576 bytes its header says" {
		t.Errorf("problem %q", problem)
	}
}

// A segment stored as it is is hashed as it goes by, not gathered.
func TestNsoCheckHashesAPlainSegmentAsItGoesBy(t *testing.T) {
	module := madeUpNso(false)
	check := newNsoCheck(int64(len(module)))
	check.Write(module[:nsoHeaderSize+100])
	if len(check.segments) != 3 || check.segments[0].data != nil || check.segments[0].read != 100 {
		t.Fatalf("segments %+v", check.segments)
	}
	check.Write(module[nsoHeaderSize+100:])
	if problem := check.result(); problem != "" {
		t.Errorf("problem %q", problem)
	}
}

func TestLz4DecompressToGivesTheBytesOfTheBlock(t *testing.T) {
	// literals, short matches, a run longer than the window and data that does not compress
	src := append(bytes.Repeat([]byte("the text of a module, "), 20000), make([]byte, 300000)...)
	src = append(src, noise(200000, 4)...)
	src = append(src, bytes.Repeat([]byte{1, 2, 3, 4, 5, 6, 7}, 50000)...)
	want := sha256.Sum256(src)
	block := testnup.Lz4(src)
	hash := sha256.New()
	if err := lz4DecompressTo(block, len(src), hash); err != nil || !bytes.Equal(hash.Sum(nil), want[:]) {
		t.Fatalf("lz4DecompressTo: %v", err)
	}
	for _, size := range []int{len(src) - 1, len(src) + 1} {
		if err := lz4DecompressTo(block, size, sha256.New()); err == nil {
			t.Errorf("%d bytes instead of %d", size, len(src))
		}
	}
	if err := lz4DecompressTo(block[:len(block)-3], len(src), sha256.New()); err == nil {
		t.Error("a block cut short")
	}
}

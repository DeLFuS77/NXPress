package nxpack

import (
	"bytes"
	"crypto/sha256"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// files writes the files given in a folder and gives them as the files of a version.
func files(t *testing.T, contents map[string][]byte) []File {
	t.Helper()
	dir := t.TempDir()
	result := []File{}
	for path, data := range contents {
		source := filepath.Join(dir, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(source), 0o755)
		if err := os.WriteFile(source, data, 0o644); err != nil {
			t.Fatal(err)
		}
		result = append(result, File{Path: path, Source: source, Size: int64(len(data))})
	}
	return result
}

// referenceCheck opens and mounts the .nx at path with the reader of the format that the
// emulators ship (nx_reader.hpp), built as the program NX_CHECK names; without it, nothing.
func referenceCheck(t *testing.T, path string) {
	t.Helper()
	check := os.Getenv("NX_CHECK")
	if check == "" {
		return
	}
	if out, err := exec.Command(check, path).CombinedOutput(); err != nil {
		t.Errorf("the reference reader refuses %s: %s", filepath.Base(path), out)
	}
}

func noise(n int, seed int64) []byte {
	data := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(data)
	return data
}

func TestWriteReadBack(t *testing.T) {
	big := noise(700<<10, 1)                           // three blocks of 256 KiB, the last one shorter
	text := bytes.Repeat([]byte("stage data "), 30000) // compresses: blocks of 64 KiB with zstd
	nacp := make([]byte, 0x4000)
	copy(nacp, "Test Game")
	copy(nacp[0x200:], "Test Publisher")
	icon := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, noise(100, 2)...)
	base := files(t, map[string][]byte{"exefs/main": noise(1000, 3), "exefs/main.npdm": []byte("META"), "romfs/big.bin": big, "romfs/text.txt": text, "romfs/empty": nil})
	update := files(t, map[string][]byte{"exefs/main": noise(1200, 4), "exefs/main.npdm": []byte("META"), "romfs/big.bin": big, "romfs/text.txt": text, "romfs/new.bin": noise(5000, 5)})
	dlc := files(t, map[string][]byte{"romfs/dlc.bin": noise(3000, 6)})
	game := Game{ID: 0x0100000000010000, Platform: 1, Versions: []Snapshot{
		{Kind: Update, Version: 65536, Program: 0x0100000000010000, Display: "1.1.0", Files: update, Nacp: nacp, Icon: icon,
			Title: "Test Game", Publisher: "Test Publisher", Names: map[string]string{"en-us": "Test Game", "ja": "テスト"}, BuildID: bytes.Repeat([]byte{7}, 32)},
		{Kind: Base, Version: 0, Program: 0x0100000000010000, Display: "1.0.0", Files: base, Dirs: []string{"exefs", "romfs", "romfs/empty dir"}},
		{Kind: Dlc, Version: 0, Addon: 0x0100000000011001, Files: dlc},
	}}
	out := filepath.Join(t.TempDir(), "game.nx")
	result, err := Write(t.Context(), out, []Game{game}, Options{Workers: 4}, nil)
	if err != nil {
		t.Fatal(err)
	}
	referenceCheck(t, out)
	// big.bin, text.txt and main.npdm are the same in the game and its update: one record each
	if result.Files != 8 {
		t.Errorf("%d records of files, want 8", result.Files)
	}
	data, err := switchfs.OpenNxData(out)
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	if err := data.Verify(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	metadata := data.Metadata()
	if m := metadata["0100000000010800"]; m == nil || m.Type != "UPD" || m.Version != 65536 || m.Ncap == nil ||
		m.Ncap.TitleName["AmericanEnglish"].Title != "Test Game" {
		t.Errorf("update: %+v", m)
	}
	if m := metadata["0100000000011001"]; m == nil || m.Type != "DLC" {
		t.Errorf("DLC: %+v", m)
	}
	if icon2, err := data.Icon(); err != nil || !bytes.Equal(icon2, icon) {
		t.Errorf("icon: %v", err)
	}
	app := data.Applications[0]
	if app.Title != "Test Game" || app.Names["ja"] != "テスト" || app.Snapshots[app.MetadataSnapshot].Type != switchfs.NxSnapshotUpdate {
		t.Errorf("game: %+v", app)
	}
	// every file of every version reads back as it was written
	sources := map[int][]File{}
	for _, v := range game.Versions {
		sources[int(v.Kind)] = v.Files
	}
	for s, snapshot := range app.Snapshots {
		want := map[string][]byte{}
		for _, f := range sources[snapshot.Type] {
			want[f.Path], _ = os.ReadFile(f.Source)
		}
		got := map[string][]byte{}
		dirs := []string{}
		err := data.Walk(0, s, func(path string) error {
			dirs = append(dirs, path)
			return nil
		}, func(path string, index int) error {
			var buffer bytes.Buffer
			if err := data.WriteFile(t.Context(), index, &buffer, nil); err != nil {
				return err
			}
			got[path] = buffer.Bytes()
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Errorf("version %d: %d files, want %d", s, len(got), len(want))
		}
		for path, bytes1 := range want {
			if sha256.Sum256(got[path]) != sha256.Sum256(bytes1) {
				t.Errorf("version %d: %s differs", s, path)
			}
		}
		if snapshot.Type == switchfs.NxSnapshotBase && len(dirs) != 4 {
			t.Errorf("the folders of the game: %v", dirs)
		}
	}
}

func TestWriteRefuses(t *testing.T) {
	out := filepath.Join(t.TempDir(), "x.nx")
	for name, game := range map[string]Game{
		"no ID":          {Versions: []Snapshot{{Kind: Base, Program: 1}}},
		"DLC with no ID": {ID: 1, Versions: []Snapshot{{Kind: Dlc}}},
		"bad path":       {ID: 1, Versions: []Snapshot{{Kind: Base, Program: 1, Files: []File{{Path: "../x"}}}}},
		"same twice":     {ID: 1, Versions: []Snapshot{{Kind: Base, Program: 1}, {Kind: Base, Program: 1}}},
	} {
		if _, err := Write(t.Context(), out, []Game{game}, Options{}, nil); err == nil {
			t.Errorf("%s: written", name)
		}
		if _, err := os.Stat(out); err == nil || func() bool { _, e := os.Stat(out + ".part"); return e == nil }() {
			t.Errorf("%s: a file was left", name)
		}
	}
}

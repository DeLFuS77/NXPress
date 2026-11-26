package jobs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"
	"github.com/DeLFuS77/NXPress/internal/testnup"
	"github.com/DeLFuS77/NXPress/internal/testnx"
)

func splitTestNup() []byte {
	return testnup.Build([]testnup.Package{
		{TitleId: 0x0400AAAA00010000, Kind: testnup.Base, Meta: []testnup.Record{{Type: testnup.TitleName, Payload: []byte("Made-up Game")}},
			Files: []testnup.File{{Path: "romfs/a.bin", Data: bytes.Repeat([]byte("data of the game "), 1000), Zstd: true}, {Path: "exefs/main", Data: []byte("main")}}},
		{TitleId: 0x0400AAAA00010000, Kind: testnup.Update, Version: 65536},
	})
}

func splitTestNx() []byte {
	return testnx.Build(testnx.Game{Id: 0x0400AAAA00010000, Platform: 2, Title: "Made-up Game", NacpName: "Made-up Game",
		Snapshots: []testnx.Snapshot{{Kind: testnx.Base}, {Kind: testnx.Update, Version: 65536}}})
}

// FAT32 splits a NUP and a .nx into a folder of parts with their name, as an NSP, and joins the
// folder back into the file byte for byte; a whole one that fits is left as it is.
func TestFat32SplitsAndJoinsNupAndNx(t *testing.T) {
	defer func(size int64) { fat32Part = size }(fat32Part)
	ctx := context.Background()
	for name, data := range map[string][]byte{"Made-up Game [0400AAAA00010000][v0].nup": splitTestNup(), "Made-up Game.nx": splitTestNx()} {
		fat32Part = int64(len(data)/3 + 1)
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, note, err := doJob(ctx, "fat32", path, Options{}, 1, testkit.Quiet{}); err != nil || note != "it fits in FAT32 as it is" {
			t.Fatalf("%s: %q %v", name, note, err)
		}
		if got, _, err := splitParts(ctx, path, int64(len(data)), Options{}, testkit.Quiet{}); err != nil || got != path {
			t.Fatalf("%s: split %q %v", name, got, err)
		}
		if _, err := os.Stat(filepath.Join(path, "02")); err != nil {
			t.Fatalf("%s: the parts: %v", name, err)
		}
		if !EmulatorParts(path) {
			t.Fatalf("%s: the folder of parts is not told", name)
		}
		if files, _ := Inputs([]string{filepath.Dir(path)}, Takes["fat32"]); len(files) != 1 {
			t.Fatalf("%s: FAT32 takes %v", name, files)
		}
		got, note, err := doJob(ctx, "fat32", path, Options{}, 1, testkit.Quiet{})
		if err != nil || got != path || note != "3 parts joined" {
			t.Fatalf("%s: join %q %q %v", name, got, note, err)
		}
		if joined, err := os.ReadFile(path); err != nil || !bytes.Equal(joined, data) {
			t.Fatalf("%s: joined otherwise: %v", name, err)
		}
	}
}

// splitInFolder writes data in a folder of parts named name, as FAT32 does.
func splitInFolder(t *testing.T, name string, data []byte, size int) string {
	t.Helper()
	folder := filepath.Join(t.TempDir(), name)
	os.Mkdir(folder, 0o755)
	for part := 0; len(data) > 0; part++ {
		n := min(size, len(data))
		if err := os.WriteFile(filepath.Join(folder, fmt.Sprintf("%02d", part)), data[:n], 0o644); err != nil {
			t.Fatal(err)
		}
		data = data[n:]
	}
	return folder
}

// Verify and Details take a NUP and a .nx in parts as the file whole; the works that write them
// in place, or rename them, leave them out.
func TestNupAndNxInPartsAreRead(t *testing.T) {
	ctx := context.Background()
	nupData, nxData := splitTestNup(), splitTestNx()
	nupFolder := splitInFolder(t, "Made-up Game [0400AAAA00010000][v0].nup", nupData, len(nupData)/3+1)
	nxFolder := splitInFolder(t, "Made-up Game.nx", nxData, 5000)
	for _, action := range []string{"verify", "info", "decompress", "tonx"} {
		if files, _ := Inputs([]string{nupFolder}, Takes[action]); len(files) != 1 {
			t.Errorf("%s takes %v", action, files)
		}
	}
	for _, action := range []string{"info", "verify", "nup"} {
		if files, _ := Inputs([]string{filepath.Dir(nxFolder)}, Takes[action]); len(files) != 1 {
			t.Errorf("%s takes %v of a folder with a .nx in parts", action, files)
		}
	}
	for _, action := range []string{"rename", "fixmeta", "fixicon", "compress"} {
		if files, _ := Inputs([]string{nupFolder}, Takes[action]); len(files) != 0 {
			t.Errorf("%s takes %v", action, files)
		}
	}

	if _, note, err := doJob(ctx, "verify", nupFolder, Options{NoCache: true}, 1, testkit.Quiet{}); err != nil || !strings.Contains(note, "checked by their SHA-256") {
		t.Fatalf("verify the NUP in parts: %q %v", note, err)
	}
	if _, note, err := doJob(ctx, "info", nupFolder, Options{}, 1, testkit.Quiet{}); err != nil || note == "" {
		t.Fatalf("details of the NUP in parts: %q %v", note, err)
	}
	title, err := ReadTitle(nupFolder, GameCover)
	if err != nil || title.Name != "Made-up Game" || title.Size != int64(len(nupData)) {
		t.Fatalf("title of the NUP in parts: %+v %v", title, err)
	}
	card, err := NupCard(nupFolder)
	if err != nil || card.Size != int64(len(nupData)) {
		t.Fatalf("card of the NUP in parts: %+v %v", card, err)
	}

	if _, note, err := doJob(ctx, "info", nxFolder, Options{}, 1, testkit.Quiet{}); err != nil || !strings.Contains(note, "2 titles") {
		t.Fatalf("details of the .nx in parts: %q %v", note, err)
	}
	nx, err := NxCard(nxFolder)
	if err != nil || nx.Size != int64(len(nxData)) || len(nx.Packages) != 2 {
		t.Fatalf("card of the .nx in parts: %+v %v", nx, err)
	}
	if title, err := ReadTitle(nxFolder, NoCover); err != nil || title.Size != int64(len(nxData)) {
		t.Fatalf("title of the .nx in parts: %+v %v", title, err)
	}
	report, err := Report(nupFolder, nil)
	if err != nil || !strings.Contains(report, "Size:    "+bytesText(int64(len(nupData)))) {
		t.Fatalf("report of the NUP in parts: %v\n%s", err, report)
	}
}

package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// The window reads a NUP without keys: its main title with the name, the newest update and the
// icon it carries, and for its card each package with its files.
func TestNupTitleAndCard(t *testing.T) {
	packages := []testnup.Package{
		{TitleId: 0x0400245021D2C000, Kind: testnup.Base, Version: 0,
			Meta: []testnup.Record{{Type: testnup.TitleName, Payload: []byte("Made-up Book [0400245021D2C000]")},
				{Type: testnup.Publisher, Payload: []byte("Nobody")}, {Type: testnup.Icon, Payload: testnup.Jpeg}},
			Files: []testnup.File{{Path: "romfs/a.bin", Data: []byte(strings.Repeat("a", 5000)), Zstd: true}, {Path: "exefs/main", Data: []byte("main")}}},
		{TitleId: 0x0400245021D2C000, Kind: testnup.Update, Version: 131072, Meta: []testnup.Record{{Type: testnup.DisplayVersion, Payload: []byte("1.0.2")}},
			Files: []testnup.File{{Path: "romfs/a.bin", Data: []byte(strings.Repeat("b", 5000)), Zstd: true}}},
		{TitleId: 0x0400245021D2C000, Kind: testnup.Update, Version: 65536,
			Files: []testnup.File{{Path: "romfs/a.bin", Data: []byte(strings.Repeat("c", 5000)), Zstd: true}}},
	}
	path := filepath.Join(t.TempDir(), "book.nup")
	os.WriteFile(path, testnup.Build(packages), 0o644)
	title, err := ReadTitle(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if title.Name != "Made-up Book" || title.TitleID != "0400245021D2C000" || title.Type != "BASE" || title.Latest != 131072 ||
		title.VersionTxt != "1.0.2" || title.Updates != 2 || title.Bases != 1 || title.Titles != 3 || title.Publisher != "Nobody" ||
		title.Container != "NUP" || !strings.HasPrefix(title.Icon, "data:image/jpeg;base64,") {
		t.Errorf("%+v", title)
	}
	size, uncompressed, cards, err := NupCard(path)
	if err != nil {
		t.Fatal(err)
	}
	if size <= 0 || uncompressed != 5000*3+4 || len(cards) != 3 {
		t.Fatalf("%d %d %+v", size, uncompressed, cards)
	}
	if cards[0].Name != "Made-up Book" || cards[0].Files != 2 || cards[0].Size != 5004 || cards[0].Stored >= cards[0].Size ||
		cards[1].TitleID != "0400245021D2C800" || cards[1].Type != "UPD" || cards[1].Shown != "1.0.2" {
		t.Errorf("%+v", cards)
	}

	// a NUP alone needs no keys, with an NSP it does, and a work that does not take NUP files
	// tells it leaves them as they are
	if NeedsKeys(Options{Verify: true, Files: []string{path}}) || NeedsKeys(Options{Info: true, Files: []string{path}}) ||
		NeedsKeys(Options{Compress: true, Files: []string{path}}) {
		t.Error("keys needed for a NUP")
	}
	nsp := filepath.Join(t.TempDir(), "a.nsp")
	os.WriteFile(nsp, []byte("PFS0"), 0o644)
	if !NeedsKeys(Options{Verify: true, Files: []string{path, nsp}}) {
		t.Error("no keys needed with an NSP")
	}
	out := &recording{}
	RunJobs(t.Context(), Options{Compress: true, Files: []string{filepath.Dir(path)}, Multi: 1}, out)
	if len(out.skipped) != 1 || !strings.Contains(out.skipped[0], "does not compress, convert or trim NUP files") {
		t.Errorf("%+v", out)
	}
}

type recording struct {
	skipped []string
	said    []string
}

func (r *recording) Progress(string, string) func(int64, int64) { return func(int64, int64) {} }
func (r *recording) Done(path string, output string, note string, err error) {
	if IsSkipped(err) {
		r.skipped = append(r.skipped, err.Error())
	}
}
func (r *recording) Say(format string, args ...any) { r.said = append(r.said, format) }

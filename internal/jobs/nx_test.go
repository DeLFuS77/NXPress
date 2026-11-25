package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnx"
)

func writeNx(t *testing.T) string {
	t.Helper()
	data := testnx.Build(testnx.Game{Id: 0x0100000000010000, Title: "Test Game", Publisher: "Test Publisher", NacpName: "Test Game",
		Snapshots: []testnx.Snapshot{
			{Kind: testnx.Base, Display: "1.0.0"},
			{Kind: testnx.Update, Version: 131072, Display: "1.2.0"},
			{Kind: testnx.Dlc, Addon: 0x0100000000011001},
			{Kind: testnx.Dlc},
		}})
	path := filepath.Join(t.TempDir(), "Test Game.nx")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Details reads a .nx without keys: its title, its card and its full report.
func TestDetailsOfNx(t *testing.T) {
	path := writeNx(t)
	if NeedsKeys(Options{Info: true, Files: []string{path}}) {
		t.Error("a .nx needs no keys")
	}
	title, err := ReadTitle(path, GameCover)
	if err != nil {
		t.Fatal(err)
	}
	if title.TitleID != "0100000000010000" || title.Name != "Test Game" || title.Container != "NX" || title.Icon == "" || title.Dlcs != 1 {
		t.Errorf("title: %+v", title)
	}
	card, err := NxCard(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.Packages) != 4 || card.Icon == nil || card.Packages[1].Type != "UPD" || card.Packages[1].TitleID != "0100000000010800" {
		t.Errorf("card: %+v", card)
	}
	report, err := Report(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Titles: 4", "Update ID:", "0100000000010800", "shown as 1.2.0", "0100000000011001", "in the CNMT", ".nx File Size:", "Format:", "NX 0.2"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report lacks %q:\n%s", want, report)
		}
	}
}

// Any other work leaves a .nx as it is, and says why.
func TestOtherWorksLeaveNx(t *testing.T) {
	path := writeNx(t)
	for _, action := range []string{"verify", "decompress", "rename"} {
		_, _, err := doJob(t.Context(), action, path, Options{}, 1, nil)
		if err == nil || !strings.Contains(err.Error(), "leaves .nx files") {
			t.Errorf("%s: %v", action, err)
		}
	}
}

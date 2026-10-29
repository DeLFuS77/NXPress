package main

import (
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

func TestNewName(t *testing.T) {
	title := gameTitle{Name: "Some: Game?", TitleID: "0100000000010000", Version: 65536, Type: "BASE", Titles: 2}
	for template, want := range map[string]string{
		"":                                  "Some - Game [0100000000010000][v65536].nsz",
		"{name} ({titles} titles)":          "Some - Game (2 titles).nsz",
		"{id} {type}":                       "0100000000010000 BASE.nsz",
		"{name} [{id}][v{version}] / copy.": "Some - Game [0100000000010000][v65536] - copy.nsz",
	} {
		if got := newName("C:/games/x.nsz", title, template); got != want {
			t.Fatalf("%q: %q, want %q", template, got, want)
		}
	}
	// without a name, the name of the file without its tags
	if got := newName("C:/games/Game (Pack)[0100000000011001][v65536].nsp", gameTitle{TitleID: "0100000000011001", Version: 65536}, ""); got != "Game (Pack) [0100000000011001][v65536].nsp" {
		t.Fatal(got)
	}
	// without a name, the name of the file
	if got := newName("C:/games/old name.nsp", gameTitle{TitleID: "0100000000010800", Version: 1}, "{name} [{id}]"); got != "old name [0100000000010800].nsp" {
		t.Fatal(got)
	}
}

// The fields of Switch Library Manager work as written there; a field with no value leaves no
// empty brackets behind.
func TestNewNameFields(t *testing.T) {
	pack := gameTitle{Name: "Hollow Knight", TitleID: "0100633007D48000", Version: 0, Latest: 65536, VersionTxt: "1.5.12183",
		Type: "BASE", Titles: 3, Bases: 1, Updates: 1, Dlcs: 1, Size: 2_650_000_000}
	for template, want := range map[string]string{
		"{name} [{id}][v{version}] ({contents})":                                       "Hollow Knight [0100633007D48000][v65536] (1G+1U+1D).nsp",
		"{TITLE_NAME} [{TITLE_ID}] (v{VERSION}) (v{VERSION_TXT}) ({PACKAGE_CONTENTS})": "Hollow Knight [0100633007D48000] (v65536) (v1.5.12183) (1G+1U+1D).nsp",
		"{name} {size_gb} {SIZE_MB}":                                                   "Hollow Knight 2.7GB 2650MB.nsp",
		"{TITLE_NAME} ({DLC_NAME})[{REGION}][{TYPE}]":                                  "Hollow Knight [BASE].nsp",
	} {
		if got := newName("C:/games/x.nsp", pack, template); got != want {
			t.Errorf("%q: %q, want %q", template, got, want)
		}
	}
	update := gameTitle{Name: "Capcom Fighting Collection 2", TitleID: "010044901C5C2800", Version: 262144, Latest: 262144, VersionTxt: "1.0.5", Type: "UPD", Titles: 1, Updates: 1}
	if got := newName("C:/games/x.nsp", update, "{name} [{id}][v{version_txt}] ({contents})"); got != "Capcom Fighting Collection 2 [010044901C5C2800][v1.0.5] (1U).nsp" {
		t.Error(got)
	}
	// a name taken from the file leaves out the tags the templates write, not the name of a DLC
	dlc := gameTitle{TitleID: "0100B6400CA57088", Version: 0, VersionTxt: "1.0.0", Type: "DLC", Titles: 2, Dlcs: 2, Size: 242064}
	if got := newName("C:/games/DAEMON X MACHINA (Arsenal Decal Eagle) [0100B6400CA57088] (v0) (v1.0.0) (2D) 0.0GB.nsp", dlc, "{name} [{id}] (v{version}) ({contents}) {size_mb}"); got != "DAEMON X MACHINA (Arsenal Decal Eagle) [0100B6400CA57088] (v0) (2D) 0MB.nsp" {
		t.Error(got)
	}
	// a version shown that is not known leaves nothing
	if got := newName("C:/games/x.nsp", gameTitle{Name: "Game", TitleID: "0100000000010000", Type: "BASE"}, "{name} (v{version_txt}) [{id}]"); got != "Game (v) [0100000000010000].nsp" {
		t.Error(got)
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"1.0.1", "1.0.0", true}, {"1.1.0", "1.0.9", true}, {"2.0.0", "10.0.0", false}, {"1.0.0", "1.0.0", false}, {"1.0.0", "dev-abc", false}} {
		if got := newer(c.a, c.b); got != c.want {
			t.Fatalf("newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

// {dlc_number} names a DLC by its number, and --keep-named leaves a name with its title ID.
func TestDlcNumberAndKeptNames(t *testing.T) {
	dlc := gameTitle{Name: "Game", TitleID: "0100000000011002", Type: "DLC", Titles: 1}
	if got := newName("x.nsp", dlc, "{name} [DLC {dlc_number}] [{id}]"); got != "Game [DLC 2] [0100000000011002].nsp" {
		t.Errorf("a DLC: %q", got)
	}
	game := gameTitle{Name: "Game", TitleID: "0100000000010000", Type: "BASE", Titles: 1}
	if got := newName("x.nsp", game, "{name} [DLC {DLC_NUMBER}] [{id}]"); got != "Game [DLC ] [0100000000010000].nsp" && got != "Game [DLC] [0100000000010000].nsp" {
		t.Errorf("a game: %q", got)
	}
	if !keepsName(`C:\games\My name [0100000000011002].nsp`, "0100000000011002") || keepsName("My name [0100000000011003].nsp", "0100000000011002") {
		t.Error("keepsName")
	}
	if dlcNumber("0100000000010800") != "" || dlcNumber("010022900D3ED01C") != "28" {
		t.Error("dlcNumber")
	}
}

// over its cover a game tells its publisher, its languages, the firmware of its newest update
// and if it is a demo; a DLC tells its game and the version of it it needs
func TestDescribeTitlesForTheHover(t *testing.T) {
	base := &switchfs.ContentMetaAttributes{TitleId: "0100BFC002B4E000", Type: "BASE", RequiredTitleVersion: 0x30100000, Ncap: &switchfs.Nacp{
		TitleName:             map[string]switchfs.NacpTitle{"AmericanEnglish": {Title: "Game", Publisher: " Studio "}},
		SupportedLanguageFlag: 1<<0 | 1<<2 | 1<<6,
		Demo:                  true,
	}}
	update := &switchfs.ContentMetaAttributes{TitleId: "0100BFC002B4E800", Type: "UPD", Version: 65536, RequiredTitleVersion: 0x48000000}
	got := describeTitles([]*switchfs.ContentMetaAttributes{update, base})
	if got.Publisher != "Studio" || !got.Demo || got.Firmware != "18.0.0" {
		t.Errorf("publisher %q, demo %v, firmware %q", got.Publisher, got.Demo, got.Firmware)
	}
	if got.GameID != "" || got.NeedsGame != 0 {
		t.Errorf("a game tells a game of its own: %q %d", got.GameID, got.NeedsGame)
	}
	dlc := describeTitles([]*switchfs.ContentMetaAttributes{{TitleId: "0100BFC002B4F001", Type: "DLC", RequiredTitleVersion: 131072}})
	if dlc.GameID != "0100BFC002B4E000" || dlc.NeedsGame != 131072 || dlc.Firmware != "" {
		t.Errorf("DLC: game %q, needs %d, firmware %q", dlc.GameID, dlc.NeedsGame, dlc.Firmware)
	}
	if len(got.Languages) != 3 || got.Languages[0] != "en-US" || got.Languages[1] != "ja" || got.Languages[2] != "es" {
		t.Errorf("languages %v", got.Languages)
	}
}

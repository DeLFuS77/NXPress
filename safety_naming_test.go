package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A name made by a template is one every file system takes, and reads as the name of the game.
func TestNewNamesFileSystemsTake(t *testing.T) {
	game := func(name string) gameTitle {
		return gameTitle{Name: name, TitleID: "0100000000010000", Type: "BASE", Titles: 1}
	}
	for name, want := range map[string]string{
		// slashes and colons part words, they do not glue them
		"Fate/EXTELLA: Link": "Fate - EXTELLA - Link [0100000000010000][v0].nsp",
		// a dot first would hide the file
		".hack//G.U. Last Recode": "hack - G.U. Last Recode [0100000000010000][v0].nsp",
		"Game: The Sequel":        "Game - The Sequel [0100000000010000][v0].nsp",
	} {
		if got := newName("x.nsp", game(name), ""); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
	// a field no template knows is left out
	if got := newName("x.nsp", game("Game"), "{name} {titel} [{id}]"); got != "Game [0100000000010000].nsp" {
		t.Errorf("an unknown field: %q", got)
	}
	// a name Windows keeps for a device gets "_" first
	for name, want := range map[string]string{"CON": "_CON.nsp", "nul": "_nul.nsp", "Aux. Story": "_Aux. Story.nsp", "Console": "Console.nsp"} {
		if got := newName("x.nsp", game(name), "{name}"); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
	// too long: the name of the game gives way, the tags and the extension stay, and the
	// hidden name it is written under fits too
	long := strings.Repeat("ゲーム", 39)
	got := newName("x.nsp", game(long), "")
	if len("."+got+".nxpress") > 255 || !utf8.ValidString(got) || !strings.HasSuffix(got, " [0100000000010000][v0].nsp") || !strings.HasPrefix(got, "ゲーム") {
		t.Errorf("a long name, %d bytes: %q", len(got), got)
	}
	if got := newName("x.nsp", game("Game"), strings.Repeat("x", 300)+" {name}"); len(got) > maxNameBytes || !strings.HasSuffix(got, ".nsp") {
		t.Errorf("a long template, %d bytes: %q", len(got), got)
	}
}

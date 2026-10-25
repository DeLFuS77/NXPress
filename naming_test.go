package main

import "testing"

func TestNewName(t *testing.T) {
	title := gameTitle{Name: "Some: Game?", TitleID: "0100000000010000", Version: 65536, Type: "BASE", Titles: 2}
	for template, want := range map[string]string{
		"":                                  "Some Game [0100000000010000][v65536].nsz",
		"{name} ({titles} titles)":          "Some Game (2 titles).nsz",
		"{id} {type}":                       "0100000000010000 BASE.nsz",
		"{name} [{id}][v{version}] / copy.": "Some Game [0100000000010000][v65536] copy.nsz",
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

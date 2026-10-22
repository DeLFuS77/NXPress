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
	// without a name, the name of the file
	if got := newName("C:/games/old name.nsp", gameTitle{TitleID: "0100000000010800", Version: 1}, "{name} [{id}]"); got != "old name [0100000000010800].nsp" {
		t.Fatal(got)
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

package main

import "testing"

// only the addresses of SteamGridDB are asked for an image
func TestSteamAddress(t *testing.T) {
	for address, want := range map[string]bool{
		"https://cdn2.steamgriddb.com/icon/abc.png":      true,
		"https://www.steamgriddb.com/api/v2/icons/game/1": true,
		"https://steamgriddb.com/x.png":                  true,
		"http://cdn2.steamgriddb.com/icon/abc.png":       false,
		"https://steamgriddb.com.evil.test/x.png":        false,
		"https://evilsteamgriddb.com/x.png":              false,
		"https://user@cdn2.steamgriddb.com/x.png":        false,
		"file:///c:/x.png":                               false,
		"":                                               false,
	} {
		if got := steamAddress(address); got != want {
			t.Errorf("steamAddress(%q) = %v, want %v", address, got, want)
		}
	}
}

// the search page of the site for a name: the words apart with %20, what is not a letter written out
func TestSteamGridSite(t *testing.T) {
	for name, want := range map[string]string{
		"Donkey Kong Bananza":    "https://www.steamgriddb.com/search/grids?term=Donkey%20Kong%20Bananza",
		"  Fire Emblem: Weave ":  "https://www.steamgriddb.com/search/grids?term=Fire%20Emblem%3A%20Weave",
		"Tom & Jerry/100%":       "https://www.steamgriddb.com/search/grids?term=Tom%20%26%20Jerry%2F100%25",
		"":                       "https://www.steamgriddb.com/search/grids?term=",
	} {
		if got := steamGridSite(name); got != want {
			t.Errorf("steamGridSite(%q) = %q, want %q", name, got, want)
		}
	}
}

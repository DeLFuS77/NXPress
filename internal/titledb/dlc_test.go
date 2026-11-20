package titledb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// a titles.dlc.json as the library app publishes it, with made-up games
const dlcSample = `{"0100A1B002C4E000":{` +
	`"0100A1B002C4F002":{"name":"Nebula Outfits","releaseDate":20250204},` +
	`"0100A1B002C4F001":{"name":" Comet\u0007  Pack ","releaseDate":20250204},` +
	`"0100A1B002C4F003":{"name":"Moon Base Story","releaseDate":20261203},` +
	`"0100A1B002C4F004":{},` +
	`"0100A1B002C4F005":{"name":"Bad day","releaseDate":20251399},` +
	`"0100A1B002C4E800":{"name":"An update"},"0100B00000001001":{"name":"Of another game"},"not an id":{}},` +
	`"0100A1B002C4E001":{"0100A1B002C4F001":{}},"0400B8E0019D4000":{"0400B8E0019D5001":{"name":"Made-up Pack"}}}`

func TestParseDLCs(t *testing.T) {
	games, err := parseDLCs([]byte(dlcSample))
	if err != nil {
		t.Fatal(err)
	}
	list := &DLCs{games: games}
	got, ok := list.Of(0x0100A1B002C4E000, "2026-11-20")
	want := GameDLC{
		// the oldest first, the ones without a day last; not out yet apart
		Out: []DLC{{ID: "0100A1B002C4F001", Name: "Comet Pack", Date: "2025-02-04"}, {ID: "0100A1B002C4F002", Name: "Nebula Outfits", Date: "2025-02-04"},
			{ID: "0100A1B002C4F004"}, {ID: "0100A1B002C4F005", Name: "Bad day"}},
		Soon: []DLC{{ID: "0100A1B002C4F003", Name: "Moon Base Story", Date: "2026-12-03"}},
	}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("Of = %+v %v, want %+v", got, ok, want)
	}
	// on its day, it is out
	if got, _ := list.Of(0x0100A1B002C4E000, "2026-12-03"); len(got.Out) != 5 || len(got.Soon) != 0 {
		t.Errorf("on its day: %+v", got)
	}
	// a game of the Switch 2 the list knows
	if got, ok := list.Of(0x0400B8E0019D4000, "2026-11-20"); !ok || len(got.Out) != 1 || got.Out[0].Name != "Made-up Pack" {
		t.Errorf("Switch 2: %+v %v", got, ok)
	}
	// a game the list does not know, and a key that is not of a game
	for _, game := range []uint64{0x0100C3D00A7F6000, 0x0100A1B002C4E001} {
		if _, ok := list.Of(game, "2026-11-20"); ok {
			t.Errorf("%016X is known", game)
		}
	}
	if len(games) != 2 {
		t.Errorf("games %v", games)
	}
	for _, broken := range []string{"", "[]", "{}", `{"0100A1B002C4E000":{}}`, `{"0100A1B002C4E000":{"0100A1B002C4E800":{}}}`, "<html>"} {
		if _, err := parseDLCs([]byte(broken)); err == nil {
			t.Errorf("parseDLCs(%q) gives no error", broken)
		}
	}
	var none *DLCs
	if _, ok := none.Of(0x0100A1B002C4E000, "2026-11-20"); ok {
		t.Error("no list gives DLC")
	}
}

// The list is downloaded once a day at most and kept, as versions.json; before the data release
// has it, there is none and nothing is said.
func TestRefreshDLCs(t *testing.T) {
	asked, there := 0, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		if !there {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", `"dlc"`)
		if r.Header.Get("If-None-Match") == `"dlc"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Write([]byte(dlcSample))
	}))
	defer server.Close()
	old := DLCAddresses
	DLCAddresses = []string{server.URL + "/titles.dlc.json"}
	defer func() { DLCAddresses = old }()

	folder := t.TempDir()
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	if dlcs, err := RefreshDLCs(context.Background(), folder, server.Client(), now); dlcs != nil || err == nil {
		t.Fatalf("with no file published: %v, %v", dlcs, err)
	}
	there = true
	dlcs, err := RefreshDLCs(context.Background(), folder, server.Client(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := dlcs.Of(0x0100A1B002C4E000, "2026-11-20"); !ok || len(got.Out) != 4 || asked != 2 {
		t.Fatalf("%+v %v, asked %d", got, ok, asked)
	}
	if _, err := os.Stat(filepath.Join(folder, DLCFile)); err != nil {
		t.Fatal(err)
	}
	// within a day, the copy kept; after it, asked whether it changed
	if dlcs, err := RefreshDLCs(context.Background(), folder, server.Client(), now.Add(time.Hour)); err != nil || dlcs == nil || asked != 2 {
		t.Errorf("within a day: %v, asked %d", err, asked)
	}
	if dlcs, err := RefreshDLCs(context.Background(), folder, server.Client(), now.Add(25*time.Hour)); err != nil || dlcs == nil || asked != 3 {
		t.Errorf("a day later: %v, asked %d", err, asked)
	}
	// with no connection, the copy kept
	server.Close()
	if dlcs, _ := RefreshDLCs(context.Background(), folder, http.DefaultClient, now.Add(50*time.Hour)); dlcs == nil {
		t.Error("offline: no copy")
	}
	if kept, err := LoadDLCs(folder); err != nil || kept == nil {
		t.Errorf("load: %v", err)
	}
	if err := ForgetDLCs(folder); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDLCs(folder); err == nil {
		t.Error("the copy is still there")
	}
}

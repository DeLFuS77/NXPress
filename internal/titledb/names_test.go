package titledb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// a titles.switch2.json as the library app publishes it
const namesSample = `{"source":"https://switchbrew.org/wiki/Switch_2:_Title_list/Games","titles":{` +
	`"040074A01BF12000":{"name":"Made-up Bananza","regions":["US"]},` +
	`"040003C023FC4000":{"name":"LEGO� Made-up�:  Legacy"},"not an id":{"name":"x"},"0400000000001000":{"name":"  "}}}`

func TestParseNames(t *testing.T) {
	names, err := parseNames([]byte(namesSample))
	if err != nil {
		t.Fatal(err)
	}
	list := &Names{names: names}
	if name, ok := list.Name(0x040074A01BF12000); !ok || name != "Made-up Bananza" {
		t.Errorf("game: %q %v", name, ok)
	}
	// by the ID of its update too
	if name, ok := list.Name(0x040074A01BF12800); !ok || name != "Made-up Bananza" {
		t.Errorf("update: %q %v", name, ok)
	}
	// the marks the list lost go
	if name, _ := list.Name(0x040003C023FC4000); name != "LEGO Made-up: Legacy" {
		t.Errorf("marks: %q", name)
	}
	if len(names) != 2 {
		t.Errorf("names %v", names)
	}
	for _, broken := range []string{"", "[]", "{}", `{"titles":{}}`, "<html>"} {
		if _, err := parseNames([]byte(broken)); err == nil {
			t.Errorf("parseNames(%q) gives no error", broken)
		}
	}
	var none *Names
	if _, ok := none.Name(1); ok {
		t.Error("no list gives a name")
	}
}

// The list is downloaded once a day at most and kept, as versions.json.
func TestRefreshNames(t *testing.T) {
	asked := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		w.Header().Set("ETag", `"names"`)
		if r.Header.Get("If-None-Match") == `"names"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Write([]byte(namesSample))
	}))
	defer server.Close()
	old := NameAddresses
	NameAddresses = []string{server.URL + "/titles.switch2.json"}
	defer func() { NameAddresses = old }()

	folder := t.TempDir()
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	names, err := RefreshNames(context.Background(), folder, server.Client(), now)
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := names.Name(0x040074A01BF12000); !ok || name != "Made-up Bananza" || asked != 1 {
		t.Fatalf("%q %v, asked %d", name, ok, asked)
	}
	if _, err := os.Stat(filepath.Join(folder, NamesFile)); err != nil {
		t.Fatal(err)
	}
	// within a day, the copy kept; after it, asked whether it changed
	if names, err := RefreshNames(context.Background(), folder, server.Client(), now.Add(time.Hour)); err != nil || names == nil || asked != 1 {
		t.Errorf("within a day: %v, asked %d", err, asked)
	}
	if names, err := RefreshNames(context.Background(), folder, server.Client(), now.Add(25*time.Hour)); err != nil || names == nil || asked != 2 {
		t.Errorf("a day later: %v, asked %d", err, asked)
	}
	if kept, err := LoadNames(folder); err != nil || kept == nil {
		t.Errorf("load: %v", err)
	}
	if err := ForgetNames(folder); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadNames(folder); err == nil {
		t.Error("the copy is still there")
	}
}

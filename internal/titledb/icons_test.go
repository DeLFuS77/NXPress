package titledb

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

// titles is a titles.json of a few titles, as the data release has it: a game with its icon, an
// update and a DLC of it, a game without one, a DLC of a game that has none, and things that are
// not icons.
const titles = `{
  "0100000000000816": {"iconUrl": null, "id": "0100000000000816", "name": null},
  "0100000000010000": {"iconUrl": "https://img.example/game1.jpg", "id": "0100000000010000", "name": "Game One",
    "screenshots": ["https://img.example/s1.jpg"], "description": "Long text"},
  "0100000000010800": {"iconUrl": "https://img.example/update1.jpg", "id": "0100000000010800"},
  "0100000000011001": {"iconUrl": "https://img.example/dlc1.jpg", "id": "0100000000011001"},
  "0100000000020000": {"iconUrl": null, "id": "0100000000020000", "name": "No Icon"},
  "0100000000033001": {"iconUrl": "https://img.example/dlc3.jpg", "id": "0100000000033001"},
  "0100000000040000": {"iconUrl": "file:///etc/passwd", "id": "0100000000040000"},
  "bad": {"iconUrl": "https://img.example/bad.jpg", "id": "bad"}
}`

func compressed(t *testing.T, data string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer, _ := zstd.NewWriter(&out)
	writer.Write([]byte(data))
	writer.Close()
	return out.Bytes()
}

func TestRefreshIcons(t *testing.T) {
	asked := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		if r.Header.Get("If-None-Match") == `"one"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"one"`)
		w.Write(compressed(t, titles))
	}))
	defer server.Close()
	old := IconAddresses
	IconAddresses = []string{server.URL + "/titles.json.zst"}
	defer func() { IconAddresses = old }()

	folder := t.TempDir()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	icons, err := RefreshIcons(context.Background(), folder, server.Client(), now)
	if err != nil || icons == nil {
		t.Fatalf("%v %v", icons, err)
	}
	for id, want := range map[uint64]string{
		0x0100000000010000: "https://img.example/game1.jpg",
		// an update and a DLC take the icon of their game
		0x0100000000010800: "https://img.example/game1.jpg",
		0x0100000000011001: "https://img.example/game1.jpg",
		0x0100000000011005: "https://img.example/game1.jpg",
		// a game titledb has no icon of takes the one of its DLC
		0x0100000000032000: "https://img.example/dlc3.jpg",
	} {
		if got, ok := icons.URL(id); !ok || got != want {
			t.Errorf("%016X: %q %v", id, got, ok)
		}
	}
	for _, id := range []uint64{0x0100000000020000, 0x0100000000040000, 0x0100000000000816, 0x0100000000099000} {
		if got, ok := icons.URL(id); ok {
			t.Errorf("%016X: %q", id, got)
		}
	}
	// only the icons are kept, by title ID
	data, err := os.ReadFile(filepath.Join(folder, IconsFile))
	if err != nil || strings.Contains(string(data), "Long text") || !strings.Contains(string(data), `"0100000000010000":"https://img.example/game1.jpg"`) {
		t.Errorf("%s %v", data, err)
	}
	// within the week, the list kept is used; past it, the server is asked whether it changed
	if _, err := RefreshIcons(context.Background(), folder, server.Client(), now.Add(6*24*time.Hour)); err != nil || asked != 1 {
		t.Errorf("%d %v", asked, err)
	}
	if icons, err := RefreshIcons(context.Background(), folder, server.Client(), now.Add(8*24*time.Hour)); err != nil || asked != 2 || icons == nil {
		t.Errorf("%d %v", asked, err)
	}
	if kept, err := LoadIcons(folder); err != nil || kept == nil {
		t.Errorf("%v", err)
	}
	// with no connection, the list kept and why
	server.Close()
	if icons, err := RefreshIcons(context.Background(), folder, http.DefaultClient, now.Add(30*24*time.Hour)); err == nil || icons == nil {
		t.Errorf("offline: %v %v", icons, err)
	}
	if err := ForgetIcons(folder); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIcons(folder); err == nil {
		t.Error("forgotten list loads")
	}
	// with no list either: nil, and why
	if icons, err := RefreshIcons(context.Background(), folder, http.DefaultClient, now); err == nil || icons != nil {
		t.Errorf("no list: %v %v", icons, err)
	}
}

func TestRefreshIconsAsItIs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zst") {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(titles))
	}))
	defer server.Close()
	old := IconAddresses
	IconAddresses = []string{server.URL + "/titles.json.zst", server.URL + "/titles.json"}
	defer func() { IconAddresses = old }()
	icons, err := RefreshIcons(context.Background(), t.TempDir(), server.Client(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := icons.URL(0x0100000000010000); !ok || got != "https://img.example/game1.jpg" {
		t.Errorf("%q", got)
	}
}

func TestRefreshIconsRefusesWhatIsNoList(t *testing.T) {
	for name, body := range map[string]string{"array": `[1,2]`, "none": `{"0100000000010000": {"iconUrl": null}}`, "cut": titles[:200]} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		old := IconAddresses
		IconAddresses = []string{server.URL + "/titles.json"}
		folder := t.TempDir()
		if icons, err := RefreshIcons(context.Background(), folder, server.Client(), time.Now()); err == nil || icons != nil {
			t.Errorf("%s: %v %v", name, icons, err)
		}
		if _, err := os.Stat(filepath.Join(folder, IconsFile)); err == nil {
			t.Errorf("%s: kept", name)
		}
		IconAddresses = old
		server.Close()
	}
}

func TestDownloadIcon(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/icon.jpg":
			w.Write([]byte("\xFF\xD8\xFFicon"))
		case "/big.jpg":
			w.Write(make([]byte, maxIconDownload+10))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	if data, err := DownloadIcon(context.Background(), server.Client(), server.URL+"/icon.jpg"); err != nil || string(data) != "\xFF\xD8\xFFicon" {
		t.Errorf("%q %v", data, err)
	}
	for _, address := range []string{server.URL + "/gone.jpg", server.URL + "/big.jpg", "file:///etc/passwd", "not an address"} {
		if _, err := DownloadIcon(context.Background(), server.Client(), address); err == nil {
			t.Errorf("%s: downloaded", address)
		}
	}
}

func TestGameID(t *testing.T) {
	for id, want := range map[uint64]uint64{
		0x0100000000010000: 0x0100000000010000, 0x0100000000010800: 0x0100000000010000,
		0x0100000000011001: 0x0100000000010000, 0x01000000000110FF: 0x0100000000010000,
		0x0400486024DED001: 0x0400486024DEC000, 0x0400486024DEC800: 0x0400486024DEC000,
	} {
		if got := GameID(id); got != want {
			t.Errorf("%016X: %016X", id, got)
		}
	}
}

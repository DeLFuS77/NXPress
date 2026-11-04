package titledb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

// a versions.json as the library app publishes it: the versions of a game, and of a DLC
const sample = `{"0100a1b2c3d40000":{"65536":"2026-01-10","393216":"2026-10-21","262144":"2026-06-02"},` +
	`"0100a1b2c3d41001":{"65536":"2026-08-12"},"not an id":{"65536":"2026-01-01"},"0100e6f5d4c30000":{"x":"y","131072":"soon"}}`

// server stands in for the servers of versions.json: the file compressed at /versions.json.zst
// and as it is at /versions.json, each answering 304 to the names of its copy, and counting what
// it was asked.
type server struct {
	*httptest.Server
	mutex sync.Mutex
	asked []string
	// the paths that fail, as a server that is down
	failing map[string]bool
}

const modified = "Mon, 02 Nov 2026 15:05:55 GMT"

func newServer(t *testing.T) *server {
	t.Helper()
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	compressed := encoder.EncodeAll([]byte(sample), nil)
	s := &server{failing: map[string]bool{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mutex.Lock()
		s.asked = append(s.asked, r.URL.Path+" "+r.Header.Get("If-None-Match")+" "+r.Header.Get("If-Modified-Since"))
		failing := s.failing[r.URL.Path]
		s.mutex.Unlock()
		if failing {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		body, etag := []byte(sample), `"plain"`
		if strings.HasSuffix(r.URL.Path, ".zst") {
			body, etag = compressed, `"zst"`
		}
		// as GitHub, the answer that nothing changed gives the ETag again, not the day
		if r.Header.Get("If-None-Match") == etag {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", modified)
		w.Write(body)
	}))
	t.Cleanup(s.Close)
	old := Addresses
	Addresses = []string{s.URL + "/versions.json.zst", s.URL + "/versions.json"}
	t.Cleanup(func() { Addresses = old })
	return s
}

func (s *server) requests() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]string(nil), s.asked...)
}

// The newest version of each title, with its day; what is not a title ID or a version is left out.
func TestParse(t *testing.T) {
	newest, err := parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint64]Published{
		0x0100A1B2C3D40000: {393216, "2026-10-21"},
		0x0100A1B2C3D41001: {65536, "2026-08-12"},
		// a day that is not one is left out, not the version
		0x0100E6F5D4C30000: {131072, ""},
	}
	if len(newest) != len(want) {
		t.Fatalf("parse = %v, want %v", newest, want)
	}
	for id, published := range want {
		if newest[id] != published {
			t.Errorf("%016X = %v, want %v", id, newest[id], published)
		}
	}
	for _, broken := range []string{"", "[]", "{}", `{"0100a1b2c3d40000":{}}`, "<html>"} {
		if _, err := parse([]byte(broken)); err == nil {
			t.Errorf("parse(%q) gives no error", broken)
		}
	}
}

// The first time, the file compressed is downloaded and kept with what the server named it;
// within a day nothing is asked; after it, the server is asked whether it changed.
func TestRefresh(t *testing.T) {
	s := newServer(t)
	folder := t.TempDir()
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)

	versions, err := Refresh(context.Background(), folder, s.Client(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := versions.Newest(0x0100A1B2C3D40000); !ok || got.Version != 393216 || got.Date != "2026-10-21" {
		t.Fatalf("newest of the game = %v, %v", got, ok)
	}
	if got := versions.Date.Format(time.DateOnly); got != "2026-11-02" {
		t.Errorf("the list is of %s, want the day the server gives", got)
	}
	if data, err := os.ReadFile(filepath.Join(folder, FileName)); err != nil || string(data) != sample {
		t.Fatalf("the copy kept is %q, %v", data, err)
	}
	if asked := s.requests(); len(asked) != 1 || !strings.HasPrefix(asked[0], "/versions.json.zst  ") {
		t.Fatalf("asked %q, want the compressed file once, with nothing to compare", asked)
	}

	// within a day, the copy kept, without asking
	if versions, err = Refresh(context.Background(), folder, s.Client(), now.Add(23*time.Hour)); err != nil || versions == nil {
		t.Fatalf("within a day: %v, %v", versions, err)
	}
	if asked := s.requests(); len(asked) != 1 {
		t.Fatalf("asked %q within a day", asked)
	}

	// a day later, the server is asked whether its copy changed, and says no
	later := now.Add(25 * time.Hour)
	versions, err = Refresh(context.Background(), folder, s.Client(), later)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := versions.Newest(0x0100A1B2C3D41001); !ok {
		t.Fatal("the copy kept is not used once the server says it did not change")
	}
	asked := s.requests()
	if want := `/versions.json.zst "zst" ` + modified; len(asked) != 2 || asked[1] != want {
		t.Fatalf("asked %q, want %q", asked, want)
	}
	// and the copy is good for a day more
	if _, err := Refresh(context.Background(), folder, s.Client(), later.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if asked := s.requests(); len(asked) != 2 {
		t.Fatalf("asked %q, an hour after the server said its copy did not change", asked)
	}
	if kept := readState(folder); !kept.Checked.Equal(later) || kept.ETag != `"zst"` || kept.Modified != modified {
		t.Errorf("state kept = %+v", kept)
	}
	// the list keeps the day the server gave it
	if versions, err := Load(folder); err != nil || versions.Date.Format(time.DateOnly) != "2026-11-02" {
		t.Errorf("after the server said its copy did not change, the list is of %v, %v", versions.Date, err)
	}
}

// A clock put back makes the copy kept older, not newer: the server is asked again.
func TestRefreshClockBack(t *testing.T) {
	s := newServer(t)
	folder := t.TempDir()
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	if _, err := Refresh(context.Background(), folder, s.Client(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := Refresh(context.Background(), folder, s.Client(), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if asked := s.requests(); len(asked) != 2 {
		t.Fatalf("asked %q", asked)
	}
}

// When the file compressed cannot be had, the file as it is comes next.
func TestRefreshNextAddress(t *testing.T) {
	s := newServer(t)
	s.failing["/versions.json.zst"] = true
	folder := t.TempDir()
	versions, err := Refresh(context.Background(), folder, s.Client(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := versions.Newest(0x0100A1B2C3D40000); !ok {
		t.Fatal("no versions from the second address")
	}
	if kept := readState(folder); kept.Address != Addresses[1] || kept.ETag != `"plain"` {
		t.Errorf("state kept = %+v", kept)
	}
}

// With no connection, the copy kept is used and the error tells why; with no copy either, there
// are no versions. Nothing is written then.
func TestRefreshOffline(t *testing.T) {
	s := newServer(t)
	folder := t.TempDir()
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	if _, err := Refresh(context.Background(), folder, s.Client(), now); err != nil {
		t.Fatal(err)
	}
	s.Close()

	versions, err := Refresh(context.Background(), folder, s.Client(), now.Add(48*time.Hour))
	if err == nil {
		t.Error("no error with no server")
	}
	if _, ok := versions.Newest(0x0100A1B2C3D40000); !ok {
		t.Fatal("the copy kept is not used with no connection")
	}
	if kept := readState(folder); !kept.Checked.Equal(now) {
		t.Errorf("the state changed with no connection: %+v", kept)
	}

	empty := t.TempDir()
	if versions, err := Refresh(context.Background(), empty, s.Client(), now); versions != nil || err == nil {
		t.Fatalf("with no copy and no connection: %v, %v", versions, err)
	}
	if entries, _ := os.ReadDir(empty); len(entries) != 0 {
		t.Errorf("written with no connection: %v", entries)
	}
}

// A server that gives something else than versions.json leaves the copy kept as it is.
func TestRefreshBroken(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>not here</html>"))
	}))
	defer broken.Close()
	old := Addresses
	defer func() { Addresses = old }()
	Addresses = []string{broken.URL + "/versions.json.zst", broken.URL + "/versions.json"}
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, FileName), []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	versions, err := Refresh(context.Background(), folder, broken.Client(), time.Now())
	if err == nil || versions == nil {
		t.Fatalf("Refresh = %v, %v", versions, err)
	}
	if data, _ := os.ReadFile(filepath.Join(folder, FileName)); string(data) != sample {
		t.Errorf("the copy kept became %q", data)
	}
}

// Load reads the copy kept without asking anyone; Forget removes it.
func TestLoadForget(t *testing.T) {
	s := newServer(t)
	folder := t.TempDir()
	if _, err := Load(folder); err == nil {
		t.Fatal("Load gives versions with no copy")
	}
	if _, err := Refresh(context.Background(), folder, s.Client(), time.Now()); err != nil {
		t.Fatal(err)
	}
	versions, err := Load(folder)
	if err != nil {
		t.Fatal(err)
	}
	if got := versions.Date.Format(time.DateOnly); got != "2026-11-02" {
		t.Errorf("the list is of %s", got)
	}
	if asked := s.requests(); len(asked) != 1 {
		t.Errorf("Load asked the server: %q", asked)
	}
	if err := Forget(folder); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(folder); len(entries) != 0 {
		t.Errorf("left after Forget: %v", entries)
	}
	if err := Forget(folder); err != nil {
		t.Errorf("Forget twice: %v", err)
	}
	// no versions know nothing
	var none *Versions
	if _, ok := none.Newest(0x0100A1B2C3D40000); ok {
		t.Error("nil versions know a title")
	}
}

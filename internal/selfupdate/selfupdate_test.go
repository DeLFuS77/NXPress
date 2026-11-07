package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// a key of the tests, as the project has one
func testKey(t *testing.T) (ed25519.PrivateKey, PublicKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var id [8]byte
	rand.Read(id[:])
	return private, PublicKey{ID: id, Key: public}
}

// A signature of the key over the file, with its comment, passes; the file, the comment or the
// signature changed, or another key, do not.
func TestVerify(t *testing.T) {
	private, public := testKey(t)
	_, other := testKey(t)
	data := []byte("abc  nxpress.exe\n")
	signature := Sign(private, public.ID, data, "NXPress 1.7.4")
	if comment, err := Verify(data, signature, []PublicKey{other, public}); err != nil || comment != "NXPress 1.7.4" {
		t.Fatalf("good signature: %q %v", comment, err)
	}
	for what, check := range map[string]func() error{
		"another file": func() error {
			_, err := Verify([]byte("abd  nxpress.exe\n"), signature, []PublicKey{public})
			return err
		},
		"another key": func() error { _, err := Verify(data, signature, []PublicKey{other}); return err },
		"no key":      func() error { _, err := Verify(data, signature, nil); return err },
		"comment changed": func() error {
			_, err := Verify(data, bytes.Replace(signature, []byte("1.7.4"), []byte("1.7.5"), 1), []PublicKey{public})
			return err
		},
		"not a signature": func() error { _, err := Verify(data, []byte("hello"), []PublicKey{public}); return err },
	} {
		if err := check(); !errors.Is(err, ErrSignature) {
			t.Errorf("%s: %v", what, err)
		}
	}
	// the key as minisign writes it reads back
	keys, err := ParsePublicKeys("# keys\n" + public.Text())
	if err != nil || len(keys) != 1 || keys[0].ID != public.ID || !keys[0].Key.Equal(public.Key) {
		t.Fatalf("public key read back: %v %v", keys, err)
	}
	// and the private one the maintainer keeps
	again, id, err := ParsePrivateKey(PrivateKeyText(private, public.ID))
	if err != nil || id != public.ID || !again.Equal(private) {
		t.Fatalf("private key read back: %v", err)
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.7.4", "1.7.3", true}, {"1.7.3", "1.7.4", false}, {"1.7.4", "1.7.4", false}, {"1.10.0", "1.9.9", true},
		{"1.7.4", "1.7.4-beta.2", true}, {"1.7.4-beta.2", "1.7.4", false}, {"1.7.4-beta.3", "1.7.4-beta.2", true},
		{"1.7.5-beta.1", "1.7.4", true}, {"dev", "1.7.4", false}, {"1.7.4", "dev", false}, {"1.7.4-beta.0", "1.7.3", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
	if !IsBeta("1.7.4-beta.1") || IsBeta("1.7.4") {
		t.Error("IsBeta")
	}
}

// a release of the tests, served by a server of its own
type fakeRelease struct {
	files map[string][]byte
}

func (f *fakeRelease) serve(t *testing.T) (*httptest.Server, Release) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := f.files[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(server.Close)
	release := Release{Tag: "1.7.4", Assets: map[string]string{}}
	for name := range f.files {
		release.Assets[name] = server.URL + "/" + name
	}
	return server, release
}

func sums(files map[string][]byte) []byte {
	var out bytes.Buffer
	for name, data := range files {
		sum := sha256.Sum256(data)
		out.WriteString(hex.EncodeToString(sum[:]) + "  " + name + "\n")
	}
	return out.Bytes()
}

// a release of Windows signed with the key, for the version given
func signedRelease(private ed25519.PrivateKey, public PublicKey, program []byte, comment string) *fakeRelease {
	checksums := sums(map[string][]byte{"nxpress.exe": program, "nxpress-1.7.4-windows-x64-portable.zip": []byte("zip")})
	return &fakeRelease{files: map[string][]byte{
		"nxpress.exe":   program,
		SumsName:        checksums,
		SignatureName:   Sign(private, public.ID, checksums, comment),
		"release-notes": []byte("notes"),
	}}
}

// the program running, in a folder of the test
func running(t *testing.T) string {
	t.Helper()
	program := filepath.Join(t.TempDir(), "nxpress.exe")
	if err := os.WriteFile(program, []byte("the old NXPress"), 0o755); err != nil {
		t.Fatal(err)
	}
	return program
}

func installer(program string, keys []PublicKey) Installer {
	return Installer{Program: program, Version: "1.7.3", GOOS: "windows", GOARCH: "amd64", Keys: keys, Allowed: func(u *url.URL) bool { return u.Hostname() == "127.0.0.1" }}
}

// A release signed for its version is installed: the program takes the place of the running
// one, which waits beside it until CleanOld, with nothing else left.
func TestInstall(t *testing.T) {
	private, public := testKey(t)
	_, release := signedRelease(private, public, []byte("the new NXPress"), "NXPress 1.7.4").serve(t)
	program := running(t)
	steps := []string{}
	in := installer(program, []PublicKey{public})
	in.Progress = func(step string, done, total int64) {
		if len(steps) == 0 || steps[len(steps)-1] != step {
			steps = append(steps, step)
		}
	}
	if !Installable(release, in.Keys, "windows", "amd64") {
		t.Fatal("not installable")
	}
	if err := in.Install(context.Background(), release); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(program); string(data) != "the new NXPress" {
		t.Fatalf("program: %q", data)
	}
	if strings.Join(steps, " ") != "download check replace" {
		t.Errorf("steps: %v", steps)
	}
	CleanOld(program)
	if entries, _ := os.ReadDir(filepath.Dir(program)); len(entries) != 1 {
		t.Errorf("left beside it: %v", entries)
	}
}

// A file changed on the way, a signature of another key, checksums signed for another version,
// a release without signature, a beta for a stable NXPress or an older version: each is refused,
// and the program stays as it was, with nothing left beside it.
func TestInstallRefuses(t *testing.T) {
	private, public := testKey(t)
	otherPrivate, other := testKey(t)
	cases := map[string]struct {
		release func(t *testing.T) Release
		want    error
	}{
		"a file changed on the way": {func(t *testing.T) Release {
			f := signedRelease(private, public, []byte("the new NXPress"), "NXPress 1.7.4")
			f.files["nxpress.exe"] = []byte("the new NXPress, with something added")
			_, r := f.serve(t)
			return r
		}, ErrChecksum},
		"a signature of another key": {func(t *testing.T) Release {
			f := signedRelease(otherPrivate, other, []byte("the new NXPress"), "NXPress 1.7.4")
			_, r := f.serve(t)
			return r
		}, ErrSignature},
		"checksums signed for another version": {func(t *testing.T) Release {
			_, r := signedRelease(private, public, []byte("the new NXPress"), "NXPress 1.7.3").serve(t)
			return r
		}, ErrSignature},
		"no signature": {func(t *testing.T) Release {
			f := signedRelease(private, public, []byte("the new NXPress"), "NXPress 1.7.4")
			delete(f.files, SignatureName)
			_, r := f.serve(t)
			return r
		}, ErrUnsigned},
		"a beta": {func(t *testing.T) Release {
			_, r := signedRelease(private, public, []byte("the new NXPress"), "NXPress 1.7.4").serve(t)
			r.Prerelease = true
			return r
		}, ErrPrerelease},
		"an older version": {func(t *testing.T) Release {
			_, r := signedRelease(private, public, []byte("the new NXPress"), "NXPress 1.7.4").serve(t)
			r.Tag = "1.7.2"
			return r
		}, ErrNotNewer},
	}
	for what, c := range cases {
		program := running(t)
		err := installer(program, []PublicKey{public}).Install(context.Background(), c.release(t))
		if !errors.Is(err, c.want) {
			t.Errorf("%s: %v", what, err)
		}
		if data, _ := os.ReadFile(program); string(data) != "the old NXPress" {
			t.Errorf("%s: the program changed", what)
		}
		if entries, _ := os.ReadDir(filepath.Dir(program)); len(entries) != 1 {
			t.Errorf("%s: left beside it: %v", what, entries)
		}
	}
}

// A beta NXPress takes a beta.
func TestBetaTakesABeta(t *testing.T) {
	private, public := testKey(t)
	_, release := signedRelease(private, public, []byte("the new beta"), "NXPress 1.7.4").serve(t)
	release.Prerelease = true
	program := running(t)
	in := installer(program, []PublicKey{public})
	in.Version = "1.7.4-beta.1"
	if err := in.Install(context.Background(), release); err != nil {
		t.Fatal(err)
	}
}

// On Linux the program comes out of its archive, executable.
func TestInstallLinux(t *testing.T) {
	private, public := testKey(t)
	var archive bytes.Buffer
	zipped := gzip.NewWriter(&archive)
	writer := tar.NewWriter(zipped)
	for _, file := range []struct{ name, data string }{{"NXPress/install.sh", "#!/bin/sh"}, {"NXPress/nxpress", "the new NXPress of Linux"}} {
		writer.WriteHeader(&tar.Header{Name: file.name, Mode: 0o755, Size: int64(len(file.data)), Typeflag: tar.TypeReg})
		writer.Write([]byte(file.data))
	}
	writer.Close()
	zipped.Close()
	name := "nxpress-1.7.4-linux-x64.tar.gz"
	checksums := sums(map[string][]byte{name: archive.Bytes()})
	f := &fakeRelease{files: map[string][]byte{name: archive.Bytes(), SumsName: checksums, SignatureName: Sign(private, public.ID, checksums, "NXPress 1.7.4")}}
	_, release := f.serve(t)
	program := filepath.Join(t.TempDir(), "nxpress")
	os.WriteFile(program, []byte("the old NXPress"), 0o755)
	in := installer(program, []PublicKey{public})
	in.GOOS = "linux"
	if err := in.Install(context.Background(), release); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(program); string(data) != "the new NXPress of Linux" {
		t.Fatalf("program: %q", data)
	}
	CleanOld(program)
	if entries, _ := os.ReadDir(filepath.Dir(program)); len(entries) != 1 {
		t.Errorf("left beside it: %v", entries)
	}
}

// A download sent away from GitHub is not followed, and an address that is not of GitHub is not
// asked.
func TestOnlyGitHub(t *testing.T) {
	if !fromGitHub(mustURL("https://github.com/DeLFuS77/NXPress/releases/download/1.7.4/nxpress.exe")) ||
		!fromGitHub(mustURL("https://objects.githubusercontent.com/x")) ||
		fromGitHub(mustURL("http://github.com/x")) || fromGitHub(mustURL("https://github.com.evil.example/x")) {
		t.Fatal("fromGitHub")
	}
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("elsewhere")) }))
	defer elsewhere.Close()
	away := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(elsewhere.URL, "127.0.0.1", "localhost", 1)+"/x", http.StatusFound)
	}))
	defer away.Close()
	in := installer(running(t), nil)
	if _, err := in.get(context.Background(), away.URL+"/x", 100); err == nil {
		t.Error("followed a download away")
	}
	if _, err := in.get(context.Background(), "https://example.com/x", 100); err == nil {
		t.Error("asked an address that is not of GitHub")
	}
}

func mustURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

func TestCode(t *testing.T) {
	for err, want := range map[error]string{
		ErrSignature: "signature", ErrUnsigned: "unsigned", ErrChecksum: "checksum", ErrNoWrite: "write",
		ErrUnsupported: "unsupported", ErrNotNewer: "version", ErrPrerelease: "beta", context.Canceled: "cancelled",
		errors.New("other"): "failed",
	} {
		if got := Code(err); got != want {
			t.Errorf("%v: %s", err, got)
		}
	}
	if AssetName("darwin", "arm64", "1.7.4") != "" || AssetName("windows", "amd64", "1.7.4") != "nxpress.exe" {
		t.Error("AssetName")
	}
}

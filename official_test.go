package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/selfupdate"
)

func hexSum(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

// A program is official when its checksum is in the signed checksums of its version, on its own
// (nxpress.exe) or in the list of the programs inside the archives; any other program of a
// version published, or a version never published, is not. Without an answer it is not known.
func TestOfficial(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := selfupdate.PublicKey{ID: [8]byte{1, 2, 3}, Key: public}
	programs := fmt.Sprintf("%s  linux-x64\n%s  macos-apple-silicon\n", hexSum("linux program"), hexSum("mac program"))
	sums := fmt.Sprintf("%s  nxpress.exe\n%s  %s\n", hexSum("windows program"), hexSum(programs), programsName("1.7.17"))
	files := map[string]string{
		"1.7.17/" + selfupdate.SumsName:      sums,
		"1.7.17/" + selfupdate.SignatureName: string(selfupdate.Sign(private, key.ID, []byte(sums), "NXPress 1.7.17")),
		"1.7.17/" + programsName("1.7.17"):   programs,
	}
	get := func(ctx context.Context, address string) ([]byte, error) {
		if data, ok := files[address[len(officialDownloads):]]; ok {
			return []byte(data), nil
		}
		return nil, errNotFound
	}
	program := func(data string) func() (string, error) {
		return func() (string, error) { return hexSum(data), nil }
	}
	keys := []selfupdate.PublicKey{key}
	for _, c := range []struct {
		tag, program, want string
	}{
		{"1.7.17", "windows program", "official"},
		{"1.7.17", "linux program", "official"},
		{"1.7.17", "mac program", "official"},
		{"1.7.17", "a program built elsewhere", "unofficial"},
		{"1.7.99", "windows program", "unofficial"},
		{"dev-1234567", "windows program", "dev"},
	} {
		if got := checkOfficial(context.Background(), c.tag, program(c.program), keys, get); got.State != c.want {
			t.Errorf("%s %q: %s, want %s", c.tag, c.program, got.State, c.want)
		}
	}
	// checksums not signed with the key of NXPress, or no answer: not known
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if got := checkOfficial(context.Background(), "1.7.17", program("windows program"), []selfupdate.PublicKey{{ID: key.ID, Key: other}}, get); got.State != "unknown" {
		t.Errorf("another key: %s", got.State)
	}
	offline := func(context.Context, string) ([]byte, error) { return nil, fmt.Errorf("offline") }
	if got := checkOfficial(context.Background(), "1.7.17", program("windows program"), keys, offline); got.State != "unknown" {
		t.Errorf("offline: %s", got.State)
	}
	// a list of programs that does not match its signed checksum is not trusted
	files["1.7.17/"+programsName("1.7.17")] = programs + hexSum("a program built elsewhere") + "  linux-x64\n"
	if got := checkOfficial(context.Background(), "1.7.17", program("a program built elsewhere"), keys, get); got.State != "unknown" {
		t.Errorf("a list changed: %s", got.State)
	}
	// a release before the list of programs: Windows is known by nxpress.exe, the others not
	files["1.7.16/"+selfupdate.SumsName] = fmt.Sprintf("%s  nxpress.exe\n", hexSum("windows 1.7.16"))
	files["1.7.16/"+selfupdate.SignatureName] = string(selfupdate.Sign(private, key.ID, []byte(files["1.7.16/"+selfupdate.SumsName]), "NXPress 1.7.16"))
	want := "unknown"
	if runtime.GOOS == "windows" {
		want = "unofficial"
	}
	if got := checkOfficial(context.Background(), "1.7.16", program("another"), keys, get); got.State != want {
		t.Errorf("before the list: %s, want %s", got.State, want)
	}
}

// A program built from the code without the tag "official" does not carry the name and the logo of
// NXPress (TRADEMARKS.md): its window says it is an unofficial build, its icon is the plain one,
// and the page is told so.
func TestBuildWithoutTheOfficialTag(t *testing.T) {
	if officialBuild {
		t.Skip("built with the tag official")
	}
	if brandName != "NXPress (unofficial build)" {
		t.Errorf("name: %q", brandName)
	}
	plain, err := os.ReadFile(filepath.Join("build", "unofficial.png"))
	if err != nil || !bytes.Equal(appIcon, plain) {
		t.Errorf("the icon is not the plain one: %v", err)
	}
	if (&App{}).Setup().OfficialBuild {
		t.Error("the page is told it is an official build")
	}
}

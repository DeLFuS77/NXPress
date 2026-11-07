package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/selfupdate"
)

// The private key goes to the backup only, never on the screen nor anywhere else; the public key
// to release.pub, which a second run does not replace without -force.
func TestRun(t *testing.T) {
	repository := t.TempDir()
	os.MkdirAll(filepath.Join(repository, "internal", "selfupdate"), 0o755)
	keysFile := filepath.Join(repository, "internal", "selfupdate", "release.pub")
	os.WriteFile(keysFile, []byte("# no key yet\n"), 0o644)
	here, _ := os.Getwd()
	os.Chdir(repository)
	defer os.Chdir(here)

	// without a backup, the key would be lost: nothing is made
	if err := run("", false, false); err == nil || !strings.Contains(err.Error(), "-backup") {
		t.Fatalf("no backup: %v", err)
	}
	if data, _ := os.ReadFile(keysFile); string(data) != "# no key yet\n" {
		t.Fatal("release.pub was written without a backup")
	}

	backup := filepath.Join(t.TempDir(), "nxpress-signing.key")
	stdout := os.Stdout
	read, write, _ := os.Pipe()
	os.Stdout = write
	err := run(backup, false, false)
	write.Close()
	os.Stdout = stdout
	printed := make([]byte, 4096)
	n, _ := read.Read(printed)
	if err != nil {
		t.Fatal(err)
	}
	kept, _ := os.ReadFile(backup)
	secret := strings.TrimSpace(string(kept))
	if secret == "" || strings.Contains(string(printed[:n]), secret) {
		t.Fatalf("the key was not kept, or was printed: %q", printed[:n])
	}
	if entries, _ := os.ReadDir(repository); len(entries) != 1 {
		t.Errorf("left in the repository: %v", entries)
	}
	private, id, err := selfupdate.ParsePrivateKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(keysFile)
	keys, err := selfupdate.ParsePublicKeys(string(data))
	if err != nil || len(keys) != 1 || keys[0].ID != id {
		t.Fatalf("release.pub: %v %v", keys, err)
	}
	signature := selfupdate.Sign(private, id, []byte("sums"), "NXPress 1.7.4")
	if _, err := selfupdate.Verify([]byte("sums"), signature, keys); err != nil {
		t.Fatalf("the public key is not the one kept: %v", err)
	}
	if err := run(filepath.Join(t.TempDir(), "another.key"), false, false); err == nil || !strings.Contains(err.Error(), "-force") {
		t.Fatalf("a second key replaced the first without -force: %v", err)
	}
}

// -add makes a spare key beside the one NXPress has: release.pub keeps both, a release signed
// with either passes, and the first key is left as it was.
func TestRunAddsASpareKey(t *testing.T) {
	repository := t.TempDir()
	os.MkdirAll(filepath.Join(repository, "internal", "selfupdate"), 0o755)
	keysFile := filepath.Join(repository, "internal", "selfupdate", "release.pub")
	os.WriteFile(keysFile, []byte("# no key yet\n"), 0o644)
	here, _ := os.Getwd()
	os.Chdir(repository)
	defer os.Chdir(here)
	quiet := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = quiet }()

	if err := run(filepath.Join(t.TempDir(), "spare.key"), false, true); err == nil || !strings.Contains(err.Error(), "no key yet") {
		t.Fatalf("a spare key without a first one: %v", err)
	}
	firstBackup := filepath.Join(t.TempDir(), "first.key")
	if err := run(firstBackup, false, false); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(keysFile)
	if err := run(filepath.Join(t.TempDir(), "spare.key"), true, true); err == nil {
		t.Fatal("-add and -force together")
	}
	spareBackup := filepath.Join(t.TempDir(), "spare.key")
	if err := run(spareBackup, false, true); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(keysFile)
	if !strings.HasPrefix(string(after), strings.TrimRight(string(before), "\n")) {
		t.Fatalf("the first key was not kept:\n%s", after)
	}
	keys, err := selfupdate.ParsePublicKeys(string(after))
	if err != nil || len(keys) != 2 || keys[0].ID == keys[1].ID {
		t.Fatalf("release.pub: %v %v", keys, err)
	}
	for _, backup := range []string{firstBackup, spareBackup} {
		kept, _ := os.ReadFile(backup)
		private, id, err := selfupdate.ParsePrivateKey(string(kept))
		if err != nil {
			t.Fatal(err)
		}
		signature := selfupdate.Sign(private, id, []byte("sums"), "NXPress 1.7.4")
		if _, err := selfupdate.Verify([]byte("sums"), signature, keys); err != nil {
			t.Errorf("a release signed with %s does not pass: %v", filepath.Base(backup), err)
		}
	}
}

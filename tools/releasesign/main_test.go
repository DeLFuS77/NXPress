package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/selfupdate"
)

// The checksums are signed for their version with the key of the secret when it is one of the
// keys of NXPress; without the key, or with another, nothing is signed and the release goes on.
func TestSign(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	var id [8]byte
	rand.Read(id[:])
	key := selfupdate.PublicKey{ID: id, Key: public}
	otherPublic, _, _ := ed25519.GenerateKey(rand.Reader)
	other := selfupdate.PublicKey{ID: id, Key: otherPublic}
	secret := selfupdate.PrivateKeyText(private, id)
	sums := filepath.Join(t.TempDir(), "SHA256SUMS.txt")
	os.WriteFile(sums, []byte("0123  nxpress.exe\n"), 0o644)

	for what, c := range map[string]struct {
		secret string
		keys   []selfupdate.PublicKey
		signed bool
	}{
		"the key of NXPress":          {secret, []selfupdate.PublicKey{key}, true},
		"no key":                      {"", []selfupdate.PublicKey{key}, false},
		"not a key":                   {"hello", []selfupdate.PublicKey{key}, false},
		"a key NXPress does not have": {secret, []selfupdate.PublicKey{other}, false},
	} {
		os.Remove(sums + ".minisig")
		note := sign(c.secret, "1.7.4", sums, c.keys)
		signature, err := os.ReadFile(sums + ".minisig")
		if c.signed != (err == nil) || c.signed != (note == "") {
			t.Errorf("%s: note %q, signature %v", what, note, err == nil)
			continue
		}
		if strings.Contains(note, secret) {
			t.Errorf("%s: the note tells the key", what)
		}
		if c.signed {
			data, _ := os.ReadFile(sums)
			if comment, err := selfupdate.Verify(data, signature, c.keys); err != nil || comment != "NXPress 1.7.4" {
				t.Errorf("%s: %q %v", what, comment, err)
			}
		}
	}
}

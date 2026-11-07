package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/selfupdate"
)

const (
	testCommit = "1111111111111111111111111111111111111111"
	testTagObj = "2222222222222222222222222222222222222222"
)

// fakeRelease is a draft on GitHub as the workflow leaves it, and the gh and git that see it.
type fakeRelease struct {
	files      map[string][]byte
	draft      bool
	localTag   string
	remoteTag  string
	unattested string
	attested   [][]string
	uploaded   []byte
	published  bool
	keyAsked   bool
}

func newFakeRelease() *fakeRelease {
	f := &fakeRelease{draft: true, localTag: testCommit, remoteTag: testCommit, files: map[string][]byte{
		"nxpress.exe":                    []byte("the program of Windows"),
		"nxpress-1.7.4-linux-x64.tar.gz": []byte("the program of Linux"),
	}}
	sums := ""
	for _, name := range []string{"nxpress.exe", "nxpress-1.7.4-linux-x64.tar.gz"} {
		sum := sha256.Sum256(f.files[name])
		sums += hex.EncodeToString(sum[:]) + "  " + name + "\n"
	}
	f.files[sumsName] = []byte(sums)
	f.files["nxpress-1.7.4.intoto.jsonl"] = []byte("{}")
	return f
}

func (f *fakeRelease) tools(secret string, yes bool, out *bytes.Buffer) tools {
	return tools{
		out: out,
		git: func(args ...string) ([]byte, error) {
			switch args[0] {
			case "rev-parse":
				if f.localTag == "" {
					return nil, errors.New("no tag")
				}
				return []byte(f.localTag + "\n"), nil
			case "ls-remote":
				return []byte(fmt.Sprintf("%s\trefs/tags/1.7.4\n%s\trefs/tags/1.7.4^{}\n", testTagObj, f.remoteTag)), nil
			}
			return nil, fmt.Errorf("git %v", args)
		},
		gh: func(args ...string) ([]byte, error) {
			switch strings.Join(args[:2], " ") {
			case "release view":
				r := map[string]any{"isDraft": f.draft}
				assets := []map[string]string{}
				for name := range f.files {
					assets = append(assets, map[string]string{"name": name})
				}
				r["assets"] = assets
				return json.Marshal(r)
			case "release download":
				folder := args[slices.Index(args, "-D")+1]
				for name, data := range f.files {
					os.WriteFile(filepath.Join(folder, name), data, 0o644)
				}
				return nil, nil
			case "attestation verify":
				f.attested = append(f.attested, args)
				if filepath.Base(args[2]) == f.unattested {
					return nil, errors.New("no attestation")
				}
				return nil, nil
			case "release upload":
				data, err := os.ReadFile(args[3])
				f.uploaded = data
				return nil, err
			case "release edit":
				f.published = true
				return nil, nil
			}
			return nil, fmt.Errorf("gh %v", args)
		},
		readKey: func(string) (string, error) {
			f.keyAsked = true
			return secret, nil
		},
		confirm: func(string) bool { return yes },
	}
}

func testKey() (string, selfupdate.PublicKey) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	var id [8]byte
	rand.Read(id[:])
	return selfupdate.PrivateKeyText(private, id), selfupdate.PublicKey{ID: id, Key: public}
}

var draft = options{repo: defaultRepo, workflow: defaultWorkflow, version: "1.7.4"}

// A draft whose files match their checksums and were built from the commit of the tag is signed
// for its version with the key of NXPress, and published once told so; the key is never shown.
func TestSignADraft(t *testing.T) {
	secret, key := testKey()
	for _, yes := range []bool{true, false} {
		f := newFakeRelease()
		var out bytes.Buffer
		if err := run(f.tools(secret, yes, &out), draft, []selfupdate.PublicKey{key}); err != nil {
			t.Fatal(err)
		}
		comment, err := selfupdate.Verify(f.files[sumsName], f.uploaded, []selfupdate.PublicKey{key})
		if err != nil || comment != "NXPress 1.7.4" {
			t.Fatalf("signature: %q %v", comment, err)
		}
		if f.published != yes {
			t.Errorf("published %v, told %v", f.published, yes)
		}
		if len(f.attested) != 2 {
			t.Fatalf("%d files checked by their provenance", len(f.attested))
		}
		for _, want := range []string{"--signer-workflow", defaultRepo + "/" + defaultWorkflow, "--source-ref", "refs/tags/1.7.4",
			"--source-digest", testCommit, "--deny-self-hosted-runners"} {
			if !slices.Contains(f.attested[0], want) {
				t.Errorf("the provenance is not checked with %s: %v", want, f.attested[0])
			}
		}
		if strings.Contains(out.String(), secret) {
			t.Error("the key was printed")
		}
	}
}

// Anything that does not hold stops before the key is asked, or before the signature goes in.
func TestSignRefuses(t *testing.T) {
	secret, key := testKey()
	other, _ := testKey()
	keys := []selfupdate.PublicKey{key}
	for what, c := range map[string]struct {
		change   func(f *fakeRelease, o *options, keys *[]selfupdate.PublicKey, secret *string)
		want     string
		keyAsked bool
	}{
		"a file changed after its checksum": {func(f *fakeRelease, _ *options, _ *[]selfupdate.PublicKey, _ *string) {
			f.files["nxpress.exe"] = []byte("something else")
		}, "does not match its checksum", false},
		"a file not in the checksums": {func(f *fakeRelease, _ *options, _ *[]selfupdate.PublicKey, _ *string) {
			f.files["nxpress-1.7.4-macos-apple-silicon.tar.gz"] = []byte("added")
		}, "not in SHA256SUMS.txt", false},
		"a checksum of a file the release lacks": {func(f *fakeRelease, _ *options, _ *[]selfupdate.PublicKey, _ *string) {
			delete(f.files, "nxpress-1.7.4-linux-x64.tar.gz")
		}, "which the release does not have", false},
		"a file not built by the workflow from the tag": {func(f *fakeRelease, _ *options, _ *[]selfupdate.PublicKey, _ *string) {
			f.unattested = "nxpress-1.7.4-linux-x64.tar.gz"
		}, "was not built by", false},
		"no tag here": {func(f *fakeRelease, _ *options, _ *[]selfupdate.PublicKey, _ *string) {
			f.localTag = ""
		}, "is not in this repository", false},
		"another tag on GitHub": {func(f *fakeRelease, _ *options, _ *[]selfupdate.PublicKey, _ *string) {
			f.remoteTag = "3333333333333333333333333333333333333333"
		}, "is not the one of this repository", false},
		"a release published already": {func(f *fakeRelease, _ *options, _ *[]selfupdate.PublicKey, _ *string) {
			f.draft = false
		}, "is published already", false},
		"a release signed already": {func(f *fakeRelease, _ *options, _ *[]selfupdate.PublicKey, _ *string) {
			f.files[signatureName] = []byte("old")
		}, "has a signature already", false},
		"not a version": {func(_ *fakeRelease, o *options, _ *[]selfupdate.PublicKey, _ *string) {
			o.version = "latest"
		}, "is not the tag of a version", false},
		"an NXPress without keys": {func(_ *fakeRelease, _ *options, k *[]selfupdate.PublicKey, _ *string) {
			*k = nil
		}, "has no public key", false},
		"not a key": {func(_ *fakeRelease, _ *options, _ *[]selfupdate.PublicKey, s *string) {
			*s = "hello"
		}, "not a private key", true},
		"a key NXPress is not built with": {func(_ *fakeRelease, _ *options, _ *[]selfupdate.PublicKey, s *string) {
			*s = other
		}, "not the one NXPress is built with", true},
	} {
		f := newFakeRelease()
		o, k, s := draft, keys, secret
		c.change(f, &o, &k, &s)
		var out bytes.Buffer
		err := run(f.tools(s, true, &out), o, k)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", what, err)
		}
		if f.uploaded != nil || f.published {
			t.Errorf("%s: signed or published", what)
		}
		if f.keyAsked != c.keyAsked {
			t.Errorf("%s: key asked %v", what, f.keyAsked)
		}
		if strings.Contains(out.String(), s) || err != nil && strings.Contains(err.Error(), s) {
			t.Errorf("%s: the key was printed", what)
		}
	}
}

// A release published before it was signed, or one signed again, when told so.
func TestSignAPublishedRelease(t *testing.T) {
	secret, key := testKey()
	f := newFakeRelease()
	f.draft = false
	f.files[signatureName] = []byte("old")
	o := draft
	o.published, o.replace = true, true
	var out bytes.Buffer
	if err := run(f.tools(secret, true, &out), o, []selfupdate.PublicKey{key}); err != nil {
		t.Fatal(err)
	}
	if _, err := selfupdate.Verify(f.files[sumsName], f.uploaded, []selfupdate.PublicKey{key}); err != nil || f.published {
		t.Errorf("signature %v, published again %v", err, f.published)
	}
}

func TestPeeled(t *testing.T) {
	for listing, want := range map[string]string{
		"aaa\trefs/tags/1.7.4\nbbb\trefs/tags/1.7.4^{}\n": "bbb",
		"aaa\trefs/tags/1.7.4\n":                          "aaa",
		"":                                                "",
		"ccc\trefs/tags/1.7.40\n":                         "",
	} {
		if got := peeled(listing, "refs/tags/1.7.4"); got != want {
			t.Errorf("%q: %q, not %q", listing, got, want)
		}
	}
}

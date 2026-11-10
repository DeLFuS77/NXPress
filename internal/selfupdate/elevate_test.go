package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// An NXPress in Program Files downloads the release to the temporary folder of the user, checks
// it there, and gives it to Elevate with its sum: nothing is written beside the program by the
// user's own NXPress.
func TestInstallElevated(t *testing.T) {
	private, public := testKey(t)
	_, release := signedRelease(private, public, []byte("the new NXPress"), "NXPress 1.7.4").serve(t)
	program := running(t)
	in := installer(program, []PublicKey{public})
	var given, sum string
	in.Elevate = func(fresh, s string) error {
		if filepath.Dir(filepath.Dir(fresh)) != filepath.Clean(os.TempDir()) {
			t.Errorf("downloaded to %s, not to the temporary folder", fresh)
		}
		data, err := os.ReadFile(fresh)
		if err != nil {
			t.Fatal(err)
		}
		given, sum = string(data), s
		return ReplaceChecked(program, fresh, s)
	}
	if err := in.Install(context.Background(), release); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte("the new NXPress"))
	if given != "the new NXPress" || sum != hex.EncodeToString(want[:]) {
		t.Fatalf("Elevate was given %q with %s", given, sum)
	}
	if data, _ := os.ReadFile(program); string(data) != "the new NXPress" {
		t.Fatalf("program: %q", data)
	}
}

// The answer No to Windows is told as such, and nothing is replaced.
func TestInstallElevatedDeclined(t *testing.T) {
	private, public := testKey(t)
	_, release := signedRelease(private, public, []byte("the new NXPress"), "NXPress 1.7.4").serve(t)
	program := running(t)
	in := installer(program, []PublicKey{public})
	in.Elevate = func(fresh, sum string) error { return ErrDeclined }
	err := in.Install(context.Background(), release)
	if !errors.Is(err, ErrDeclined) || Code(err) != "declined" {
		t.Fatalf("err: %v (%s)", err, Code(err))
	}
	if data, _ := os.ReadFile(program); string(data) != "the old NXPress" {
		t.Fatalf("program: %q", data)
	}
}

// ReplaceChecked puts the program in place only when it has the sum given: one changed after the
// check, by another program of the user, is refused, and the program and its folder stay as they
// were.
func TestReplaceChecked(t *testing.T) {
	program := running(t)
	fresh := filepath.Join(t.TempDir(), "fresh")
	os.WriteFile(fresh, []byte("the new NXPress"), 0o644)
	good := sha256.Sum256([]byte("the new NXPress"))

	other := sha256.Sum256([]byte("something else"))
	if err := ReplaceChecked(program, fresh, hex.EncodeToString(other[:])); !errors.Is(err, ErrChecksum) {
		t.Fatalf("a changed program: %v", err)
	}
	if data, _ := os.ReadFile(program); string(data) != "the old NXPress" {
		t.Fatalf("program after a refusal: %q", data)
	}
	if entries, _ := os.ReadDir(filepath.Dir(program)); len(entries) != 1 {
		t.Errorf("left beside it: %v", entries)
	}

	if err := ReplaceChecked(program, fresh, hex.EncodeToString(good[:])); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(program); string(data) != "the new NXPress" {
		t.Fatalf("program: %q", data)
	}
	CleanOld(program)
	if entries, _ := os.ReadDir(filepath.Dir(program)); len(entries) != 1 {
		t.Errorf("left beside it: %v", entries)
	}
	if err := ReplaceChecked(program, filepath.Join(t.TempDir(), "gone"), hex.EncodeToString(good[:])); err == nil {
		t.Error("a program not there was taken")
	}
}

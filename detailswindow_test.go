package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// what a window of details shows comes by a file in the temporary folder, which it reads once
func TestDetailsFile(t *testing.T) {
	want := DetailsData{Title: "Kirby · NXPress", Text: "line 1\nline 2 · ünï\t", Copy: "Copy", Copied: "Copied", Export: "Export", Saved: "Saved as {name}", FileName: "Kirby"}
	path, err := writeDetails(want)
	if err != nil {
		t.Fatal(err)
	}
	if !detailsPath(path) {
		t.Fatalf("%s is not taken as a file of details", path)
	}
	got, err := readDetails(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("read %+v, wrote %+v", got, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the file was not removed once read")
		os.Remove(path)
	}
}

// a window of details takes no other file than the ones made for it
func TestDetailsFileRefusals(t *testing.T) {
	// in another folder, with the right name
	other := filepath.Join(t.TempDir(), detailsPrefix+"x.json")
	if err := os.WriteFile(other, []byte(`{"text":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readDetails(other); err == nil {
		t.Error("a file in another folder was read")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("a file that is not one of details was removed")
	}
	// in the temporary folder, with another name
	plain := filepath.Join(os.TempDir(), "nxpress-test-not-details.json")
	if err := os.WriteFile(plain, []byte(`{"text":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(plain)
	if _, err := readDetails(plain); err == nil {
		t.Error("a file of another name was read")
	}
	if detailsPath("") || detailsPath("..") {
		t.Error("an empty path or a folder is taken as a file of details")
	}
	// too long for a window
	if _, err := writeDetails(DetailsData{Text: strings.Repeat("x", maxDetails+1)}); err == nil {
		t.Error("details longer than a window takes were written")
	}
}

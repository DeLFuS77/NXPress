package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The sound chosen for the end of the work reaches the page whole, as a data URL of its kind;
// what is not a sound, or is too long to send, is refused.
func TestSound(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "done.WAV")
	content := []byte("RIFF\x24\x00\x00\x00WAVEfmt ")
	if err := os.WriteFile(wav, content, 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	url, err := app.Sound(wav)
	if err != nil {
		t.Fatal(err)
	}
	if want := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(content); url != want {
		t.Errorf("Sound = %q, want %q", url, want)
	}

	text := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(text, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Sound(text); err == nil {
		t.Error("a text file was taken as a sound")
	}

	big := filepath.Join(dir, "long.mp3")
	if err := os.WriteFile(big, make([]byte, maxSoundSize+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Sound(big); err == nil || !strings.Contains(err.Error(), "bigger") {
		t.Errorf("a sound over the size was sent: %v", err)
	}

	if _, err := app.Sound(filepath.Join(dir, "gone.ogg")); err == nil {
		t.Error("a missing sound gave no error")
	}
}

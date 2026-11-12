package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An image chosen in Combine is read and made the icon it will be, with its name and size; one
// that cannot be an icon is told at once.
func TestCoverImage(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for y := range 480 {
		for x := range 640 {
			picture.Set(x, y, color.RGBA{200, 30, 30, 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, picture); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cover.png")
	os.WriteFile(path, data.Bytes(), 0o644)
	cover, err := coverImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if cover.Path != path || cover.Name != "cover.png" || cover.Width != 640 || cover.Height != 480 {
		t.Errorf("%+v", *cover)
	}
	icon, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(cover.Preview, "data:image/jpeg;base64,"))
	if err != nil || !strings.HasPrefix(cover.Preview, "data:image/jpeg;base64,") {
		t.Fatalf("preview %.40q %v", cover.Preview, err)
	}
	if config, _, err := image.DecodeConfig(bytes.NewReader(icon)); err != nil || config.Width != 256 || config.Height != 256 {
		t.Errorf("preview %+v %v", config, err)
	}

	text := filepath.Join(t.TempDir(), "not.png")
	os.WriteFile(text, []byte("not an image"), 0o644)
	if _, err := coverImage(text); err == nil || !strings.Contains(err.Error(), "cannot be read") {
		t.Errorf("%v", err)
	}
	if _, err := coverImage(filepath.Join(t.TempDir(), "missing.png")); err == nil {
		t.Error("a missing image was taken")
	}
}

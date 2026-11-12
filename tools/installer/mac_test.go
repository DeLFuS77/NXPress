package main

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dmgbuild is given the settings of the window, the app and its background, and names the disk
// NXPress.
func TestDmgArgs(t *testing.T) {
	got := filepath.ToSlash(strings.Join(dmgArgs("out/NXPress.app", "dist/nxpress-macos-intel.dmg"), " "))
	want := "-s build/macos/dmg/settings.py -D app=out/NXPress.app -D background=build/macos/dmg/background.png NXPress dist/nxpress-macos-intel.dmg"
	if got != want {
		t.Errorf("%s", got)
	}
}

// The background fits the window of the settings, at twice its size for Retina beside it, and
// the settings put in the image the app and the link to Applications only, where the background
// has room for them.
func TestDmgWindow(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	settings, err := os.ReadFile(filepath.Join(dmgFolder, "settings.py"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(settings)
	for _, want := range []string{
		`files = [(app, "NXPress.app")]`,
		`symlinks = {"Applications": "/Applications"}`,
		`window_rect = ((200, 140), (540, 360))`,
		`icon_locations = {"NXPress.app": (140, 170), "Applications": (400, 170)}`,
		`format = "UDZO"`,
		`background = defines["background"]`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("settings.py has no %s", want)
		}
	}
	for name, size := range map[string]image.Point{"background.png": {540, 360}, "background@2x.png": {1080, 720}} {
		file, err := os.Open(filepath.Join(dmgFolder, name))
		if err != nil {
			t.Fatal(err)
		}
		config, _, err := image.DecodeConfig(file)
		file.Close()
		if err != nil || config.Width != size.X || config.Height != size.Y {
			t.Errorf("%s: %dx%d %v", name, config.Width, config.Height, err)
		}
	}
}

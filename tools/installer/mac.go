package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// The download of macOS: a disk image (.dmg) with NXPress.app and a link to Applications, in a
// window whose background says to drag one onto the other (build/macos/dmg). Nothing runs when it
// is opened or copied: there is no script in it, nor an installer. It is made on a Mac by dmgbuild
// (a pinned version, see the workflow), which writes the window the Finder shows and makes the
// image with hdiutil.
//
//	go run ./tools/installer -mac -app out/NXPress.app -out dist/nxpress-macos-apple-silicon.dmg

// volumeName is the name of the disk in the Finder.
const volumeName = "NXPress"

// dmgFolder holds the settings of the window and its background, from the root of the repository.
var dmgFolder = filepath.Join("build", "macos", "dmg")

func runMac(app, out string) error {
	if app == "" || out == "" {
		return fmt.Errorf("give -app and -out")
	}
	if _, err := os.Stat(filepath.Join(app, "Contents", "MacOS", "nxpress")); err != nil {
		return fmt.Errorf("%s is not the app of NXPress: %w", app, err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	// hdiutil fails now and then on a busy machine ("Resource busy"): it is tried again
	for attempt := 1; ; attempt++ {
		err := run("dmgbuild", dmgArgs(app, out)...)
		if err == nil || attempt == 3 {
			return err
		}
		time.Sleep(time.Duration(attempt) * 5 * time.Second)
	}
}

// dmgArgs are the arguments of dmgbuild for the image of the app given, named NXPress.
func dmgArgs(app, out string) []string {
	return []string{"-s", filepath.Join(dmgFolder, "settings.py"), "-D", "app=" + app,
		"-D", "background=" + filepath.Join(dmgFolder, "background.png"), volumeName, out}
}

func run(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

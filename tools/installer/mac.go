package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// The download of macOS: a disk image (.dmg) with NXPress.app and a link to Applications, to
// drag the app onto. Nothing runs when it is opened or copied: there is no script in it, nor an
// installer. It is made on a Mac, with hdiutil.
//
//	go run ./tools/installer -mac -app out/NXPress.app -out dist/nxpress-macos-apple-silicon.dmg

// volumeName is the name of the disk in the Finder.
const volumeName = "NXPress"

func runMac(app, out string) error {
	if app == "" || out == "" {
		return fmt.Errorf("give -app and -out")
	}
	work, err := os.MkdirTemp("", "nxpress-dmg-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	folder, err := dmgFolder(work, func(to string) error { return run("ditto", app, to) })
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	// hdiutil fails now and then on a busy machine ("Resource busy"): it is tried again
	for attempt := 1; ; attempt++ {
		err = run("hdiutil", dmgArgs(folder, out)...)
		if err == nil || attempt == 3 {
			return err
		}
		time.Sleep(time.Duration(attempt) * 5 * time.Second)
	}
}

// dmgFolder makes, in work, the folder the image is made of: the app, copied by copyApp (ditto
// keeps its signature), and the link to Applications. Nothing else goes in it.
func dmgFolder(work string, copyApp func(to string) error) (string, error) {
	folder := filepath.Join(work, volumeName)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}
	if err := copyApp(filepath.Join(folder, "NXPress.app")); err != nil {
		return "", err
	}
	if err := os.Symlink("/Applications", filepath.Join(folder, "Applications")); err != nil {
		return "", err
	}
	return folder, nil
}

// dmgArgs are the arguments of hdiutil for the image of the folder given: compressed, read-only,
// and named NXPress.
func dmgArgs(folder, out string) []string {
	return []string{"create", "-volname", volumeName, "-srcfolder", folder, "-fs", "HFS+", "-format", "UDZO", "-ov", out}
}

func run(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

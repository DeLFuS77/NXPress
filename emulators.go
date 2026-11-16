package main

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The Emulators page lists the emulators the person may want, by the console, each with its page,
// which opens in the browser. The list here is the one NXPress comes with: the page of the Emulators lets
// the person change it (sections, categories, emulators, notes), and keeps their list by itself.

// Emulator is an emulator of the list: its name, its page, the key of the note to tell of it ("" for
// none) and the key of the family it is of ("" for none), which the page translates. The emulators of a
// family are one after the other.
type Emulator struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Note    string `json:"note"`
	Family  string `json:"family"`
	// Plain says the note is not a warning: the page draws it without the sign of one
	Plain bool `json:"plain,omitempty"`
}

// EmulatorGroup is the emulators of a console: its title, the icon the page draws beside it, and them.
type EmulatorGroup struct {
	Title     string     `json:"title"`
	Icon      string     `json:"icon"`
	Emulators []Emulator `json:"emulators"`
}

var emulatorGroups = []EmulatorGroup{
	{Title: "Nintendo Switch 1", Icon: "switch1", Emulators: []Emulator{
		{Name: "Eden", Address: "https://git.eden-emu.dev/", Family: "nEmuFamilyYuzu"},
		{Name: "NeXium", Address: "https://nexium-emu.org/index.html", Family: "nEmuFamilyYuzu"},
		{Name: "Citron Neo", Address: "https://citron-neo.github.io/", Family: "nEmuFamilyYuzu"},
		{Name: "Lemon", Address: "https://git.lemon-emu.org/lemon/Lemon-Project", Note: "nEmuNoteLemon", Family: "nEmuFamilyYuzu"},
		{Name: "Ryubing (Stable)", Address: "https://git.ryujinx.app/projects/Ryubing/releases", Family: "nEmuFamilyRyujinx"},
		{Name: "Ryubing (Canary)", Address: "https://git.ryujinx.app/Ryubing/Canary/releases", Family: "nEmuFamilyRyujinx"},
	}},
	{Title: "Nintendo Switch 2", Icon: "switch2", Emulators: []Emulator{
		{Name: "Orchid", Address: "https://gitlab.com/MythosOfs/Orchid", Note: "nEmuNoteEdenFork", Family: "nEmuFamilyYuzu"},
		{Name: "Lichii", Address: "https://github.com/zortos293/Lichii", Note: "nEmuNoteEdenFork", Family: "nEmuFamilyYuzu"},
		{Name: "Frichii", Address: "https://github.com/Frizti/Fritchii", Note: "nEmuNoteFrichii", Family: "nEmuFamilyYuzu"},
		{Name: "Nemyu", Address: "https://gitlab.com/nemyu-emulator/nemyu", Note: "nEmuNoteEdenFork", Family: "nEmuFamilyYuzu"},
		{Name: "Droplet", Address: "https://gitlab.com/seighttt/droplet", Note: "nEmuNoteDroplet", Family: "nEmuFamilyYuzu"},
		{Name: "FEFW", Address: "https://github.com/TheSneakyBadger/FEFW-Stable-with-60-FPS-Patch/", Note: "nEmuNoteFefw", Family: "nEmuFamilyYuzu"},
		{Name: "eden-nx2-android", Address: "https://git.eden-emu.dev/moceure/eden-nx2-android", Note: "nEmuNoteLemon", Family: "nEmuFamilyYuzu"},
		{Name: "Hokkaido", Address: "https://github.com/pavelgajdo/Hokkaido", Note: "nEmuNoteLemon", Family: "nEmuFamilyYuzu"},
		{Name: "BananaNative", Address: "https://github.com/shamonj03/BananaNative", Note: "nEmuNoteBanana", Family: "nEmuFamilyNative"},
		{Name: "NX Play", Address: "https://github.com/varyform/nx-native-wrapper", Note: "nEmuNoteApple", Family: "nEmuFamilyNative"},
		{Name: "NX2Apple", Address: "https://github.com/LNPANDY/NX2Apple", Note: "nEmuNoteApple", Family: "nEmuFamilyNative"},
		{Name: "LumaNX", Address: "https://github.com/Xc1Ym/LumaNX", Note: "nEmuNoteApple", Family: "nEmuFamilyNative"},
		{Name: "nxnative", Address: "https://github.com/seedds/nxnative", Note: "nEmuNoteOriginal", Family: "nEmuFamilyNative"},
	}},
	// the games that were recompiled to run on their own, not emulated
	{Title: "Recompilations", Icon: "recompile", Emulators: []Emulator{
		{Name: "Longshot", Address: "https://git.magicbean.dev/MagicBeanDev/Longshot", Note: "nEmuNoteLongshot", Family: "nEmuFamilySwitch2"},
		{Name: "NX2navi", Address: "https://github.com/tangerineturtle1/NX2navi", Note: "nEmuNoteLongshot", Family: "nEmuFamilySwitch2"},
		{Name: "DecompBuddy", Address: "https://decompbuddy.com/", Note: "nEmuNoteDecomp", Family: "nEmuFamilyAll"},
	}},
}

// EmulatorList is the emulators of the Emulators page, by console.
func (a *App) EmulatorList() []EmulatorGroup {
	return emulatorGroups
}

// OpenEmulatorPage opens the page of an emulator in the browser. The person writes the pages of their own
// list, so any page on http or https with a host is opened; no other kind of address (a file, a program).
func (a *App) OpenEmulatorPage(address string) error {
	if !emulatorPage(address) {
		return errors.New("not a web page")
	}
	openURL(a.ctx, address)
	return nil
}

// emulatorPage says whether an address is a web page: http or https, with a host, and no space or control
// character in it.
func emulatorPage(address string) bool {
	if address != strings.TrimSpace(address) || strings.ContainsAny(address, " \t\r\n\\") {
		return false
	}
	page, err := url.Parse(address)
	if err != nil || page.Hostname() == "" {
		return false
	}
	return page.Scheme == "https" || page.Scheme == "http"
}

// maxEmulatorsFile is the biggest file of emulators NXPress reads: a list of any size a person writes is
// a few kilobytes, so a bigger file is not one.
const maxEmulatorsFile = 2 << 20

// SaveEmulators asks where to keep the list of emulators the person shares, and writes it there. It gives
// the path it wrote, or "" when the person closed the window.
func (a *App) SaveEmulators(content string) (string, error) {
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename:      "NXPress emulators.json",
		Filters:              []wailsruntime.FileFilter{{DisplayName: "Emulators (*.json)", Pattern: "*.json"}},
		CanCreateDirectories: true,
	})
	if err != nil || path == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		path += ".json"
	}
	return path, os.WriteFile(path, []byte(content), 0o644)
}

// OpenEmulators asks for a file of emulators and gives its text, "" when the person closed the window. The
// page checks what it holds.
func (a *App) OpenEmulators() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Filters: []wailsruntime.FileFilter{{DisplayName: "Emulators (*.json, *.txt)", Pattern: "*.json;*.txt"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return readEmulatorsFile(path)
}

// readEmulatorsFile reads a file of emulators: text, and not a big one.
func readEmulatorsFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("not a file of emulators")
	}
	if info.Size() > maxEmulatorsFile {
		return "", errors.New("too big")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", errors.New("not a file of emulators")
	}
	return strings.TrimPrefix(string(data), "\ufeff"), nil
}

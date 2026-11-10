package main

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"

	"github.com/DeLFuS77/NXPress/internal/selfupdate"
	"github.com/DeLFuS77/NXPress/internal/system"
)

// The installer of Windows puts NXPress in Windows with the same code as the Settings of the
// window: it runs the program it copied with --installer, hidden.
//
//	nxpress --installer add [files] [menu] [sendto] [--language xx]
//	nxpress --installer remove
//	nxpress --installer replace <program> <sha256>
//
// add opens the files of Switch with NXPress, puts its menu in the Explorer in that language
// and NXPress in "Send to"; remove takes away what is there for this copy of NXPress, and leaves
// what another copy (a portable one) put there. replace is an update of an NXPress in Program
// Files: the NXPress installed, run as the administrator, puts the program downloaded and checked
// in its place, when it has the sum given (see selfupdate.ReplaceChecked).

// installerChecksum is the exit code of a replace whose program has not the sum given.
const installerChecksum = 2

// installerParts are what the options of the installer can add.
var installerParts = []string{"files", "menu", "sendto"}

// installerLanguages are the languages of the page, by the name of their file in frontend/i18n.
var installerLanguages = []string{"en", "es", "fr", "de", "it", "pt", "nl", "ru", "ja", "ko", "zh"}

// runInstaller does what the installer asks, and tells on out and errs how it went.
func runInstaller(args []string, out, errs io.Writer) int {
	executable, err := system.Executable()
	if err == nil {
		switch {
		case len(args) > 0 && args[0] == "add":
			err = installerAdd(executable, args[1:])
		case len(args) == 1 && args[0] == "remove":
			err = installerRemove(executable)
		case len(args) == 3 && args[0] == "replace":
			err = selfupdate.ReplaceChecked(executable, args[1], args[2])
			if errors.Is(err, selfupdate.ErrChecksum) {
				fmt.Fprintln(errs, err)
				return installerChecksum
			}
		default:
			err = errors.New("usage: nxpress --installer add [files] [menu] [sendto] [--language xx] | remove | replace <program> <sha256>")
		}
	}
	if err != nil {
		fmt.Fprintln(errs, err)
		return 1
	}
	fmt.Fprintln(out, "done")
	return 0
}

func installerAdd(executable string, args []string) error {
	language := "en"
	parts := []string{}
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--language" && i+1 < len(args):
			i++
			if slices.Contains(installerLanguages, args[i]) {
				language = args[i]
			}
		case slices.Contains(installerParts, args[i]):
			parts = append(parts, args[i])
		default:
			return fmt.Errorf("the installer cannot add %q", args[i])
		}
	}
	for _, part := range parts {
		var err error
		switch part {
		case "files":
			err = system.AddFiles(executable)
		case "menu":
			err = system.AddMenu(executable, menuTexts(language))
		case "sendto":
			err = system.EnableSendTo()
		}
		if err != nil {
			return fmt.Errorf("%s: %w", part, err)
		}
	}
	return nil
}

// installerRemove takes away only what points to this copy of NXPress.
func installerRemove(executable string) error {
	var errs []error
	if place, _ := system.FilesPlace(executable); place == system.PlaceHere {
		errs = append(errs, system.RemoveFiles())
	}
	if system.MenuPlace(executable) == system.PlaceHere {
		errs = append(errs, system.RemoveMenu())
	}
	if system.SendToEnabled() {
		errs = append(errs, system.DisableSendTo())
	}
	return errors.Join(errs...)
}

// menuEntry finds the text of a section in a file of frontend/i18n: its first line names it
var menuEntry = regexp.MustCompile(`\b(verify|info|compress|decompress|pack): "([^"]*)"`)

// menuTexts are the names of the entries of the menu of the Explorer in a language, as the page
// gives them; the page writes them again when its language changes (#88). A name the language
// has not is the English one.
func menuTexts(language string) system.ExplorerMenu {
	names := map[string]string{}
	for _, name := range []string{"en", language} {
		page, err := frontend.ReadFile("frontend/i18n/" + name + ".js")
		if err != nil {
			continue
		}
		found := map[string]bool{}
		for _, match := range menuEntry.FindAllStringSubmatch(string(page), -1) {
			if !found[match[1]] {
				found[match[1]] = true
				names[match[1]] = match[2]
			}
		}
	}
	return system.ExplorerMenu{Verify: names["verify"], Info: names["info"], Compress: names["compress"], Decompress: names["decompress"], Pack: names["pack"]}
}

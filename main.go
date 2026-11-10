package main

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DeLFuS77/NXPress/internal/cli"
	"github.com/DeLFuS77/NXPress/internal/selfupdate"
	"github.com/DeLFuS77/NXPress/internal/system"
	"github.com/DeLFuS77/NXPress/internal/titledb"
)

// version is set by the build of a release
var version = "dev"

func main() {
	cli.Version = version
	titledb.UserAgent = "NXPress/" + version
	args := afterUpdate(os.Args[1:])
	// the installer of Windows, putting NXPress in Windows or taking it away (see installer.go)
	if len(args) > 0 && args[0] == "--installer" {
		os.Exit(runInstaller(args[1:], os.Stdout, os.Stderr))
	}
	// the window, with the files it is given (see windowLaunch). Started from the Explorer, the
	// console Windows opened for it is closed
	if start, ok := windowLaunch(args); ok {
		system.HideConsole()
		os.Exit(runWindow(start))
	}
	os.Exit(run(args))
}

// afterUpdate takes away --after-update and the process it names, which the NXPress an update
// replaced starts the new one with (#91): the new one waits for it to stop, so its window has
// gone, then deletes what it left. Any other start deletes what an update left too.
func afterUpdate(args []string) []string {
	if len(args) >= 2 && args[0] == "--after-update" {
		if pid, err := strconv.Atoi(args[1]); err == nil {
			system.WaitForExit(pid, 30*time.Second)
		}
		args = args[2:]
	}
	if program, err := system.Executable(); err == nil {
		selfupdate.CleanOld(program)
	}
	return args
}

// launch is what the window opens with: files, and how they came.
type launch struct {
	files []string
	// the section the menu of the Explorer asked for (#88)
	section string
	// opened with a double click: NXPress is the program of the files (#87)
	open bool
}

// sections are the ones the menu of the Explorer can ask for, by their names in the page.
var sections = []string{"compress", "decompress", "recompress", "trim", "verify", "convert", "split", "pack", "info", "fat32", "rename"}

// windowLaunch tells whether the arguments open the window, and with what: nothing; a notice
// clicked; only files or folders, dropped on NXPress or sent to it from the Explorer; --open and
// files, opened with NXPress as their program (#87); or --section, a section and files, from the
// menu of the Explorer (#88). The files of the Explorer open the window even when one of them
// is gone meanwhile; any other arguments are for the command line.
func windowLaunch(args []string) (launch, bool) {
	switch {
	case len(args) == 0:
		return launch{}, true
	case onlyPaths(args):
		return launch{files: args}, true
	case args[0] == "--open" && len(args) > 1:
		return launch{files: existing(args[1:]), open: true}, true
	case args[0] == "--section" && len(args) > 2 && slices.Contains(sections, args[1]):
		return launch{files: existing(args[2:]), section: args[1]}, true
	case fromNotification(args):
		return launch{}, true
	}
	return launch{}, false
}

// existing are the paths there, none an option.
func existing(paths []string) []string {
	there := []string{}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil && !strings.HasPrefix(path, "-") {
			there = append(there, path)
		}
	}
	return there
}

// absolute makes the paths given to another NXPress whole, from the folder it was started in,
// so the window that takes them finds them: not the options, nor the section after --section.
func absolute(args []string, folder string) []string {
	whole := slices.Clone(args)
	for i, arg := range whole {
		if strings.HasPrefix(arg, "-") || i == 1 && whole[0] == "--section" || filepath.IsAbs(arg) || folder == "" {
			continue
		}
		whole[i] = filepath.Join(folder, arg)
	}
	return whole
}

// fromNotification tells whether Windows started NXPress for a notice of it clicked while it was
// closed: it gives -Embedding (the notices of NXPress are a COM server) or -ToastActivated, and
// the window is what opens, not the command line.
func fromNotification(args []string) bool {
	for _, arg := range args {
		if strings.EqualFold(arg, "-Embedding") || strings.EqualFold(arg, "-ToastActivated") {
			return true
		}
	}
	return false
}

// onlyPaths tells whether every argument is a file or folder there, none an option.
func onlyPaths(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return false
		}
		if _, err := os.Stat(arg); err != nil {
			return false
		}
	}
	return len(args) > 0
}

func run(args []string) int {
	return cli.RunCommand(args, os.Stdout, os.Stderr)
}

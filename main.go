package main

import (
	"os"
	"strings"
)

// version is set by the build of a release
var version = "dev"

func main() {
	// with no arguments it is the window, and with only files or folders too: the ones dropped
	// on NXPress, or sent to it from the Explorer. Started from the Explorer, the console
	// Windows opened for it is closed
	if len(os.Args) == 1 || onlyPaths(os.Args[1:]) {
		hideConsole()
		os.Exit(runWindow(os.Args[1:]))
	}
	if fromNotification(os.Args[1:]) {
		hideConsole()
		os.Exit(runWindow(nil))
	}
	os.Exit(run(os.Args[1:]))
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
	return runCommand(args, os.Stdout, os.Stderr)
}

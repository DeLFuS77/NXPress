package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/settings"
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
	os.Exit(run(os.Args[1:]))
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
	o, err := parseOptions(args)
	if err == nil && o.help {
		fmt.Printf(usage, version)
		return 0
	}
	if err == nil && o.version {
		fmt.Println("NXPress", version)
		return 0
	}
	if err == nil {
		err = o.check()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "nxpress:", err)
		return 2
	}
	out := &reporter{out: os.Stdout, minimal: o.minimal, machine: o.machine}
	for _, ignored := range o.ignored {
		out.say("note: %s has no effect in NXPress", ignored)
	}
	// recompressing needs no keys (the NCZ hold their data decrypted, with their keys), nor do
	// files split for FAT32 or joined
	if _, err := loadKeys(o.keys); err != nil && !o.recompress && !o.fat32 {
		fmt.Fprintln(os.Stderr, "nxpress:", err)
		return 2
	}

	// Ctrl+C stops the files at work; their half written files are removed
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	jobs := runJobs
	if o.watch {
		jobs = func(ctx context.Context, o options, out output) int { return watchJobs(ctx, o, out, watchEvery) }
	}
	if failed := jobs(ctx, o, out); failed > 0 {
		return 1
	}
	return 0
}

// loadKeys makes the keys active, and gives the file they were read from: the ones given, else
// a prod.keys or a keys.txt next to NXPress, in the working folder or in ~/.switch, the folder
// nsz and hactool use too.
func loadKeys(given string) (string, error) {
	if given != "" {
		if err := settings.UseSwitchKeys(given); err != nil {
			return "", fmt.Errorf("the keys %s cannot be read: %w", given, err)
		}
		return given, nil
	}
	folders := []string{"."}
	if executable, err := os.Executable(); err == nil {
		folders = append(folders, filepath.Dir(executable))
	}
	if home, err := os.UserHomeDir(); err == nil {
		folders = append(folders, filepath.Join(home, ".switch"))
	}
	for _, folder := range folders {
		for _, name := range []string{"prod.keys", "keys.txt"} {
			path := filepath.Join(folder, name)
			if _, err := os.Stat(path); err == nil && settings.UseSwitchKeys(path) == nil {
				if absolute, err := filepath.Abs(path); err == nil {
					path = absolute
				}
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("no prod.keys or keys.txt found (looked in %s): give one with --keys", strings.Join(folders, ", "))
}

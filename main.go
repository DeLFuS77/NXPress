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
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	o, err := parseOptions(args)
	if err == nil && (o.help || len(args) == 0) {
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
	if err := loadKeys(o.keys); err != nil {
		fmt.Fprintln(os.Stderr, "nxpress:", err)
		return 2
	}

	// Ctrl+C stops the files at work; their half written files are removed
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if failed := runJobs(ctx, o, out); failed > 0 {
		return 1
	}
	return 0
}

// loadKeys makes the keys active: the ones given, else a prod.keys or a keys.txt next to
// NXPress, in the working folder or in ~/.switch, the folder nsz and hactool use too.
func loadKeys(given string) error {
	if given != "" {
		if err := settings.UseSwitchKeys(given); err != nil {
			return fmt.Errorf("the keys %s cannot be read: %w", given, err)
		}
		return nil
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
				return nil
			}
		}
	}
	return fmt.Errorf("no prod.keys or keys.txt found (looked in %s): give one with --keys", strings.Join(folders, ", "))
}

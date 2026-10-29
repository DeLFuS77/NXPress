package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
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

	if o.lowPriority {
		if err := setLowPriority(true); err != nil {
			out.say("note: the priority could not be lowered: %v", err)
		}
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

// unreadFolder is a folder that could not be read, and why.
type unreadFolder struct {
	path string
	err  error
}

// folderFiles are the files in a folder and its folders of parts, in order of name, of every
// kind (the caller takes the ones it works on); with recursive, the ones in its folders too,
// and in theirs. unread are the folders that could not be read, and nested tells whether
// folders were left out, without recursive.
func folderFiles(folder string, recursive bool) (found []string, unread []unreadFolder, nested bool) {
	filepath.WalkDir(folder, func(walked string, entry os.DirEntry, err error) error {
		if err != nil {
			// told, not passed over: a folder without permission would leave its games out
			// without a word
			unread = append(unread, unreadFolder{path: walked, err: err})
			return nil
		}
		if walked == folder {
			return nil
		}
		if !entry.IsDir() {
			found = append(found, walked)
			return nil
		}
		// a folder of parts is one file: its parts are not looked into
		if _, ok := splitContainer(walked); ok {
			found = append(found, walked)
			return filepath.SkipDir
		}
		if !recursive {
			nested = true
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(found)
	return found, unread, nested
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

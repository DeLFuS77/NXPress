package main

import (
	"context"
	"fmt"
	"io"
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
	return runCommand(args, os.Stdout, os.Stderr)
}

// runCommand does what the command line asks, printing to stdout and stderr, and gives the
// exit code.
func runCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	o, err := parseOptions(args)
	if err == nil && o.help {
		fmt.Fprintf(stdout, usage, version)
		return 0
	}
	if err == nil && o.version {
		fmt.Fprintln(stdout, "NXPress", version)
		return 0
	}
	if err == nil {
		err = o.check()
	}
	if err != nil {
		fmt.Fprintln(stderr, "nxpress:", err)
		return 2
	}
	out := &reporter{out: stdout, minimal: o.minimal, machine: o.machine}
	for _, ignored := range o.ignored {
		out.say("note: %s has no effect in NXPress", ignored)
	}
	// recompressing needs no keys (the NCZ hold their data decrypted, with their keys), nor do
	// files split for FAT32 or joined
	if _, err := loadKeys(o.keys); err != nil && !o.recompress && !o.fat32 {
		fmt.Fprintln(stderr, "nxpress:", err)
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
	failed := 0
	jobs := runJobs
	if o.watch {
		// a watch looks at the folders given themselves, and into all their folders
		jobs = func(ctx context.Context, o options, out output) int { return watchJobs(ctx, o, out, watchEvery) }
	} else {
		o.files, failed = commandLineFiles(o, out)
	}
	if failed += jobs(ctx, o, out); failed > 0 {
		return 1
	}
	return 0
}

// commandLineFiles are the files given, with the files of each folder given in its place: only
// the ones in it, as nsz takes a folder, and with --recursive the ones in its folders too, and
// in theirs. A script written for nsz with --rm-source must not delete the originals of every
// folder below. The window, which does not come here, always looks into them all. It gives too
// how many folders could not be read, each told as failed.
func commandLineFiles(o options, out output) ([]string, int) {
	files := []string{}
	failed := 0
	for _, path := range o.files {
		info, err := os.Stat(path)
		if _, parts := splitContainer(path); err != nil || !info.IsDir() || parts {
			files = append(files, path)
			continue
		}
		found, unread, nested := folderFiles(path, o.recursive)
		for _, folder := range unread {
			out.done(folder.path, "", "", folder.err)
			failed++
		}
		if nested {
			out.say("note: the folders in %s are left out (--recursive looks into them)", path)
		}
		files = append(files, found...)
	}
	return files, failed
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

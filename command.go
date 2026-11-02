package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// commandVersion is the version the command line tells: the one of the build, given by main
var commandVersion = "dev"

// stopSignals stop the files at work: Ctrl+C, and SIGTERM, which Go gives on Windows too when
// the console is closed, the user signs out or the computer shuts down (the system then waits
// a few seconds for NXPress to remove its half written files)
var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// runCommand does what the command line asks, printing to stdout and stderr, and gives the
// exit code.
func runCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	o, err := parseOptions(args)
	if err == nil && o.help {
		fmt.Fprintf(stdout, usage, commandVersion)
		return 0
	}
	if err == nil && o.version {
		fmt.Fprintln(stdout, "NXPress", commandVersion)
		return 0
	}
	if err == nil {
		err = o.check()
	}
	if err != nil {
		// nsz tells this one in JSON too, to the program that asked for it
		if errors.Is(err, errOutputModes) {
			(&reporter{out: stdout}).json(jsonObject{"type", "error", "code", codeOutputModes, "message", err.Error()})
		}
		fmt.Fprintln(stderr, "nxpress:", err)
		return 2
	}
	out := &reporter{out: stdout, minimal: o.minimal, machine: o.machine, action: o.action()}
	for _, ignored := range o.ignored {
		out.note("%s has no effect in NXPress", ignored)
	}
	for _, note := range o.notes {
		out.note("%s", note)
	}
	// recompressing needs no keys (the NCZ hold their data decrypted, with their keys), nor do
	// files split for FAT32 or joined
	if _, err := loadKeys(o.keys); err != nil && !o.recompress && !o.fat32 {
		fmt.Fprintln(stderr, "nxpress:", err)
		return 2
	}

	if o.lowPriority {
		if err := setLowPriority(true); err != nil {
			out.note("the priority could not be lowered: %v", err)
		}
	}

	// Ctrl+C stops the files at work, and so does the end of NXPress the system asks for; their
	// half written files are removed
	ctx, stop := signal.NotifyContext(context.Background(), stopSignals...)
	defer stop()
	// a program reading --machine-readable is told now and then that NXPress is at work
	stopHeartbeat := func() {}
	if o.machine {
		stopHeartbeat = out.heartbeat(heartbeatEvery)
	}
	failed := 0
	jobs := runJobs
	if o.watch {
		// a watch looks at the folders given themselves, and into all their folders
		jobs = func(ctx context.Context, o options, out output) int { return watchJobs(ctx, o, out, watchEvery) }
	} else {
		// the folders given stay known: the pack of the files of one goes next to it (#45)
		o.origins = o.files
		o.files, failed = commandLineFiles(o, out)
	}
	failed += jobs(ctx, o, out)
	stopHeartbeat()
	out.finish()
	if failed > 0 {
		return 1
	}
	return 0
}

// commandLineFiles are the files given, with the files of each folder given in its place: only
// the ones in it, as nsz takes a folder, and with --recursive the ones in its folders too, and
// in theirs. A script written for nsz with --rm-source must not delete the originals of every
// folder below. The window, which does not come here, always looks into them all. It gives too
// how many folders could not be read, each told as failed.
func commandLineFiles(o options, out *reporter) ([]string, int) {
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
			out.note("the folders in %s are left out (--recursive looks into them)", path)
		}
		files = append(files, found...)
	}
	return files, failed
}

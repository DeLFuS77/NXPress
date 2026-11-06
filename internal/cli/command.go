// Package cli is the command line of NXPress: its options, as nsz takes them, and what it
// prints.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/DeLFuS77/NXPress/internal/jobs"
	"github.com/DeLFuS77/NXPress/internal/portable"
	"github.com/DeLFuS77/NXPress/internal/system"
	"github.com/DeLFuS77/NXPress/internal/titledb"
)

// Version is the version the command line tells: the one of the build, given by main
var Version = "dev"

// stopSignals stop the files at work: Ctrl+C, and SIGTERM, which Go gives on Windows too when
// the console is closed, the user signs out or the computer shuts down (the system then waits
// a few seconds for NXPress to remove its half written files)
var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// keptVersions gives the versions published the window keeps, nil when it keeps none
var keptVersions = func() *titledb.Versions {
	folder, err := portable.DataFolder()
	if err != nil {
		return nil
	}
	versions, _ := titledb.Load(folder)
	return versions
}

// keptNames gives the names of the games of the Switch 2 the window keeps, nil when it keeps none:
// --nup names with them a dump folder without control data
var keptNames = func() *titledb.Names {
	folder, err := portable.DataFolder()
	if err != nil {
		return nil
	}
	names, _ := titledb.LoadNames(folder)
	return names
}

// RunCommand does what the command line asks, printing to stdout and stderr, and gives the
// exit code.
func RunCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	o, err := jobs.ParseOptions(args)
	if err == nil && o.Help {
		fmt.Fprintf(stdout, jobs.Usage, Version)
		return 0
	}
	if err == nil && o.Version {
		fmt.Fprintln(stdout, "NXPress", Version)
		return 0
	}
	if err == nil {
		err = o.Check()
	}
	if err != nil {
		// nsz tells this one in JSON too, to the program that asked for it
		if errors.Is(err, jobs.ErrOutputModes) {
			(&jobs.Reporter{Out: stdout}).JSON(jobs.JSONObject{"type", "error", "code", jobs.CodeOutputModes, "message", err.Error()})
		}
		fmt.Fprintln(stderr, "nxpress:", err)
		return 2
	}
	out := &jobs.Reporter{Out: stdout, Minimal: o.Minimal, Machine: o.Machine, Action: o.Action()}
	for _, ignored := range o.Ignored {
		out.Note("%s has no effect in NXPress", ignored)
	}
	for _, note := range o.Notes {
		out.Note("%s", note)
	}
	// recompressing needs no keys (the NCZ hold their data decrypted, with their keys), nor do
	// files split for FAT32 or joined, nor NUP files verified or shown (their games are decrypted)
	if _, err := jobs.LoadKeys(o.Keys); err != nil && jobs.NeedsKeys(o) {
		fmt.Fprintln(stderr, "nxpress:", err)
		return 2
	}

	// -i tells the newest versions published too, from the copy the window keeps when its setting
	// is on: the command line does not go on the internet
	if o.Info {
		o.Versions = keptVersions()
	}
	// --nup names a dump folder without control data from the names of the games of the Switch 2
	// the window keeps, the same way
	if o.Nup {
		jobs.UseNames(keptNames())
	}

	if o.LowPriority {
		if err := system.SetLowPriority(true); err != nil {
			out.Note("the priority could not be lowered: %v", err)
		}
	}

	// Ctrl+C stops the files at work, and so does the end of NXPress the system asks for; their
	// half written files are removed
	ctx, stop := signal.NotifyContext(context.Background(), stopSignals...)
	defer stop()
	// a program reading --machine-readable is told now and then that NXPress is at work
	stopHeartbeat := func() {}
	if o.Machine {
		stopHeartbeat = out.Heartbeat(jobs.HeartbeatEvery)
	}
	failed := 0
	work := jobs.RunJobs
	if o.Watch {
		// a watch looks at the folders given themselves, and into all their folders
		work = func(ctx context.Context, o jobs.Options, out jobs.Output) int {
			return jobs.WatchJobs(ctx, o, out, jobs.WatchEvery)
		}
	} else {
		// the folders given stay known: the pack of the files of one goes next to it (#45)
		o.Origins = o.Files
		o.Files, failed = commandLineFiles(o, out)
	}
	failed += work(ctx, o, out)
	stopHeartbeat()
	out.Finish()
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
func commandLineFiles(o jobs.Options, out *jobs.Reporter) ([]string, int) {
	files := []string{}
	failed := 0
	for _, path := range o.Files {
		info, err := os.Stat(path)
		if _, parts := jobs.SplitContainer(path); err != nil || !info.IsDir() || parts || jobs.IsDumpFolder(path) {
			files = append(files, path)
			continue
		}
		found, unread, nested := jobs.FolderFiles(path, o.Recursive)
		for _, folder := range unread {
			out.Done(folder.Path, "", "", folder.Err)
			failed++
		}
		if nested {
			out.Note("the folders in %s are left out (--recursive looks into them)", path)
		}
		files = append(files, found...)
	}
	return files, failed
}

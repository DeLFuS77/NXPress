// Package jobs is the work NXPress does on the files (compress, decompress, verify, trim, split,
// combine, rename, FAT32 parts, measure and watch), with the options and the progress the window
// and the command line share.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/DeLFuS77/NXPress/internal/cache"
	"github.com/DeLFuS77/NXPress/internal/dat"
	"github.com/DeLFuS77/NXPress/internal/durable"
	"github.com/DeLFuS77/NXPress/internal/system"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// the files each action takes, by extension
var Takes = map[string][]string{
	"compress":   {".nsp", ".xci"},
	"decompress": {".nsz", ".xcz", ".nup"},
	"verify":     {".nsp", ".nsz", ".xci", ".xcz", ".nup"},
	"convert":    {".xci", ".xcz"},
	"recompress": {".nsz", ".xcz"},
	"trim":       {".xci", ".nsp", ".nsz"},
	"estimate":   {".nsp", ".xci"},
	"measure":    {".nsp", ".xci"},
	"rename":     {".nsp", ".nsz", ".xci", ".xcz", ".nup"},
	// with the files in parts, to make them whole
	"fat32": {".nsp", ".nsz", ".xci", ".xcz", splitFiles},
	"split": {".nsp", ".nsz", ".xci", ".xcz"},
	"info":  {".nsp", ".nsz", ".xci", ".xcz", ".nup"},
	// the dump folders packed into NUP files
	"nup": {dumpFolders},
	// Combine: the files it combines, and the dump folders it packs into NUP files
	"pack": {".nsp", ".nsz", ".xci", ".xcz", dumpFolders},
}

// checksOutput tells, by action, whether the new file it gives is checked as a game (every NCA
// by its hashes) before it gets its name, so the cache of the files checked keeps it. A file
// made whole from its parts is only compared with them, and a file renamed is not read.
var checksOutput = map[string]bool{
	"compress":   true,
	"decompress": true,
	"convert":    true,
	"recompress": true,
	"trim":       true,
	"fat32":      false,
	"rename":     false,
	"split":      false,
	"estimate":   false,
	"measure":    false,
	"info":       false,
	"verify":     false,
	"nup":        true,
}

// RunJobs does the action on every file, several at once with -m, and gives how many failed.
func RunJobs(ctx context.Context, o Options, out Output) int {
	defer cache.Checked.Flush()
	// the computer does not go to sleep while files are at work
	defer system.KeepAwake()()
	action := o.Action()
	// the LZ4 blocks of the extra compression keep to the threads of each file (lz4Threads)
	ctx = switchfs.WithThreads(ctx, lz4Threads(o))
	// a NUP given to a work that does not take it is told, not left out without a word
	nupsLeft := tellNupsLeft(o, out)
	// the dump folders given to Combine are packed into NUP files, each on its own; the files
	// given with them are combined as they would be without them
	failedDumps := 0
	if action == "pack" {
		var dumps []string
		dumps, o.Files = splitDumps(o.Files)
		if len(dumps) > 0 {
			nupWork := o
			nupWork.Files, nupWork.Pack, nupWork.PackGames, nupWork.PackOne, nupWork.Nup = dumps, "", false, false, true
			failedDumps = RunJobs(ctx, nupWork, out)
			if len(o.Files) == 0 {
				return failedDumps
			}
		}
	}
	if action == "pack" && o.PackGames {
		return failedDumps + PackGames(ctx, o, out)
	}
	if action == "pack" && o.PackOne {
		return failedDumps + packOne(ctx, o, out)
	}
	if action == "pack" {
		if err := pack(ctx, o, out); err != nil {
			out.Done(o.Pack, "", "", shown(err))
			if IsSkipped(err) {
				return failedDumps
			}
			return failedDumps + 1
		}
		return failedDumps
	}
	files, missing := Inputs(o.Files, Takes[action])
	for _, path := range missing {
		out.Done(path, "", "", errors.New("no such file or folder"))
	}
	if len(files) == 0 {
		if nupsLeft == 0 {
			out.Say("no file to %s among the ones given", action)
		}
		return len(missing)
	}
	// the measure tries its settings on the biggest game that can be compressed
	if action == "measure" {
		path, err := MeasurableGame(ctx, files, o)
		output, note := "", ""
		if err == nil {
			output, note, err = measure(ctx, path, o, out)
		}
		out.Done(path, output, note, shown(err))
		if err != nil && !IsSkipped(err) {
			return len(missing) + 1
		}
		return len(missing)
	}
	if o.Dat != "" && o.datFile == nil {
		loaded, err := dat.LoadDat(o.Dat)
		if err != nil {
			out.Done(o.Dat, "", "", shown(err))
			return len(files) + len(missing)
		}
		o.datFile = loaded
	}
	ctx = withDestinations(ctx, files)
	threads := switchfs.LimitThreads(strconv.Itoa(o.Level), o.Threads, o.Multi)
	var (
		mutex  sync.Mutex
		failed = len(missing)
		wait   sync.WaitGroup
	)
	work := make(chan string)
	for worker := 0; worker < min(o.Multi, len(files)); worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for path := range work {
				// a file is taken only while the work is not paused: renaming, -i and the files
				// checked before have no progress of their own to pause in
				waitWhilePaused(out)
				if ctx.Err() != nil {
					continue
				}
				output, note, err := safeJob(ctx, action, path, o, threads, out)
				// a new file checked as a game before it got its name need not be read again (a
				// folder a NUP was unpacked into is no file to remember)
				if err == nil && output != "" && checksOutput[action] && !isDir(output) {
					cache.Checked.Keep(output, "")
				}
				out.Done(path, output, note, shown(err))
				if isFailure(err) {
					mutex.Lock()
					failed++
					mutex.Unlock()
				}
			}
		}()
	}
	for _, path := range files {
		if ctx.Err() != nil {
			break
		}
		work <- path
	}
	close(work)
	wait.Wait()
	return failed
}

// waitWhilePaused waits while the work is paused, in the window; the console has no pause.
func waitWhilePaused(out Output) {
	if p, ok := out.(interface{ WaitWhilePaused() }); ok {
		p.WaitWhilePaused()
	}
}

// skippedError is a file left as it is, with why: not done, and not failed either, so it
// is told apart from both (SKIPPED, and in yellow in the window).
type skippedError struct {
	reason string
}

func (e skippedError) Error() string {
	return e.reason
}

func Skipped(reason string) error {
	return skippedError{reason: reason}
}

func IsSkipped(err error) bool {
	var s skippedError
	return errors.As(err, &s)
}

// modifiedError is a file checked whose NCA files are not all signed by Nintendo, while none
// of them is damaged: a tool rewrote them, and it usually installs and plays. It is no failure
// (#42): it is told apart (MODIFIED, and in orange in the window), as a damaged file is not.
type modifiedError struct {
	reason string
}

func (e modifiedError) Error() string {
	return e.reason
}

func modified(reason string) error {
	return modifiedError{reason: reason}
}

func IsModified(err error) bool {
	var m modifiedError
	return errors.As(err, &m)
}

// isFailure tells whether a file ended in an error: not skipped, and not only modified.
func isFailure(err error) bool {
	return err != nil && !IsSkipped(err) && !IsModified(err)
}

// splitFiles, among the extensions taken, takes the files in parts too: a folder of parts, or
// the first part of name.ns0, name.ns1...
const splitFiles = "parts"

// Inputs are the files given and the ones in the folders given (and in their folders), of the
// extensions taken, each once and in order of name; missing are the paths not found, and the
// folders that could not be read. A folder of parts is one file, taken only with splitFiles.
func Inputs(paths []string, extensions []string) (files []string, missing []string) {
	seen := map[string]bool{}
	// the files taken by their size, to find one given twice under two names
	bySize := map[int64][]os.FileInfo{}
	withParts, withDumps := false, false
	for _, taken := range extensions {
		withParts = withParts || taken == splitFiles
		withDumps = withDumps || taken == dumpFolders
	}
	takes := func(path string) bool {
		if strings.HasPrefix(filepath.Base(path), ".") {
			return false
		}
		if IsDumpFolder(path) {
			return withDumps
		}
		if _, ok := SplitContainer(path); ok {
			return withParts
		}
		extension := strings.ToLower(filepath.Ext(path))
		for _, taken := range extensions {
			if extension == taken {
				return true
			}
		}
		return false
	}
	add := func(path string) {
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		if seen[path] {
			return
		}
		seen[path] = true
		// the same file by another name is done once: a.nsp and A.NSP on Windows and macOS, or a
		// link to it
		if info, err := os.Stat(path); err == nil {
			for _, other := range bySize[info.Size()] {
				if os.SameFile(info, other) {
					return
				}
			}
			bySize[info.Size()] = append(bySize[info.Size()], info)
		}
		files = append(files, path)
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			missing = append(missing, path)
			continue
		}
		if _, ok := SplitContainer(path); !info.IsDir() || ok || IsDumpFolder(path) {
			if takes(path) {
				add(path)
			}
			continue
		}
		found, unread, _ := FolderFiles(path, true)
		for _, folder := range unread {
			missing = append(missing, folder.Path)
		}
		for _, walked := range found {
			if takes(walked) {
				add(walked)
			}
		}
	}
	return files, missing
}

// target is where the new file of path goes: next to it, or in the output folder.
func target(o Options, name string) (string, error) {
	if o.Output == "" {
		return name, nil
	}
	folder, err := outputFolder(o)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(folder, filepath.Base(name)), nil
}

// outputFolder is the folder of -o as an absolute path: "." or a folder given relative names the
// folder it is at the start, not wherever the work is when it is used.
func outputFolder(o Options) (string, error) {
	if o.Output == "" {
		return "", nil
	}
	return filepath.Abs(o.Output)
}

// SameFile tells whether two paths name the same file or folder, however they are written: in
// another case, through a link or a junction, by a short name, or on a mapped drive.
func SameFile(a string, b string) bool {
	first, err := os.Stat(a)
	if err != nil {
		return false
	}
	second, err := os.Stat(b)
	return err == nil && os.SameFile(first, second)
}

// destinations are the names the files of a work take, so that no two of them get the same one
// (nor the hidden name a new file is written under), and none takes the name of a file the
// work has to read: each file given holds its own name from the start, and each new name is
// held by the file it comes from. Names that differ only in case are one on Windows and macOS.
type destinations struct {
	mutex   sync.Mutex
	owners  map[string]string
	folders map[string]string
}

type destinationsKey struct{}

// withDestinations gives ctx the destinations of a work on files. A watched folder works in
// rounds under one set, so a round never takes the name of a new file of a round before.
func withDestinations(ctx context.Context, files []string) context.Context {
	if d, ok := ctx.Value(destinationsKey{}).(*destinations); ok {
		d.mutex.Lock()
		defer d.mutex.Unlock()
		for _, file := range files {
			if _, taken := d.owners[d.key(file)]; !taken {
				d.owners[d.key(file)] = file
			}
		}
		return ctx
	}
	d := &destinations{owners: map[string]string{}, folders: map[string]string{}}
	for _, file := range files {
		d.owners[d.key(file)] = file
	}
	return context.WithValue(ctx, destinationsKey{}, d)
}

// claim holds name for the new file of path, or tells which file of the work has it.
func claim(ctx context.Context, path string, name string) error {
	d, ok := ctx.Value(destinationsKey{}).(*destinations)
	if !ok {
		return nil
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	key := d.key(name)
	owner, ok := d.owners[key]
	switch {
	case !ok || owner == path:
		d.owners[key] = path
		return nil
	case d.key(owner) == key:
		return fmt.Errorf("it would get the same name as %s, another file of this work", owner)
	}
	return fmt.Errorf("it would get the same name as the new file of %s: %s", owner, filepath.Base(name))
}

// key is a name as the file system tells it apart: absolute, with the links of its folder
// followed (and on Windows its short names made long), in lower case where case is not told.
func (d *destinations) key(name string) string {
	if absolute, err := filepath.Abs(name); err == nil {
		name = absolute
	}
	folder := filepath.Dir(name)
	real, ok := d.folders[folder]
	if !ok {
		real = folder
		if followed, err := filepath.EvalSymlinks(folder); err == nil {
			real = followed
		}
		d.folders[folder] = real
	}
	key := filepath.Join(real, filepath.Base(name))
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		key = strings.ToLower(key)
	}
	return key
}

// temporary is the hidden name a new file is written under until it is checked.
func temporary(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".nxpress")
}

// finish gives the checked temporary file its name. With -w the rename replaces a file there,
// and nothing is deleted before it; without -w the rename fails when a file took the name
// meanwhile.
func finish(o Options, written string, path string) error {
	if o.Overwrite {
		return durable.CommitFile(written, path)
	}
	err := durable.CommitNew(written, path)
	if errors.Is(err, fs.ErrExist) {
		if there := free(o, path); there != nil {
			return there
		}
	}
	return err
}

// free tells whether a new file can be written at path: a file there is replaced only with -w,
// and without it the file worked on is skipped, as nsz skips it (a script run again over a
// library does not fail for the files done before).
func free(o Options, path string) error {
	if _, err := os.Stat(path); err == nil && !o.Overwrite {
		return Skipped(fmt.Sprintf("%s is already there (use -w to replace it)", filepath.Base(path)))
	}
	return nil
}

// ErrEmpty is a file of 0 bytes, a copy that never happened: told plainly, before it is read.
var ErrEmpty = errors.New("the file is empty")

// isDir tells whether a path is a folder.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// splitDumps parts the paths given into the dump folders among them and the rest.
func splitDumps(paths []string) (dumps []string, rest []string) {
	for _, path := range paths {
		if IsDumpFolder(path) {
			dumps = append(dumps, path)
		} else {
			rest = append(rest, path)
		}
	}
	return dumps, rest
}

// isEmpty tells whether path is a file of 0 bytes (a folder of parts is not).
func isEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() == 0
}

// safeJob runs doJob and turns a panic into a failed file instead of a crash: a malformed file
// that trips a parser stops that one job, and the rest of the work goes on.
func safeJob(ctx context.Context, action string, path string, o Options, threads int, out Output) (output string, note string, err error) {
	defer func() {
		if r := recover(); r != nil {
			output, note, err = "", "", fmt.Errorf("the file could not be read: %v", r)
		}
	}()
	return doJob(ctx, action, path, o, threads, out)
}

func doJob(ctx context.Context, action string, path string, o Options, threads int, out Output) (string, string, error) {
	if isEmpty(path) {
		return "", "", ErrEmpty
	}
	if isNup(path) {
		switch action {
		case "info":
			note, err := infoNup(path, o.Versions, out)
			return "", note, err
		case "verify":
			return verifyNup(ctx, path, o, out)
		case "decompress":
			return unpackNup(ctx, path, o, threads, out)
		case "rename":
			return rename(ctx, path, o, out)
		}
		return "", "", Skipped(NupNotDone)
	}
	if action == "nup" {
		return packNup(ctx, path, o, threads, out)
	}
	switch action {
	case "compress":
		return compress(ctx, path, o, threads, out)
	case "decompress":
		return decompress(ctx, path, o, out)
	case "convert":
		return convert(ctx, path, o, out)
	case "recompress":
		return recompress(ctx, path, o, threads, out)
	case "trim":
		return trim(ctx, path, o, out)
	case "estimate":
		return estimate(ctx, path, o, threads, out)
	case "fat32":
		return fat32(ctx, path, o, out)
	case "rename":
		return rename(ctx, path, o, out)
	case "split":
		return split(ctx, path, o, out)
	case "info":
		note, err := info(path, o.Versions, out)
		return "", note, err
	}
	return verify(ctx, path, o, out)
}

// removeSource deletes the original with --rm-source once its new file is checked whole, and
// gives back name and note. A file checked only in part (an NCA rewritten by a tool with a
// section that cannot be checked on its own) does not replace it: the original is kept, and the
// note says why. An original that is the new file itself, however its name is written, is
// never deleted.
func removeSource(o Options, path string, check switchfs.FileCheck, name string, note string) (string, string, error) {
	if !o.RmSource {
		return name, note, nil
	}
	if check.Partial {
		return name, keptNote(o, check, note), nil
	}
	if name != "" && SameFile(path, name) {
		return name, note, nil
	}
	if err := os.Remove(path); err != nil {
		return name, note, fmt.Errorf("the new file is checked, but the original could not be deleted: %w", err)
	}
	return name, note, nil
}

// keptNote is note, with why the original stays when --rm-source cannot delete it.
func keptNote(o Options, check switchfs.FileCheck, note string) string {
	if !o.RmSource || !check.Partial {
		return note
	}
	return joinNotes(note, keptInPart)
}

func modifiedNote(check switchfs.FileCheck) string {
	if len(check.Modified) == 0 {
		return ""
	}
	return fmt.Sprintf("%d NCA rewritten by a tool, their data checked", len(check.Modified))
}

func size(bytes int64) string {
	value, unit := float64(bytes), "B"
	for _, next := range []string{"KB", "MB", "GB", "TB"} {
		if value < 1024 {
			break
		}
		value, unit = value/1024, next
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}

func percent(part int64, whole int64) string {
	if whole <= 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", float64(part)*100/float64(whole))
}

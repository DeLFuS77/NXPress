package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// the files each action takes, by extension
var takes = map[string][]string{
	"compress":   {".nsp", ".xci"},
	"decompress": {".nsz", ".xcz"},
	"verify":     {".nsp", ".nsz", ".xci", ".xcz"},
	"convert":    {".xci", ".xcz"},
	"split":      {".nsp", ".nsz", ".xci", ".xcz"},
	"info":       {".nsp", ".nsz", ".xci", ".xcz"},
}

func (o options) action() string {
	switch {
	case o.compress:
		return "compress"
	case o.decompress:
		return "decompress"
	case o.convert:
		return "convert"
	case o.split:
		return "split"
	case o.info:
		return "info"
	case o.pack != "":
		return "pack"
	}
	return "verify"
}

// runJobs does the action on every file, several at once with -m, and gives how many failed.
func runJobs(ctx context.Context, o options, out *reporter) int {
	action := o.action()
	if action == "pack" {
		if err := pack(ctx, o, out); err != nil {
			out.done(o.pack, "", "", err)
			return 1
		}
		return 0
	}
	files, missing := inputs(o.files, takes[action])
	for _, path := range missing {
		out.done(path, "", "", errors.New("no such file or folder"))
	}
	if len(files) == 0 {
		out.say("no file to %s among the ones given", action)
		return len(missing)
	}
	threads := switchfs.LimitThreads(strconv.Itoa(o.level), o.threads, o.multi)
	var (
		mutex  sync.Mutex
		failed = len(missing)
		wait   sync.WaitGroup
	)
	work := make(chan string)
	for worker := 0; worker < min(o.multi, len(files)); worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for path := range work {
				output, note, err := doJob(ctx, action, path, o, threads, out)
				out.done(path, output, note, err)
				if err != nil {
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

// inputs are the files given and the ones in the folders given (and in their folders), of the
// extensions taken, each once and in order of name; missing are the paths not found.
func inputs(paths []string, extensions []string) (files []string, missing []string) {
	seen := map[string]bool{}
	takes := func(path string) bool {
		extension := strings.ToLower(filepath.Ext(path))
		for _, taken := range extensions {
			if extension == taken {
				return !strings.HasPrefix(filepath.Base(path), ".")
			}
		}
		return false
	}
	add := func(path string) {
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		if !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			missing = append(missing, path)
			continue
		}
		if !info.IsDir() {
			if takes(path) {
				add(path)
			}
			continue
		}
		found := []string{}
		filepath.WalkDir(path, func(walked string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && takes(walked) {
				found = append(found, walked)
			}
			return nil
		})
		sort.Strings(found)
		for _, walked := range found {
			add(walked)
		}
	}
	return files, missing
}

// target is where the new file of path goes: next to it, or in the output folder.
func target(o options, name string) (string, error) {
	if o.output == "" {
		return name, nil
	}
	if err := os.MkdirAll(o.output, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(o.output, filepath.Base(name)), nil
}

// temporary is the hidden name a new file is written under until it is checked.
func temporary(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".nxpress")
}

// finish gives the checked temporary file its name, replacing a file there with -w.
func finish(o options, written string, path string) error {
	if o.overwrite {
		os.Remove(path)
	}
	return os.Rename(written, path)
}

// free tells whether a new file can be written at path.
func free(o options, path string) error {
	if _, err := os.Stat(path); err == nil && !o.overwrite {
		return fmt.Errorf("%s is already there (use -w to replace it)", filepath.Base(path))
	}
	return nil
}

func doJob(ctx context.Context, action string, path string, o options, threads int, out *reporter) (string, string, error) {
	switch action {
	case "compress":
		return compress(ctx, path, o, threads, out)
	case "decompress":
		return decompress(ctx, path, o, out)
	case "convert":
		return convert(ctx, path, o, out)
	case "split":
		return split(ctx, path, o, out)
	case "info":
		return "", info(path, out), nil
	}
	check, err := switchfs.VerifyFile(ctx, path, out.progress(path, "Verifying"))
	return "", modifiedNote(check), err
}

func compress(ctx context.Context, path string, o options, threads int, out *reporter) (string, string, error) {
	name, err := target(o, switchfs.CompressedPath(path))
	if err == nil {
		err = free(o, name)
	}
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	result, err := switchfs.CompressGame(ctx, path, written, switchfs.CompressOptions{
		Level: strconv.Itoa(o.level), Workers: threads, Blocks: o.block, Progress: out.progress(path, "Compressing"),
	})
	if errors.Is(err, switchfs.ErrNotCompressible) {
		return "", "nothing in it can be compressed, it is left as it is", nil
	}
	if err != nil {
		return "", "", err
	}
	// the original was hashed while it was read: an NCA that does not match its content ID is
	// checked by its hash trees, then every file of the compressed copy is checked against it
	check, err := switchfs.CheckHashes(ctx, path, result.Hashes)
	if err != nil {
		return "", "", err
	}
	if err := switchfs.VerifyCompressed(ctx, written, result.Hashes, out.progress(path, "Verifying")); err != nil {
		return "", "", err
	}
	if err := finish(o, written, name); err != nil {
		return "", "", err
	}
	note := fmt.Sprintf("%s -> %s, %s smaller", size(result.InputSize), size(result.OutputSize), percent(result.InputSize-result.OutputSize, result.InputSize))
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	return name, note, removeSource(o, path)
}

func decompress(ctx context.Context, path string, o options, out *reporter) (string, string, error) {
	if strings.EqualFold(filepath.Ext(path), ".xcz") {
		return "", "", errors.New("XCZ to XCI is not supported yet: --convert turns it into an NSZ, and -D then into an NSP")
	}
	name, err := target(o, switchfs.DecompressedPath(path))
	if err == nil {
		err = free(o, name)
	}
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	check, err := switchfs.DecompressAndCheck(ctx, path, written, out.progress(path, "Decompressing"))
	if err == nil {
		err = finish(o, written, name)
	}
	if err != nil {
		return "", "", err
	}
	return name, modifiedNote(check), removeSource(o, path)
}

func convert(ctx context.Context, path string, o options, out *reporter) (string, string, error) {
	name, err := target(o, switchfs.ConvertedPath(path))
	if err == nil {
		err = free(o, name)
	}
	if err != nil {
		return "", "", err
	}
	written := temporary(name)
	defer os.Remove(written)
	check, err := switchfs.ConvertAndCheck(ctx, path, written, out.progress(path, "Converting"))
	if err == nil {
		err = finish(o, written, name)
	}
	if err != nil {
		return "", "", err
	}
	return name, modifiedNote(check), removeSource(o, path)
}

// split writes each title of a multi-title file (each game of a game card with several) to a
// file of its own, named like the original with the title ID and version of the part.
func split(ctx context.Context, path string, o options, out *reporter) (string, string, error) {
	plan, err := switchfs.PlanSplit(path)
	if err != nil {
		return "", "", err
	}
	if len(plan.Parts) < 2 {
		return "", "it holds one title, nothing to split", nil
	}
	base := strings.TrimSpace(strings.SplitN(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), "[", 2)[0])
	names := []string{}
	for _, part := range plan.Parts {
		extension := ".nsp"
		switch {
		case part.Cartridge && part.Compressed:
			extension = ".xcz"
		case part.Cartridge:
			extension = ".xci"
		case part.Compressed:
			extension = ".nsz"
		}
		name, err := target(o, filepath.Join(filepath.Dir(path), fmt.Sprintf("%s [%s][v%d]%s", base, strings.ToUpper(part.TitleId), part.Version, extension)))
		if err == nil {
			err = free(o, name)
		}
		if err != nil {
			return "", "", err
		}
		written := temporary(name)
		_, err = switchfs.WriteSplitPartAndCheck(ctx, path, part, written, out.progress(path, "Splitting "+part.Type))
		if err == nil {
			err = finish(o, written, name)
		}
		os.Remove(written)
		if err != nil {
			return "", "", err
		}
		names = append(names, filepath.Base(name))
	}
	note := fmt.Sprintf("%d files: %s", len(names), strings.Join(names, ", "))
	if !plan.Complete() {
		// files of the original that no title lists: it is kept, whatever --rm-source says
		return "", note + "; some of its files are in no part, so it is kept", nil
	}
	return "", note, removeSource(o, path)
}

// pack merges the files given into one multi-title file (a SuperNSP), checked as it is
// written; the delta fragments of the updates are left out, as installers do not need them.
func pack(ctx context.Context, o options, out *reporter) error {
	sources, missing := inputs(o.files, []string{".nsp", ".nsz"})
	if len(missing) > 0 {
		return fmt.Errorf("not found: %s", strings.Join(missing, ", "))
	}
	if len(sources) < 2 {
		return errors.New("a pack needs at least two NSP or NSZ files")
	}
	name, err := target(o, o.pack)
	if err == nil {
		err = free(o, name)
	}
	if err != nil {
		return err
	}
	written := temporary(name)
	defer os.Remove(written)
	skipped, check, err := switchfs.MergeAndCheck(ctx, sources, written, true, out.progress(o.pack, "Packing"))
	if err == nil {
		err = finish(o, written, name)
	}
	if err != nil {
		return err
	}
	note := fmt.Sprintf("%d files merged", len(sources))
	if skipped > 0 {
		note += fmt.Sprintf(", %s of delta fragments left out", size(skipped))
	}
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	out.done(o.pack, name, note, nil)
	for _, source := range sources {
		if err := removeSource(o, source); err != nil {
			return err
		}
	}
	return nil
}

// info tells the titles a file holds.
func info(path string, out *reporter) string {
	read := switchfs.ReadNspMetadata
	if extension := strings.ToLower(filepath.Ext(path)); extension == ".xci" || extension == ".xcz" {
		read = switchfs.ReadXciMetadata
	}
	titles, err := read(path)
	if err != nil {
		return "cannot be read: " + err.Error()
	}
	ids := make([]string, 0, len(titles))
	for id := range titles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lines := []string{}
	for _, id := range ids {
		title := titles[id]
		line := fmt.Sprintf("%s %s v%d", strings.ToUpper(title.TitleId), title.Type, title.Version)
		if title.Ncap != nil {
			if name := strings.TrimSpace(title.Ncap.TitleName["AmericanEnglish"].Title); name != "" {
				line += " " + name
			}
		}
		lines = append(lines, line)
	}
	out.say("%s:\n  %s", filepath.Base(path), strings.Join(lines, "\n  "))
	return fmt.Sprintf("%d titles", len(titles))
}

func removeSource(o options, path string) error {
	if !o.rmSource {
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("the new file is checked, but the original could not be deleted: %w", err)
	}
	return nil
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

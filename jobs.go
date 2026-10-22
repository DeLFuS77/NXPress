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
	"time"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// the files each action takes, by extension
var takes = map[string][]string{
	"compress":   {".nsp", ".xci"},
	"decompress": {".nsz", ".xcz"},
	"verify":     {".nsp", ".nsz", ".xci", ".xcz"},
	"convert":    {".xci", ".xcz"},
	"recompress": {".nsz", ".xcz"},
	"trim":       {".xci", ".nsp", ".nsz"},
	"estimate":   {".nsp", ".xci"},
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
	case o.recompress:
		return "recompress"
	case o.trim:
		return "trim"
	case o.estimate:
		return "estimate"
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
func runJobs(ctx context.Context, o options, out output) int {
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
	if o.dat != "" && o.datFile == nil {
		loaded, err := loadDat(o.dat)
		if err != nil {
			out.done(o.dat, "", "", err)
			return len(files) + len(missing)
		}
		o.datFile = loaded
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

func doJob(ctx context.Context, action string, path string, o options, threads int, out output) (string, string, error) {
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
	case "split":
		return split(ctx, path, o, out)
	case "info":
		return "", info(path, out), nil
	}
	check, err := switchfs.VerifyFile(ctx, path, out.progress(path, "Verifying"))
	note := modifiedNote(check)
	if err != nil || o.datFile == nil {
		return "", note, err
	}
	found, err := o.datFile.match(ctx, path, out.progress(path, "Comparing"))
	if note != "" {
		found = note + "; " + found
	}
	return "", found, err
}

func compress(ctx context.Context, path string, o options, threads int, out output) (string, string, error) {
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

func decompress(ctx context.Context, path string, o options, out output) (string, string, error) {
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

func convert(ctx context.Context, path string, o options, out output) (string, string, error) {
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

// recompress writes an NSZ or XCZ again, solid or in blocks, at another level. Without an output
// folder the file is replaced, once its new copy is checked.
func recompress(ctx context.Context, path string, o options, threads int, out output) (string, string, error) {
	name, err := target(o, path)
	if err != nil {
		return "", "", err
	}
	inPlace := strings.EqualFold(filepath.Clean(name), filepath.Clean(path))
	if !inPlace {
		if err := free(o, name); err != nil {
			return "", "", err
		}
	}
	written := temporary(name)
	defer os.Remove(written)
	result, err := switchfs.RecompressGame(ctx, path, written, switchfs.CompressOptions{
		Level: strconv.Itoa(o.level), Workers: threads, Blocks: o.block, Progress: out.progress(path, "Recompressing"),
	})
	if err != nil {
		return "", "", err
	}
	check, err := switchfs.VerifyFile(ctx, written, out.progress(path, "Verifying"))
	if err != nil {
		return "", "", err
	}
	if inPlace {
		err = os.Rename(written, path)
	} else {
		err = finish(o, written, name)
	}
	if err != nil {
		return "", "", err
	}
	mode := "solid"
	if o.block || strings.EqualFold(filepath.Ext(path), ".xcz") {
		mode = "in blocks"
	}
	note := fmt.Sprintf("%s, %s -> %s", mode, size(result.InputSize), size(result.OutputSize))
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	if inPlace {
		return name, note, nil
	}
	return name, note, removeSource(o, path)
}

// trim takes away what installers do not use: the padding after the data of an XCI, or the
// delta fragments of the updates of an NSP or NSZ. In place, or into the output folder.
func trim(ctx context.Context, path string, o options, out output) (string, string, error) {
	if extension := strings.ToLower(filepath.Ext(path)); extension == ".nsp" || extension == ".nsz" {
		return stripDeltas(ctx, path, o, out)
	}
	name, err := target(o, path)
	if err != nil {
		return "", "", err
	}
	inPlace := strings.EqualFold(filepath.Clean(name), filepath.Clean(path))
	written := name
	if !inPlace {
		if err := free(o, name); err != nil {
			return "", "", err
		}
		written = temporary(name)
		defer os.Remove(written)
	}
	removed, err := switchfs.TrimXci(ctx, path, written, out.progress(path, "Trimming"))
	if errors.Is(err, switchfs.ErrNothingToTrim) {
		return "", "nothing to trim, it ends where its data ends", nil
	}
	if err != nil {
		return "", "", err
	}
	check, err := switchfs.VerifyFile(ctx, written, out.progress(path, "Verifying"))
	if err != nil {
		return "", "", err
	}
	if !inPlace {
		if err := finish(o, written, name); err != nil {
			return "", "", err
		}
	}
	note := fmt.Sprintf("%s of padding taken away", size(removed))
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	if inPlace {
		return name, note, nil
	}
	return name, note, removeSource(o, path)
}

// stripDeltas writes an NSP or NSZ again without the delta fragments of its updates.
func stripDeltas(ctx context.Context, path string, o options, out output) (string, string, error) {
	deltas, err := switchfs.DeltaFragmentsSize(path)
	if err != nil {
		return "", "", err
	}
	if deltas == 0 {
		return "", "nothing to trim, it holds no delta fragments", nil
	}
	name, err := target(o, path)
	if err != nil {
		return "", "", err
	}
	inPlace := strings.EqualFold(filepath.Clean(name), filepath.Clean(path))
	if !inPlace {
		if err := free(o, name); err != nil {
			return "", "", err
		}
	}
	written := temporary(name)
	defer os.Remove(written)
	removed, check, err := switchfs.StripDeltasAndCheck(ctx, path, written, out.progress(path, "Trimming"))
	if err != nil {
		return "", "", err
	}
	if inPlace {
		err = os.Rename(written, path)
	} else {
		err = finish(o, written, name)
	}
	if err != nil {
		return "", "", err
	}
	note := fmt.Sprintf("%s of delta fragments taken away", size(removed))
	if modified := modifiedNote(check); modified != "" {
		note += ", " + modified
	}
	if inPlace {
		return name, note, nil
	}
	return name, note, removeSource(o, path)
}

// how much of a game is compressed to tell its estimate
const (
	estimateSamples    = 8
	estimateSampleSize = 16 << 20
)

// estimate tells the size and the time a compression would give, from samples of the game.
func estimate(ctx context.Context, path string, o options, threads int, out output) (string, string, error) {
	result, err := switchfs.EstimateCompression(ctx, path, switchfs.CompressOptions{
		Level: strconv.Itoa(o.level), Workers: threads, Blocks: o.block, Progress: out.progress(path, "Estimating"),
	}, estimateSamples, estimateSampleSize)
	if errors.Is(err, switchfs.ErrNothingToMeasure) {
		return "", "nothing in it can be compressed", nil
	}
	if err != nil {
		return "", "", err
	}
	return "", fmt.Sprintf("about %s -> %s, %s smaller, %s at level %d", size(result.InputSize), size(result.OutputSize),
		percent(result.InputSize-result.OutputSize, result.InputSize), duration(result.Duration), o.level), nil
}

// duration tells a time for people, rounded.
func duration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", max(1, int(d.Round(time.Second).Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Round(time.Minute).Minutes()))
	}
	return fmt.Sprintf("%d h %d min", int(d.Hours()), int(d.Round(time.Minute).Minutes())%60)
}

// split writes each title of a multi-title file (each game of a game card with several) to a
// file of its own, named like the original with the title ID and version of the part.
func split(ctx context.Context, path string, o options, out output) (string, string, error) {
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
func pack(ctx context.Context, o options, out output) error {
	sources, missing := inputs(o.files, []string{".nsp", ".nsz", ".xci", ".xcz"})
	if len(missing) > 0 {
		return fmt.Errorf("not found: %s", strings.Join(missing, ", "))
	}
	if len(sources) < 2 {
		return errors.New("a pack needs at least two files")
	}
	// a game card among them: its updates and DLC go into it, and the pack is a game card
	cards := []string{}
	others := []string{}
	for _, source := range sources {
		if extension := strings.ToLower(filepath.Ext(source)); extension == ".xci" || extension == ".xcz" {
			cards = append(cards, source)
		} else {
			others = append(others, source)
		}
	}
	if len(cards) > 1 {
		return errors.New("only one game card can be packed: give its updates and DLC as NSP or NSZ")
	}
	// a pack of NSP files is an NSP, uncompressed; one with an NSZ among them holds NCZ, an NSZ
	compressed := false
	for _, source := range sources {
		if facts, err := switchfs.DescribeFile(source); err != nil {
			return err
		} else if facts.Solid+facts.Blocks > 0 {
			compressed = true
		}
	}
	extension := ".nsp"
	switch {
	case len(cards) == 1 && compressed:
		extension = ".xcz"
	case len(cards) == 1:
		extension = ".xci"
	case compressed:
		extension = ".nsz"
	}
	name, err := target(o, strings.TrimSuffix(o.pack, filepath.Ext(o.pack))+extension)
	if err == nil {
		err = free(o, name)
	}
	if err != nil {
		return err
	}
	written := temporary(name)
	defer os.Remove(written)
	// of a title given twice (an update in a SuperNSP, and a newer one), the newest is kept
	var (
		skipped  int64
		replaced []string
		check    switchfs.FileCheck
	)
	if len(cards) == 1 {
		replaced, check, err = switchfs.PackIntoCard(ctx, cards[0], others, written, out.progress(o.pack, "Packing"))
	} else {
		skipped, replaced, check, err = switchfs.MergeNewestAndCheck(ctx, sources, written, true, out.progress(o.pack, "Packing"))
	}
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
	if len(replaced) > 0 {
		note += ", newer versions in place of " + strings.Join(replaced, ", ")
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
func info(path string, out output) string {
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
	lines = append(lines, describe(path)...)
	out.say("%s:\n  %s", filepath.Base(path), strings.Join(lines, "\n  "))
	if len(titles) == 1 {
		return "1 title"
	}
	return fmt.Sprintf("%d titles", len(titles))
}

// describe tells how a file is stored, what it works with, and what trimming takes away.
func describe(path string) []string {
	facts, err := switchfs.DescribeFile(path)
	if err != nil {
		return nil
	}
	lines := []string{}
	switch {
	case facts.Solid > 0 && facts.Blocks > 0:
		lines = append(lines, fmt.Sprintf("%s, solid and in blocks: %s, %s uncompressed (%s smaller)", facts.Kind, size(facts.Size), size(facts.DecompressedSize), percent(facts.DecompressedSize-facts.Size, facts.DecompressedSize)))
	case facts.Solid > 0:
		lines = append(lines, fmt.Sprintf("%s, solid: %s, %s uncompressed (%s smaller)", facts.Kind, size(facts.Size), size(facts.DecompressedSize), percent(facts.DecompressedSize-facts.Size, facts.DecompressedSize)))
	case facts.Blocks > 0:
		lines = append(lines, fmt.Sprintf("%s, in blocks: %s, %s uncompressed (%s smaller)", facts.Kind, size(facts.Size), size(facts.DecompressedSize), percent(facts.DecompressedSize-facts.Size, facts.DecompressedSize)))
	default:
		lines = append(lines, fmt.Sprintf("%s, uncompressed: %s", facts.Kind, size(facts.Size)))
	}
	switch {
	case facts.Solid > 0:
		lines = append(lines, "works with: every installer (DBI, Tinfoil, Awoo, CyberFoil, Sphaira); emulators need it decompressed")
	case facts.Blocks > 0:
		lines = append(lines, "works with: every installer but Awoo; emulators that read NSZ run it as it is")
	default:
		lines = append(lines, "works with: every installer and emulator")
	}
	if facts.Padding > 0 {
		lines = append(lines, fmt.Sprintf("padding: %s that trimming takes away", size(facts.Padding)))
	}
	if facts.DeltaFragments > 0 {
		lines = append(lines, fmt.Sprintf("delta fragments: %s that trimming takes away", size(facts.DeltaFragments)))
	}
	return lines
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

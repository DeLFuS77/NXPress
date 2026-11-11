package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/optimize"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// DefaultOptimizeTemplate is the name of an NSP optimized for install: one Application of the
// version of the update, Game [0100...][v65536] (optimized).
const DefaultOptimizeTemplate = "{name} [{id}][v{version}] (optimized)"

// what each step of the optimization is told as
var optimizeSteps = map[string]string{
	"Decompressing": "Decompressing",
	"Reading":       "Reading the game with its update",
	"Measuring":     "Optimizing (measuring)",
	"Writing":       "Optimizing (writing)",
	"Checking":      "Checking the RomFS",
	"Verifying":     "Verifying",
}

// optimizeTemplate is the name of the files of --optimize.
func optimizeTemplate(o Options) string {
	if o.PackName == "" {
		return DefaultOptimizeTemplate
	}
	return o.PackName
}

// OptimizeGames optimizes the files given by game (--optimize, experimental): each game with its
// newest update into one NSP (an NSZ with -C) that takes the least room installed, next to its
// files or in -o. A game without its update, an update without its game and the DLC are left as
// they are, told why. It gives how many files failed.
func OptimizeGames(ctx context.Context, o Options, out Output) int {
	sources, missing := Inputs(o.Files, Takes["optimize"])
	failed := len(missing)
	for _, path := range missing {
		out.Done(path, "", "", errors.New("no such file or folder"))
	}
	if !optimize.Built {
		for _, path := range sources {
			out.Done(path, "", "", optimize.ErrNotBuilt)
		}
		return failed + len(sources)
	}
	ctx = withDestinations(ctx, sources)
	order, groups, unread := groupGames(ctx, sources)
	for _, source := range sources {
		if err, ok := unread[source]; ok {
			out.Done(source, "", "", shown(err))
			failed++
		}
	}
	folders := GivenFolders(o)
	for _, id := range order {
		waitWhilePaused(out)
		if ctx.Err() != nil {
			break
		}
		files := groups[id]
		if o.SkipGames[fmt.Sprintf("%016X", id)] {
			for _, file := range files {
				out.Done(file, "", "", Skipped("left out of the games to optimize"))
			}
			continue
		}
		game, dlcs, hasBase, hasUpdate := optimizeFiles(files)
		for _, dlc := range dlcs {
			out.Done(dlc, "", "", Skipped("a DLC is not optimized: install it as it is, next to the optimized game"))
		}
		if !hasBase || !hasUpdate {
			why := "Optimize for install needs the game and its update: its update is not among the files given"
			if !hasBase {
				why = "Optimize for install needs the game and its update: the game is not among the files given"
			}
			for _, file := range game {
				out.Done(file, "", "", Skipped(why))
			}
			continue
		}
		name, note, err := optimizeGame(ctx, o, game, PackFolder(game, sources, folders), out)
		err = shown(err)
		out.Done(game[0], name, note, err)
		for _, file := range game[1:] {
			if err != nil {
				out.Done(file, "", "", err)
				continue
			}
			out.Done(file, "", "optimized into "+filepath.Base(name), nil)
		}
		if isFailure(err) {
			failed += len(game)
		}
	}
	return failed
}

// optimizeFiles splits the files of a game: the game and its updates (a file holding both
// too), and the DLC files.
func optimizeFiles(files []string) (game []string, dlcs []string, hasBase bool, hasUpdate bool) {
	for _, file := range files {
		titles, err := ReadTitles(file)
		if err != nil {
			game = append(game, file)
			continue
		}
		kinds := map[string]bool{}
		for _, title := range titles {
			kinds[title.Type] = true
		}
		hasBase = hasBase || kinds["BASE"]
		hasUpdate = hasUpdate || kinds["UPD"]
		if !kinds["BASE"] && !kinds["UPD"] {
			dlcs = append(dlcs, file)
			continue
		}
		game = append(game, file)
	}
	return game, dlcs, hasBase, hasUpdate
}

// optimizeName is the name of the optimized file of a game: by the game and its newest update,
// in next or in the output folder.
func optimizeName(o Options, files []string, extension string, next string) (string, error) {
	titles := []*switchfs.ContentMetaAttributes{}
	for _, file := range files {
		read, err := ReadTitles(file)
		if err != nil {
			return "", err
		}
		for _, title := range read {
			if title.Type != "DLC" {
				titles = append(titles, title)
			}
		}
	}
	title := describeTitles(titles)
	title.Titles, title.Updates, title.Dlcs = 1, 0, 0
	return target(o, filepath.Join(next, strings.TrimSuffix(NewName(files[0], title, optimizeTemplate(o)), filepath.Ext(files[0]))+extension))
}

// optimizeGame writes the optimized file of a game, checked whole, and tells what it saves.
func optimizeGame(ctx context.Context, o Options, files []string, next string, out Output) (string, string, error) {
	extension := ".nsp"
	if o.Compress {
		extension = ".nsz"
	}
	name, err := optimizeName(o, files, extension, next)
	if err == nil {
		err = claim(ctx, files[0], name)
	}
	if err == nil {
		err = free(o, name)
	}
	if err != nil {
		return "", "", err
	}
	folder := filepath.Dir(name)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", "", err
	}
	nsp := temporary(strings.TrimSuffix(name, extension) + ".nsp")
	defer os.Remove(nsp)
	progress := map[string]func(int64, int64){}
	res, err := optimize.Run(ctx, files, nsp, optimize.Options{
		Level: 9, Threads: min(max(o.Threads, 1), 8), Work: folder,
		Progress: func(step string, done, total int64) {
			p, ok := progress[step]
			if !ok {
				label := optimizeSteps[step]
				if label == "" {
					label = step
				}
				p = out.Progress(files[0], label)
				progress[step] = p
			}
			p(done, total)
		},
	})
	var notPair optimize.ErrNotPair
	if errors.As(err, &notPair) {
		return "", "", Skipped("not optimized: " + notPair.Why)
	}
	if err != nil {
		return "", "", err
	}
	written := nsp
	packed := res.OutputSize
	if o.Compress {
		nsz := temporary(name)
		defer os.Remove(nsz)
		result, err := switchfs.CompressGame(ctx, nsp, nsz, switchfs.CompressOptions{
			Level: strconv.Itoa(o.Level), Workers: o.Threads, Blocks: o.Block, Progress: out.Progress(files[0], "Compressing"),
		})
		if err != nil {
			return "", "", err
		}
		if err := switchfs.VerifyCompressed(ctx, nsz, result.Hashes, out.Progress(files[0], "Verifying")); err != nil {
			return "", "", err
		}
		written, packed = nsz, result.OutputSize
	}
	if err := finish(o, written, name); err != nil {
		return "", "", err
	}
	note := fmt.Sprintf("experimental: v%d, one Application; installed, the game and its update take %s, now %s (%s smaller); install it with NXDock or DBI, with sigpatches, and do not install official updates over it",
		res.Version, size(res.InputSize), size(res.OutputSize), percent(res.InputSize-res.OutputSize, res.InputSize))
	if o.Compress {
		note += fmt.Sprintf("; the NSZ is %s", size(packed))
	}
	return name, note, nil
}

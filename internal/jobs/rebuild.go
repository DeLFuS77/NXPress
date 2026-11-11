package jobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/optimize"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// what each step of a rebuild is told as
var rebuildSteps = map[string]string{
	"Decompressing": "Decompressing",
	"Rebuilding":    "Rebuilding",
	"Verifying":     "Verifying",
}

// rebuildName is the name of the rebuilt file of path: Game (rebuilt).nsp, or .nsz with -C.
func rebuildName(o Options, path string) string {
	extension := ".nsp"
	if o.Compress {
		extension = ".nsz"
	}
	base := filepath.Base(path)
	if parts, ok := SplitContainer(path); ok {
		base = filepath.Base(parts)
	}
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(filepath.Dir(path), base+" (rebuilt)"+extension)
}

// rebuild writes a package again as a clean NSP (an NSZ with -C), checked whole (--rebuild).
func rebuild(ctx context.Context, path string, o Options, out Output) (string, string, error) {
	if !optimize.Built {
		return "", "", optimize.ErrNotBuilt
	}
	name, err := newFile(ctx, o, path, rebuildName(o, path))
	if err != nil {
		return "", "", err
	}
	folder := filepath.Dir(name)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", "", err
	}
	nsp := temporary(strings.TrimSuffix(name, filepath.Ext(name)) + ".nsp")
	defer os.Remove(nsp)
	progress := map[string]func(int64, int64){}
	res, err := optimize.Rebuild(ctx, path, nsp, optimize.Options{
		Level: 9, Threads: min(max(o.Threads, 1), 8), Work: folder,
		Progress: func(step string, done, total int64) {
			p, ok := progress[step]
			if !ok {
				label := rebuildSteps[step]
				if label == "" {
					label = step
				}
				p = out.Progress(path, label)
				progress[step] = p
			}
			p(done, total)
		},
	})
	if err != nil {
		return "", "", err
	}
	written := nsp
	if o.Compress {
		nsz := temporary(name)
		defer os.Remove(nsz)
		result, err := switchfs.CompressGame(ctx, nsp, nsz, switchfs.CompressOptions{
			Level: strconv.Itoa(o.Level), Workers: o.Threads, Blocks: o.Block, Progress: out.Progress(path, "Compressing"),
		})
		if err != nil {
			return "", "", err
		}
		if err := switchfs.VerifyCompressed(ctx, nsz, result.Hashes, out.Progress(path, "Verifying")); err != nil {
			return "", "", err
		}
		written = nsz
	}
	if err := finish(o, written, name); err != nil {
		return "", "", err
	}
	return name, RebuildNote(res), nil
}

// RebuildNote tells what a rebuild made right.
func RebuildNote(res optimize.RebuildResult) string {
	done := []string{}
	if res.Renamed > 0 {
		done = append(done, fmt.Sprintf("%d NCA named again by their hash", res.Renamed))
	}
	if res.Standard > 0 {
		done = append(done, fmt.Sprintf("%d NCA moved to standard crypto (no ticket)", res.Standard))
	}
	if res.FsFixed > 0 {
		done = append(done, fmt.Sprintf("%d FS header hashes made right", res.FsFixed))
	}
	if res.Download > 0 {
		done = append(done, fmt.Sprintf("%d NCA marked for a game card marked as downloaded", res.Download))
	}
	if res.Dropped > 0 {
		done = append(done, fmt.Sprintf("%d files left out (tickets, certificates, delta fragments)", res.Dropped))
	}
	what := "nothing was wrong besides the CNMT and the PFS0, made again"
	if len(done) > 0 {
		what = strings.Join(done, ", ") + "; the CNMT and the PFS0 made again"
	}
	return fmt.Sprintf("rebuilt: %d title(s), %d NCA, its data as it is; %s; it needs sigpatches", res.Titles, res.NCAs, what)
}

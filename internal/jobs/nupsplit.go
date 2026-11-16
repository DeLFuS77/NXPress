package jobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/DeLFuS77/NXPress/internal/nup"
)

// nupIconFile is the name of the icon of a NUP in the root of what is unpacked from it.
const nupIconFile = "icon.jpg"

// nupIcon is the icon a NUP carries, nil when it has none or cannot be read.
func nupIcon(path string) []byte {
	container, err := nup.Open(path)
	if err != nil {
		return nil
	}
	return container.Icon()
}

// writeNupIcon puts the icon in the root of a folder unpacked.
func writeNupIcon(folder string, icon []byte) {
	if len(icon) > 0 {
		// the icon is a gift to the person: a folder that cannot take it is still the NUP unpacked
		_ = os.WriteFile(filepath.Join(folder, nupIconFile), icon, 0o644)
	}
}

// splitPick is what the person chose to take out of one NUP in Split: the game (Base), its
// updates, its DLC and its icon.
type splitPick struct{ base, update, dlc, icon bool }

// splitPicks are the choices, by NUP; a NUP with none has all of them chosen.
var splitPicks = struct {
	sync.RWMutex
	byFile map[string]splitPick
}{byFile: map[string]splitPick{}}

// SetSplitParts tells Split what to take out of a NUP: its game (Base), its updates, its DLC and its
// icon, when it has them.
func SetSplitParts(path string, base, update, dlc, icon bool) {
	splitPicks.Lock()
	defer splitPicks.Unlock()
	splitPicks.byFile[coverKey(path)] = splitPick{base, update, dlc, icon}
}

// splitPickOf is the choice for a NUP: all of it unless the person chose otherwise.
func splitPickOf(path string) splitPick {
	splitPicks.RLock()
	defer splitPicks.RUnlock()
	if pick, ok := splitPicks.byFile[coverKey(path)]; ok {
		return pick
	}
	return splitPick{true, true, true, true}
}

// wants tells whether a package of the NUP is one the person chose.
func (pick splitPick) wants(pkg nup.Package) bool {
	switch pkg.KindName() {
	case "BASE":
		return pick.base
	case "UPD":
		return pick.update
	case "DLC":
		return pick.dlc
	}
	return false
}

// splitNup unpacks a NUP as Decompress does, into one folder named after the NUP (its base, updates
// and dlc folders, and the icon of the game as icon.jpg in its root), but only what the person
// chose: the game, its updates, its DLC and the icon. What the NUP does not have is skipped, and
// only the packages chosen are read from it. It is checked as it is written and stays as it is.
func splitNup(ctx context.Context, path string, o Options, threads int, out Output) (string, string, error) {
	container, err := nup.Open(path)
	if err != nil {
		return "", "", err
	}
	pick := splitPickOf(path)
	icon := container.Icon()
	if !pick.icon {
		icon = nil
	}
	wanted := make([]bool, len(container.Packages))
	chosen := 0
	for i, pkg := range container.Packages {
		if wanted[i] = pick.wants(pkg); wanted[i] {
			chosen++
		}
	}
	if chosen == 0 && len(icon) == 0 {
		if pick.icon {
			return "", "", Skipped("nothing to take out: none of Base, Update and DLC is chosen, and the NUP has no icon")
		}
		return "", "", Skipped("nothing to take out: Base, Update, DLC and Icon are all off")
	}
	name, err := target(o, strings.TrimSuffix(path, filepath.Ext(path)))
	if err == nil {
		err = claim(ctx, path, name)
	}
	if err != nil {
		return "", "", err
	}
	if _, err := os.Lstat(name); err == nil {
		return "", "", Skipped(fmt.Sprintf("%s is already there: move or rename it to unpack the NUP again", filepath.Base(name)))
	}
	written := temporary(name)
	if _, err := os.Lstat(written); err == nil {
		return "", "", fmt.Errorf("%s, left by a work that stopped, is in the way: remove it first", written)
	}
	notes := []string{}
	if chosen == 0 {
		// only the icon: the folder holds it alone
		if err := os.Mkdir(written, 0o755); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(filepath.Join(written, nupIconFile), icon, 0o644); err != nil {
			os.RemoveAll(written)
			return "", "", err
		}
		notes = append(notes, "its icon, "+size(int64(len(icon))))
	} else {
		workers := max(1, threads)
		result, err := nup.UnpackSelected(ctx, path, written, workers, nupReaders(path, workers), func(pkg int) bool { return wanted[pkg] }, out.Progress(path, "Unpacking"))
		if err != nil {
			return "", "", err
		}
		writeNupIcon(written, icon)
		notes = append(notes, fmt.Sprintf("%s, %s, checked by their SHA-256 as they were written", count(result.Files, "file", "files"), size(result.Size)))
	}
	// a folder that took the name meanwhile is never replaced (on Linux and macOS a rename would
	// replace an empty one without a word): the files stay in the hidden folder, said where
	if _, err := os.Lstat(name); err == nil {
		return "", "", fmt.Errorf("%s appeared while the NUP was unpacked: the files unpacked are in %s", filepath.Base(name), written)
	}
	if err := os.Rename(written, name); err != nil {
		// the hidden folder is the one this work made and filled
		os.RemoveAll(written)
		return "", "", err
	}
	if o.RmSource {
		notes = append(notes, keptOriginal)
	}
	return name, joinNotes(notes...), nil
}

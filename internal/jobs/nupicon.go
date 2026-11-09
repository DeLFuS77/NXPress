package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DeLFuS77/NXPress/internal/nup"
	"github.com/DeLFuS77/NXPress/internal/titledb"
)

// A NUP made from a dump folder without control data carries no icon: it gets the one of its game
// in titledb, found by its title ID (the one of the game for an update or a DLC), or an image of
// one's own (--nup-icon), made a JPEG of 256×256. A NUP made already without one gets it the same
// way (--fix-nup-icon, and the card of the NUP in the window). With no connection, or a game
// titledb does not know, the NUP is made all the same, without an icon, and the note tells why.

// IconLookup finds the icon of the game of a title ID, as an image of any size.
type IconLookup func(ctx context.Context, id uint64) ([]byte, error)

// ErrNoIcon is told when titledb has no icon of a game.
var ErrNoIcon = errors.New("titledb has no icon of its game")

// errNoLookup is told when titledb is not looked up: the window looks it up when its setting of
// the versions published is on, and the command line then too.
var errNoLookup = errors.New("titledb is not looked up for its icon: turn on “Warn of updates you do not have” in the settings of the window")

var iconLookup atomic.Pointer[IconLookup]

// UseIconLookup gives how the icons of the NUP without one are found, nil for not at all.
func UseIconLookup(lookup IconLookup) {
	if lookup == nil {
		iconLookup.Store(nil)
		return
	}
	iconLookup.Store(&lookup)
}

// lookupIcon finds the icon of a game, by the lookup in use.
func lookupIcon(ctx context.Context, id uint64) ([]byte, error) {
	lookup := iconLookup.Load()
	if lookup == nil {
		return nil, errNoLookup
	}
	return (*lookup)(ctx, id)
}

// OnlineIcons finds the icons in titledb: the list of the icons, kept in folder and downloaded
// again once a week at most (see titledb.RefreshIcons), read once, then each icon downloaded.
func OnlineIcons(folder string, client *http.Client) IconLookup {
	var mutex sync.Mutex
	var icons *titledb.Icons
	var checked time.Time
	return func(ctx context.Context, id uint64) ([]byte, error) {
		mutex.Lock()
		if icons == nil || time.Since(checked) > time.Hour {
			found, err := titledb.RefreshIcons(ctx, folder, client, time.Now())
			if found != nil {
				icons, checked = found, time.Now()
			} else if err != nil {
				mutex.Unlock()
				return nil, fmt.Errorf("titledb cannot be read: %w", err)
			}
		}
		address, ok := icons.URL(id)
		mutex.Unlock()
		if !ok {
			return nil, ErrNoIcon
		}
		data, err := titledb.DownloadIcon(ctx, client, address)
		if err != nil {
			return nil, fmt.Errorf("its icon cannot be downloaded from titledb: %w", err)
		}
		return data, nil
	}
}

// iconOf is the icon a NUP without one takes: the image given (--nup-icon), else the one of its
// game in titledb; made 256×256. It tells where it comes from.
func iconOf(ctx context.Context, o Options, id uint64) ([]byte, string, error) {
	if o.NoNupIcon {
		return nil, "", errors.New("--no-nup-icon was given")
	}
	from := "the icon of its game in titledb"
	var data []byte
	var err error
	if o.NupIcon != "" {
		from = "the image given"
		data, err = readIconFile(o.NupIcon)
	} else {
		data, err = lookupIcon(ctx, id)
	}
	if err != nil {
		return nil, "", err
	}
	icon, err := nup.MakeIcon(data)
	if err != nil {
		return nil, "", err
	}
	return icon, from, nil
}

// readIconFile reads an image of one's own, of the size an icon can be made from.
func readIconFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > nup.MaxIconFile {
		return nil, errors.New("the image is too big to be an icon")
	}
	return os.ReadFile(path)
}

// packIcon is the icon a dump folder without one gives its NUP, and the note that tells of it.
func packIcon(ctx context.Context, dir string, o Options) ([]byte, string) {
	summary, err := readDump(dir)
	if err != nil {
		return nil, ""
	}
	main := summary.sources[appSource(summary.sources)]
	if nup.ControlIcon(main.Control) != nil {
		return nil, ""
	}
	if o.NoNupIcon {
		return nil, ""
	}
	icon, from, err := iconOf(ctx, o, main.TitleID)
	if err != nil {
		return nil, noIconNote(err)
	}
	return icon, "its folder has no icon: it carries " + from + ", 256×256"
}

// appSource is the package of a dump folder that carries the icon of the game, as nup.Pack picks
// it: the game, else the first.
func appSource(sources []nup.Source) int {
	for i, source := range sources {
		if source.Kind == nup.KindBase {
			return i
		}
	}
	return 0
}

// noIconNote tells why a NUP has no icon.
func noIconNote(err error) string {
	return "it has no icon: " + err.Error() + "; one can be given with --nup-icon, or from its card in the window"
}

// FixNupIcon gives a NUP without an icon the one of its game in titledb, or the image given
// (--nup-icon); a NUP that carries one is skipped.
func FixNupIcon(ctx context.Context, path string, o Options) (string, error) {
	container, err := nup.Open(path)
	if err != nil {
		return "", err
	}
	if container.Icon() != nil {
		return "", Skipped(hasIcon)
	}
	pkg := container.Packages[container.IconPackage()]
	icon, from, err := iconOf(ctx, o, pkg.TitleID)
	if err != nil {
		return "", err
	}
	set, err := nup.SetIcon(path, icon)
	switch {
	case err != nil:
		return "", err
	case !set:
		return "", Skipped(hasIcon)
	}
	return "it carries " + from + " now, 256×256: the files of the game as they were", nil
}

// hasIcon is why --fix-nup-icon leaves a NUP as it is.
const hasIcon = "it carries an icon already: it is kept"

package titledb

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dtrunk90/switch-library-manager-web/settings"
)

// The DLC of each game, from titles.dlc.json of the data release of the library app: the window
// tells with them the DLC of the games of its list that are not in it (#160). versions.json does
// not do, as it lists only the DLC that had a new version; titles.dlc.json is made from
// titles.json, which has them all, and is a fraction of its size (2 MB, 400 KB compressed). The
// window downloads it with versions.json, once a day at most and only when its settings allow it.
const (
	DLCFile  = "titles.dlc.json"
	dlcState = "titles.dlc.state.json"
)

// DLCAddresses are where titles.dlc.json is downloaded from, compressed first.
var DLCAddresses = settings.CompressedFirst(settings.DATA_RELEASE_URL + DLCFile)

// DLC is a DLC of a game: its title ID (16 hexadecimal digits, upper case), its name and the day
// it came out (2025-02-04), each when titledb has it.
type DLC struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Date string `json:"date,omitempty"`
}

// GameDLC are the DLC of a game: the ones out, and the ones that come out later.
type GameDLC struct {
	Out  []DLC `json:"out"`
	Soon []DLC `json:"soon,omitempty"`
}

// DLCs are the DLC of each game, by the title ID of the game.
type DLCs struct {
	games map[uint64][]DLC
}

// Of gives the DLC of a game the list knows, by the title ID of the game: the ones out on the day
// given (2026-11-20) and the ones that come out after it, oldest first. A DLC without a day is
// out, as the library app counts it.
func (d *DLCs) Of(game uint64, today string) (GameDLC, bool) {
	if d == nil {
		return GameDLC{}, false
	}
	all, ok := d.games[game]
	if !ok {
		return GameDLC{}, false
	}
	result := GameDLC{Out: []DLC{}}
	for _, dlc := range all {
		if dlc.Date != "" && dlc.Date > today {
			result.Soon = append(result.Soon, dlc)
		} else {
			result.Out = append(result.Out, dlc)
		}
	}
	return result, true
}

// LoadDLCs reads the copy kept in a folder, without going on the internet; it fails when there is
// none.
func LoadDLCs(folder string) (*DLCs, error) {
	data, err := os.ReadFile(filepath.Join(folder, DLCFile))
	if err != nil {
		return nil, err
	}
	games, err := parseDLCs(data)
	if err != nil {
		return nil, err
	}
	return &DLCs{games: games}, nil
}

// RefreshDLCs gives the DLC of each game, downloading them again into folder when the copy kept is
// a day old or more, as Refresh does with the versions. With no copy and no server giving one
// (before the data release has the file, or with no connection), the DLC are nil.
func RefreshDLCs(ctx context.Context, folder string, client *http.Client, now time.Time) (*DLCs, error) {
	kept, err := LoadDLCs(folder)
	data, _, err := refreshFile(ctx, folder, client, now, DLCFile, dlcState, DLCAddresses, err == nil, func(data []byte) error {
		_, err := parseDLCs(data)
		return err
	})
	if data == nil {
		return kept, err
	}
	games, _ := parseDLCs(data)
	return &DLCs{games: games}, err
}

// ForgetDLCs removes the copy kept and what was remembered of it.
func ForgetDLCs(folder string) error {
	return errors.Join(remove(filepath.Join(folder, DLCFile)), remove(filepath.Join(folder, dlcState)))
}

// parseDLCs reads titles.dlc.json: {"<game>": {"<DLC>": {"name": "...", "releaseDate": 20250204}}}.
// A DLC whose title ID is not of a DLC of that game is left out, and so is a day that is not one.
func parseDLCs(data []byte) (map[uint64][]DLC, error) {
	var list map[string]map[string]struct {
		Name        string `json:"name"`
		ReleaseDate int    `json:"releaseDate"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", DLCFile, err)
	}
	games := make(map[uint64][]DLC, len(list))
	for key, dlcs := range list {
		game, err := strconv.ParseUint(key, 16, 64)
		if err != nil || len(key) != 16 || game&0x1FFF != 0 {
			continue
		}
		for dlcKey, item := range dlcs {
			id, err := strconv.ParseUint(dlcKey, 16, 64)
			// a DLC is its game with bit 12 and an index from 1 (see GameID)
			if err != nil || len(dlcKey) != 16 || GameID(id) != game || id&0x1000 == 0 || id&0xFFF == 0 {
				continue
			}
			dlc := DLC{ID: fmt.Sprintf("%016X", id), Name: cleanName(item.Name)}
			if day, err := time.Parse("20060102", strconv.Itoa(item.ReleaseDate)); err == nil && item.ReleaseDate > 0 {
				dlc.Date = day.Format(time.DateOnly)
			}
			games[game] = append(games[game], dlc)
		}
	}
	for game, dlcs := range games {
		// the oldest first, the ones without a day last, then by title ID
		slices.SortFunc(dlcs, func(a, b DLC) int {
			if (a.Date == "") != (b.Date == "") {
				if a.Date == "" {
					return 1
				}
				return -1
			}
			return cmp.Or(strings.Compare(a.Date, b.Date), strings.Compare(a.ID, b.ID))
		})
		games[game] = dlcs
	}
	if len(games) == 0 {
		return nil, fmt.Errorf("%s lists no DLC", DLCFile)
	}
	return games, nil
}

// cleanName is a name of a list without the marks it lost, control characters and runs of
// spaces; "" when it is too long to be a name.
func cleanName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F || r == utf8.RuneError {
			return -1
		}
		return r
	}, strings.ToValidUTF8(name, ""))
	name = strings.Join(strings.Fields(name), " ")
	if len(name) > 0x200 {
		return ""
	}
	return name
}

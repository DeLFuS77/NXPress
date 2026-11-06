package titledb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dtrunk90/switch-library-manager-web/settings"
)

// The names of the games of the Switch 2, from titles.switch2.json of the data release of the
// library app (a list of switchbrew, 13 KB compressed): a dump folder without control data has
// no name of its own, and its NUP takes the one of this list, so emulators do not show it blank
// or with the name of the folder. The window downloads it with versions.json, once a day at most
// and only when its settings allow it; the command line only reads the copy kept.
const (
	NamesFile  = "titles.switch2.json"
	namesState = "titles.switch2.state.json"
)

// NameAddresses are where titles.switch2.json is downloaded from, compressed first.
var NameAddresses = settings.CompressedFirst(settings.DATA_RELEASE_URL + NamesFile)

// Names are the names of the games of the Switch 2, by the title ID of each game.
type Names struct {
	names map[uint64]string
}

// Name gives the name of a game of the Switch 2, by its title ID or the one of its update.
func (n *Names) Name(id uint64) (string, bool) {
	if n == nil {
		return "", false
	}
	name, ok := n.names[id&^0xFFF]
	return name, ok
}

// LoadNames reads the copy kept in a folder, without going on the internet; it fails when there
// is none.
func LoadNames(folder string) (*Names, error) {
	data, err := os.ReadFile(filepath.Join(folder, NamesFile))
	if err != nil {
		return nil, err
	}
	names, err := parseNames(data)
	if err != nil {
		return nil, err
	}
	return &Names{names: names}, nil
}

// RefreshNames gives the names of the games of the Switch 2, downloading them again into folder
// when the copy kept is a day old or more, as Refresh does with the versions.
func RefreshNames(ctx context.Context, folder string, client *http.Client, now time.Time) (*Names, error) {
	kept, err := LoadNames(folder)
	data, _, err := refreshFile(ctx, folder, client, now, NamesFile, namesState, NameAddresses, err == nil, func(data []byte) error {
		_, err := parseNames(data)
		return err
	})
	if data == nil {
		return kept, err
	}
	names, _ := parseNames(data)
	return &Names{names: names}, err
}

// ForgetNames removes the copy kept and what was remembered of it.
func ForgetNames(folder string) error {
	return errors.Join(remove(filepath.Join(folder, NamesFile)), remove(filepath.Join(folder, namesState)))
}

// parseNames reads titles.switch2.json: {"titles": {"<title ID>": {"name": "..."}}}.
func parseNames(data []byte) (map[uint64]string, error) {
	var list struct {
		Titles map[string]struct {
			Name string `json:"name"`
		} `json:"titles"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", NamesFile, err)
	}
	names := make(map[uint64]string, len(list.Titles))
	for key, title := range list.Titles {
		id, err := strconv.ParseUint(key, 16, 64)
		// the marks the list lost (LEGO® came as a replacement character) and control characters go
		name := strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7F || r == utf8.RuneError {
				return -1
			}
			return r
		}, strings.ToValidUTF8(title.Name, ""))
		name = strings.Join(strings.Fields(name), " ")
		if err != nil || len(key) != 16 || name == "" || len(name) > 0x200 {
			continue
		}
		names[id&^0xFFF] = name
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s lists no game", NamesFile)
	}
	return names, nil
}

package jobs

import (
	"sort"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/nup"
)

// CardPart is a title a file holds, as the card of a game over its name tells it: its kind (BASE,
// UPD or DLC), its title ID and its version (the internal one, 65536), and the version the game
// shows (the external one, 1.0.1) of a game and of an update: a DLC has none.
type CardPart struct {
	Kind    string `json:"kind"`
	TitleID string `json:"titleId"`
	Version int    `json:"version"`
	Shown   string `json:"shown,omitempty"`
}

// Parts are the titles of a file in the order the card tells them: the games, then the updates,
// then the DLC, each by title ID. A dump folder is not read here: the window has its packages.
func Parts(path string) ([]CardPart, error) {
	var parts []CardPart
	if isNup(path) {
		container, err := nup.Open(path)
		if err != nil {
			return nil, err
		}
		for _, pkg := range container.Packages {
			part := CardPart{Kind: pkg.KindName(), TitleID: pkg.TitleIDText(), Version: int(pkg.Version)}
			if pkg.Kind != nup.KindAddon {
				part.Shown = pkg.DisplayVersion()
			}
			parts = append(parts, part)
		}
	} else {
		titles, err := ReadTitles(path)
		if err != nil {
			return nil, err
		}
		for _, title := range titles {
			part := CardPart{Kind: title.Type, TitleID: strings.ToUpper(title.TitleId), Version: int(title.Version)}
			if title.Type != "DLC" && title.Ncap != nil {
				part.Shown = strings.TrimSpace(title.Ncap.DisplayVersion)
			}
			parts = append(parts, part)
		}
	}
	order := map[string]int{"BASE": 0, "UPD": 1, "DLC": 2}
	sort.SliceStable(parts, func(i, j int) bool {
		if order[parts[i].Kind] != order[parts[j].Kind] {
			return order[parts[i].Kind] < order[parts[j].Kind]
		}
		return parts[i].TitleID < parts[j].TitleID
	})
	return parts, nil
}

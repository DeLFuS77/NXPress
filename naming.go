package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// gameTitle is what a game file holds, by its main title: the game, else its update, else a
// DLC.
type gameTitle struct {
	Name    string `json:"name"`
	TitleID string `json:"titleId"`
	Version int    `json:"version"`
	Type    string `json:"type"`
	Titles  int    `json:"titles"`
	// the icon of the game, as a data URL, when asked for
	Icon string `json:"icon,omitempty"`
}

// readTitle reads the main title of a game file; withIcon adds its icon.
func readTitle(path string, withIcon bool) (gameTitle, error) {
	read := switchfs.ReadNspMetadata
	if extension := strings.ToLower(filepath.Ext(path)); extension == ".xci" || extension == ".xcz" {
		read = switchfs.ReadXciMetadata
	}
	titles, err := read(path)
	if err != nil {
		return gameTitle{}, err
	}
	if len(titles) == 0 {
		return gameTitle{}, errors.New("it holds no title")
	}
	all := make([]*switchfs.ContentMetaAttributes, 0, len(titles))
	for _, title := range titles {
		all = append(all, title)
	}
	order := map[string]int{"BASE": 0, "UPD": 1, "DLC": 2}
	sort.Slice(all, func(i, j int) bool {
		if order[all[i].Type] != order[all[j].Type] {
			return order[all[i].Type] < order[all[j].Type]
		}
		return all[i].TitleId < all[j].TitleId
	})
	main := all[0]
	result := gameTitle{TitleID: strings.ToUpper(main.TitleId), Version: main.Version, Type: main.Type, Titles: len(all)}
	// the name of the game: the main title, else another title of the file that has one
	for _, title := range all {
		if title.Ncap == nil {
			continue
		}
		for _, language := range []string{"AmericanEnglish", "BritishEnglish", "Spanish", "French", "German", "Italian", "Japanese"} {
			if name := strings.TrimSpace(title.Ncap.TitleName[language].Title); name != "" {
				result.Name = name
				break
			}
		}
		if result.Name == "" {
			for _, entry := range title.Ncap.TitleName {
				if name := strings.TrimSpace(entry.Title); name != "" {
					result.Name = name
					break
				}
			}
		}
		if result.Name != "" {
			break
		}
	}
	if withIcon {
		if icon, err := switchfs.ExtractIcon(path); err == nil && len(icon) > 0 {
			result.Icon = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(icon)
		}
	}
	return result, nil
}

// defaultTemplate is the name a file gets when no template is given.
const defaultTemplate = "{name} [{id}][v{version}]"

// newName is the name of a file by a template: {name}, {id}, {version} and {type} of its main
// title, and {titles}, how many it holds. Its extension is kept.
func newName(path string, title gameTitle, template string) string {
	if template == "" {
		template = defaultTemplate
	}
	name := title.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	replacer := strings.NewReplacer("{name}", name, "{id}", title.TitleID, "{version}", fmt.Sprint(title.Version),
		"{type}", title.Type, "{titles}", fmt.Sprint(title.Titles))
	result := replacer.Replace(template)
	// what file systems do not take in a name
	result = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return -1
		}
		return r
	}, result)
	result = strings.Join(strings.Fields(result), " ")
	result = strings.TrimRight(result, ". ")
	if result == "" {
		result = title.TitleID
	}
	return result + filepath.Ext(path)
}

// rename gives a file the name of its template, in its folder.
func rename(ctx context.Context, path string, o options, out output) (string, string, error) {
	title, err := readTitle(path, false)
	if err != nil {
		return "", "", err
	}
	name := filepath.Join(filepath.Dir(path), newName(path, title, o.template))
	if name == path {
		return "", "the name is already that one", nil
	}
	if !strings.EqualFold(name, path) {
		if err := free(o, name); err != nil {
			return "", "", err
		}
	}
	if err := os.Rename(path, name); err != nil {
		return "", "", err
	}
	return name, "renamed to " + filepath.Base(name), nil
}

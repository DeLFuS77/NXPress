package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	// what a name tells of the file: the version of its newest update when it holds the game
	// and its updates (its own version else), the version as the game shows it, the titles of
	// each kind it holds and its size
	Latest     int    `json:"latest"`
	VersionTxt string `json:"versionTxt"`
	Bases      int    `json:"bases"`
	Updates    int    `json:"updates"`
	Dlcs       int    `json:"dlcs"`
	Size       int64  `json:"size"`
	// what the window shows of the game over its cover: who publishes it, the languages it
	// comes in (as tags like en-US), the firmware the game or its update needs, if it is a demo,
	// the game of an update or a DLC, and the version of its game a DLC needs
	Publisher string   `json:"publisher,omitempty"`
	Languages []string `json:"languages,omitempty"`
	Firmware  string   `json:"firmware,omitempty"`
	Demo      bool     `json:"demo,omitempty"`
	GameID    string   `json:"gameId,omitempty"`
	NeedsGame int      `json:"needsGame,omitempty"`
	// the icon of the game, as a data URL, when asked for
	Icon string `json:"icon,omitempty"`
}

// readTitles reads the titles a game file holds.
func readTitles(path string) ([]*switchfs.ContentMetaAttributes, error) {
	read := switchfs.ReadNspMetadata
	if extension := strings.ToLower(filepath.Ext(path)); extension == ".xci" || extension == ".xcz" {
		read = switchfs.ReadXciMetadata
	}
	titles, err := read(path)
	if err != nil {
		return nil, err
	}
	if len(titles) == 0 {
		return nil, errors.New("it holds no title")
	}
	all := make([]*switchfs.ContentMetaAttributes, 0, len(titles))
	for _, title := range titles {
		all = append(all, title)
	}
	return all, nil
}

// readTitle reads the main title of a game file; withIcon adds its icon.
func readTitle(path string, withIcon bool) (gameTitle, error) {
	all, err := readTitles(path)
	if err != nil {
		return gameTitle{}, err
	}
	result := describeTitles(all)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		result.Size = info.Size()
	}
	if withIcon {
		if icon, err := switchfs.ExtractIcon(path); err == nil && len(icon) > 0 {
			result.Icon = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(icon)
		}
	}
	return result, nil
}

// describeTitles tells what titles tell together: the main one (the game, else its update,
// else a DLC), its name, its versions and how many of each kind there are.
func describeTitles(all []*switchfs.ContentMetaAttributes) gameTitle {
	order := map[string]int{"BASE": 0, "UPD": 1, "DLC": 2}
	sort.Slice(all, func(i, j int) bool {
		if order[all[i].Type] != order[all[j].Type] {
			return order[all[i].Type] < order[all[j].Type]
		}
		return all[i].TitleId < all[j].TitleId
	})
	main := all[0]
	result := gameTitle{TitleID: strings.ToUpper(main.TitleId), Version: main.Version, Type: main.Type, Titles: len(all), Latest: main.Version}
	// the version a name tells: the newest update of the game when the file holds it, else
	// the version of the main title; as the game shows it, from its control data (a DLC has
	// none: 1.0.0 for its first version)
	shown := main
	for _, title := range all {
		switch title.Type {
		case "BASE":
			result.Bases++
		case "UPD":
			result.Updates++
			if main.Type == "BASE" && title.Version > result.Latest {
				result.Latest, shown = title.Version, title
			}
		case "DLC":
			result.Dlcs++
		}
	}
	if shown.Ncap != nil {
		result.VersionTxt = strings.TrimSpace(shown.Ncap.DisplayVersion)
	}
	if shown.Type != "DLC" && shown.RequiredTitleVersion > 0 {
		result.Firmware = firmware(uint32(shown.RequiredTitleVersion))
	}
	// an update or a DLC tells its game, and a DLC the version of its game it needs
	if main.Type != "BASE" {
		result.GameID = fmt.Sprintf("%016X", baseTitleID(main))
	}
	if main.Type == "DLC" {
		result.NeedsGame = main.RequiredTitleVersion
	}
	if result.VersionTxt == "" && main.Type == "DLC" && main.Version == 0 {
		result.VersionTxt = "1.0.0"
	}
	// the name of the game: the main title, else another title of the file that has one
	for _, title := range all {
		if title.Ncap == nil {
			continue
		}
		for _, language := range []string{"AmericanEnglish", "BritishEnglish", "Spanish", "French", "German", "Italian", "Japanese"} {
			if name := strings.TrimSpace(title.Ncap.TitleName[language].Title); name != "" {
				result.Name = name
				result.Publisher = strings.TrimSpace(title.Ncap.TitleName[language].Publisher)
				break
			}
		}
		if result.Name == "" {
			for _, entry := range title.Ncap.TitleName {
				if name := strings.TrimSpace(entry.Title); name != "" {
					result.Name = name
					result.Publisher = strings.TrimSpace(entry.Publisher)
					break
				}
			}
		}
		if result.Name != "" {
			// bit n of the flag is the language n of the control data
			for language := switchfs.Language(0); language < 16; language++ {
				if title.Ncap.SupportedLanguageFlag&(1<<uint(language)) != 0 {
					result.Languages = append(result.Languages, language.ToLanguageTag())
				}
			}
			result.Demo = title.Ncap.Demo
			break
		}
	}
	return result
}

// baseTitleID is the title ID of the game a title belongs to: an update is the game with 0x800
// added, a DLC the game with 0x1000 and its number added.
func baseTitleID(title *switchfs.ContentMetaAttributes) uint64 {
	id, _ := strconv.ParseUint(title.TitleId, 16, 64)
	switch title.Type {
	case "UPD":
		return id &^ 0xFFF
	case "DLC":
		return id&^0xFFF - 0x1000
	}
	return id
}

// trailingTags are the tags at the end of a name the templates write, so a name taken from the
// file does not get them twice: [0100...][v0][US], (v65536), (v1.0.5), (1G+1U+2D), 2.7GB, 600MB
var trailingTags = regexp.MustCompile(`(\s*(\[[^\]]*\]|\(v[0-9.]*\)|\([0-9]+[GUD](\+[0-9]+[GUD])*\)|[0-9]+(\.[0-9])?[GM]B))+$`)

// defaultTemplate is the name a file gets when no template is given.
const defaultTemplate = "{name} [{id}][v{version}]"

// contents tells the titles of each kind a file holds, as Switch Library Manager writes it:
// 1G+1U+2D for a game, its update and two DLC; the kinds it has none of are left out.
func (title gameTitle) contents() string {
	parts := []string{}
	for _, kind := range []struct {
		count  int
		letter string
	}{{title.Bases, "G"}, {title.Updates, "U"}, {title.Dlcs, "D"}} {
		if kind.count > 0 {
			parts = append(parts, fmt.Sprintf("%d%s", kind.count, kind.letter))
		}
	}
	return strings.Join(parts, "+")
}

// emptyPairs are brackets left with nothing in them, a field that had no value
var emptyPairs = regexp.MustCompile(`\(\s*\)|\[\s*\]|<\s*>|\{\s*\}`)

// newName is the name of a file by a template. Its fields, in lower case or as Switch Library
// Manager writes them: {name} {TITLE_NAME}, {id} {TITLE_ID}, {version} {VERSION} (of the newest
// update for a game with its updates), {version_txt} {VERSION_TXT} (as the game shows it,
// 1.0.5), {type} {TYPE}, {titles}, {contents} {PACKAGE_CONTENTS} (1G+1U+2D), {size_gb}
// {SIZE_GB}, {size_mb} {SIZE_MB} and {dlc_number} {DLC_NUMBER} (1 for the first DLC of its
// game, empty for other files). {REGION} and {DLC_NAME}, which that program takes from
// its database of titles, are left empty, as brackets left with nothing. Its extension is kept.
func newName(path string, title gameTitle, template string) string {
	if template == "" {
		template = defaultTemplate
	}
	name := title.Name
	if name == "" {
		// the name of the file, without the [title ID][version] tags the template adds again
		name = strings.TrimSpace(trailingTags.ReplaceAllString(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), ""))
	}
	sizeGB, sizeMB := "", ""
	if title.Size > 0 {
		// decimal units, rounded half up, as Switch Library Manager writes them
		tenths := (title.Size + 50_000_000) / 100_000_000
		sizeGB = fmt.Sprintf("%d.%dGB", tenths/10, tenths%10)
		sizeMB = fmt.Sprintf("%dMB", (title.Size+500_000)/1_000_000)
	}
	fields := []string{
		"{name}", name, "{id}", title.TitleID, "{version}", fmt.Sprint(max(title.Latest, title.Version)), "{version_txt}", title.VersionTxt,
		"{type}", title.Type, "{titles}", fmt.Sprint(title.Titles), "{contents}", title.contents(),
		"{size_gb}", sizeGB, "{size_mb}", sizeMB, "{dlc_number}", "",
	}
	if title.Type == "DLC" && title.Titles == 1 {
		fields[len(fields)-1] = dlcNumber(title.TitleID)
	}
	// the same fields as Switch Library Manager writes them, added after the ones above
	lower := len(fields)
	for i := 0; i < lower; i += 2 {
		upper := map[string]string{"{name}": "{TITLE_NAME}", "{id}": "{TITLE_ID}", "{contents}": "{PACKAGE_CONTENTS}"}[fields[i]]
		if upper == "" {
			upper = strings.ToUpper(fields[i])
		}
		if upper != "{TITLES}" {
			fields = append(fields, upper, fields[i+1])
		}
	}
	fields = append(fields, "{REGION}", "", "{region}", "", "{DLC_NAME}", "", "{dlc_name}", "")
	result := strings.NewReplacer(fields...).Replace(template)
	result = emptyPairs.ReplaceAllString(result, "")
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

// dlcNumber is the number of a DLC among the ones of its game: its title ID is the one of the
// game with 0x1000 added and the number, 1 for the first. "" when id is not one.
func dlcNumber(id string) string {
	value, err := strconv.ParseUint(id, 16, 64)
	if err != nil || value&0x1000 == 0 || value&0xFFF == 0 {
		return ""
	}
	return strconv.FormatUint(value&0xFFF, 10)
}

// keepsName tells whether a file keeps its name with --keep-named: it has its title ID in
// brackets, in any case.
func keepsName(path string, id string) bool {
	return id != "" && strings.Contains(strings.ToUpper(filepath.Base(path)), "["+strings.ToUpper(id)+"]")
}

// rename gives a file the name of its template, in its folder.
func rename(ctx context.Context, path string, o options, out output) (string, string, error) {
	title, err := readTitle(path, false)
	if err != nil {
		return "", "", err
	}
	if o.keepNamed && keepsName(path, title.TitleID) {
		return "", "its name has its title ID, it is kept", nil
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

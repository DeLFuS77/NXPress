package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/export"
	"github.com/DeLFuS77/NXPress/internal/jobs"
	"github.com/DeLFuS77/NXPress/internal/titledb"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The lists the window exports to a spreadsheet (#89): Details, a row for each title of each
// file, and the log as the page shows it.

// ExportTexts are the words of the page for an export, in its language: the name of the sheet
// and of the file proposed, the header, and the names of the kinds of title (BASE, UPD, DLC).
type ExportTexts struct {
	Sheet  string            `json:"sheet"`
	File   string            `json:"file"`
	Header []string          `json:"header"`
	Kinds  map[string]string `json:"kinds"`
}

// ExportDetails asks where to save what Details tells of the files given, as XLSX or CSV, and
// writes it; it gives the file written, "" when none was chosen.
func (a *App) ExportDetails(paths []string, format string, texts ExportTexts) (string, error) {
	path, err := a.exportPath(texts.File, format)
	if err != nil || path == "" {
		return "", err
	}
	done, err := a.chore(true)
	if err != nil {
		return "", err
	}
	defer done()
	table := export.Table{Sheet: texts.Sheet, Header: texts.Header, Rows: detailsRows(paths, a.versions.Load(), texts.Kinds)}
	return path, export.Write(path, table)
}

// ExportLog asks where to save the works of the log the page shows, one row for each file, and
// writes them.
func (a *App) ExportLog(format string, texts ExportTexts, rows [][]any) (string, error) {
	path, err := a.exportPath(texts.File, format)
	if err != nil || path == "" {
		return "", err
	}
	done, err := a.chore(true)
	if err != nil {
		return "", err
	}
	defer done()
	return path, export.Write(path, export.Table{Sheet: texts.Sheet, Header: texts.Header, Rows: rows})
}

// exportPath asks where to save a file of the format, with the name proposed.
func (a *App) exportPath(name string, format string) (string, error) {
	extension, filter := ".csv", wailsruntime.FileFilter{DisplayName: "CSV (*.csv)", Pattern: "*.csv"}
	if format == "xlsx" {
		extension, filter = ".xlsx", wailsruntime.FileFilter{DisplayName: "Excel (*.xlsx)", Pattern: "*.xlsx"}
	}
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename: safeName(name) + extension, Filters: []wailsruntime.FileFilter{filter}, CanCreateDirectories: true,
	})
	if err != nil || path == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(path), extension) {
		path += extension
	}
	return path, nil
}

// safeName is a name a file can have on every system.
func safeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return ' '
		}
		return r
	}, name)
	if name = strings.TrimSpace(name); name == "" {
		return "NXPress"
	}
	return name
}

// detailsRows are the rows of the export of Details, in the order of the files given: for each
// title of a file, the file, its format and size, the title (its ID, kind, version as a number
// and as the game shows it, name and publisher), the newest version published when the list of
// versions knows it, and the folder; a file that cannot be read gets one row, which tells why.
func detailsRows(paths []string, versions *titledb.Versions, kinds map[string]string) [][]any {
	rows := [][]any{}
	for _, path := range paths {
		size := int64(0)
		if info, err := os.Stat(path); err == nil {
			size = info.Size()
		}
		file := []any{filepath.Base(path), strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), ".")), export.Bytes(size)}
		titles, err := exportTitles(path)
		if err != nil {
			rows = append(rows, append(file, nil, nil, nil, nil, nil, nil, nil, filepath.Dir(path), err.Error()))
			continue
		}
		for _, title := range titles {
			kind := title.kind
			if name, ok := kinds[kind]; ok && name != "" {
				kind = name
			}
			var newest any
			if published, ok := newestPublished(title.id, title.kind, versions); ok {
				newest = int64(published.Version)
			}
			rows = append(rows, append(append([]any{}, file...), title.id, kind, int64(title.version), title.shown, title.name, title.publisher, newest, filepath.Dir(path), nil))
		}
	}
	return rows
}

// exportTitle is a title of a file, as the export tells it.
type exportTitle struct {
	id, kind, shown, name, publisher string
	version                          int
}

// exportTitles reads the titles of a file, by their IDs: a DLC, which has no name of its own,
// takes the one of its game when the file holds it.
func exportTitles(path string) ([]exportTitle, error) {
	titles := []exportTitle{}
	if strings.EqualFold(filepath.Ext(path), ".nup") {
		card, err := jobs.NupCard(path)
		if err != nil {
			return nil, err
		}
		for _, pkg := range card.Packages {
			titles = append(titles, exportTitle{id: strings.ToUpper(pkg.TitleID), kind: pkg.Type, version: pkg.Version, shown: pkg.Shown, name: pkg.Name, publisher: pkg.Publisher})
		}
	} else {
		read, err := jobs.ReadTitles(path)
		if err != nil {
			return nil, err
		}
		for _, title := range read {
			found := exportTitle{id: strings.ToUpper(title.TitleId), kind: title.Type, version: title.Version}
			if title.Ncap != nil {
				found.shown = title.Ncap.DisplayVersion
				found.name, found.publisher = nacpName(title.Ncap)
			}
			titles = append(titles, found)
		}
	}
	if len(titles) == 0 {
		return nil, errors.New("it holds no title")
	}
	sort.Slice(titles, func(i, j int) bool { return titles[i].id < titles[j].id })
	games := map[uint64]string{}
	for _, title := range titles {
		if game, err := switchfs.GameTitleID(title.id, title.kind); err == nil && title.name != "" && title.kind != "DLC" {
			games[game] = title.name
		}
	}
	for i, title := range titles {
		if game, err := switchfs.GameTitleID(title.id, title.kind); err == nil && title.name == "" {
			titles[i].name = games[game]
		}
	}
	return titles, nil
}

// nacpName is the name and the publisher of a game: in English when it has them, else in the
// first language that has a name.
func nacpName(nacp *switchfs.Nacp) (string, string) {
	if title, ok := nacp.TitleName["AmericanEnglish"]; ok && strings.TrimSpace(title.Title) != "" {
		return strings.TrimSpace(title.Title), strings.TrimSpace(title.Publisher)
	}
	languages := make([]string, 0, len(nacp.TitleName))
	for language := range nacp.TitleName {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	for _, language := range languages {
		if title := nacp.TitleName[language]; strings.TrimSpace(title.Title) != "" {
			return strings.TrimSpace(title.Title), strings.TrimSpace(title.Publisher)
		}
	}
	return "", ""
}

// newestPublished is the newest version published of a title: the newest update of its game
// for a game or an update, its own newest version for a DLC.
func newestPublished(id string, kind string, versions *titledb.Versions) (titledb.Published, bool) {
	number, err := strconv.ParseUint(id, 16, 64)
	if err != nil || versions == nil {
		return titledb.Published{}, false
	}
	if kind == "UPD" {
		if number, err = switchfs.GameTitleID(id, kind); err != nil {
			return titledb.Published{}, false
		}
	}
	return versions.Newest(number)
}

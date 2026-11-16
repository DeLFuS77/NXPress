package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/portable"
	"github.com/DeLFuS77/NXPress/internal/system"

	"github.com/wailsapp/wails/v2"
	wailsoptions "github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The details of a file in a window of its own, out of the window of NXPress: a window is one
// program here, so View Details starts another NXPress (--details-view and a file with what to show)
// that opens only this one. It is a window of the system: it moves, sizes, minimizes and has its
// button in the taskbar, and it stays when the window of NXPress is closed.

//go:embed all:detailsview
var detailsAssets embed.FS

// detailsPrefix begins the name of the file that gives a window of details what to show.
const detailsPrefix = "nxpress-details-"

// maxDetails is the most text a window of details takes.
const maxDetails = 4 << 20

// DetailsData is what a window of details shows: its title, the text, the words of its buttons (copy
// and export), what it says once the text is saved ({name} is the file), and the name the file is
// offered under.
type DetailsData struct {
	Title    string `json:"title"`
	Text     string `json:"text"`
	Copy     string `json:"copy"`
	Copied   string `json:"copied"`
	Export   string `json:"export"`
	Saved    string `json:"saved"`
	FileName string `json:"fileName"`
}

// DetailsView is what the page of a window of details asks for.
type DetailsView struct {
	data DetailsData
	ctx  context.Context
}

// Data is what the window shows.
func (v *DetailsView) Data() DetailsData { return v.data }

// Export asks where to save the text, as a .txt file, and writes it; it gives the file written, "" when
// none was chosen.
func (v *DetailsView) Export() (string, error) {
	if v.ctx == nil {
		return "", errors.New("the window is not ready")
	}
	path, err := wailsruntime.SaveFileDialog(v.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename:      safeName(v.data.FileName) + ".txt",
		Filters:              []wailsruntime.FileFilter{{DisplayName: "Text (*.txt)", Pattern: "*.txt"}},
		CanCreateDirectories: true,
	})
	if err != nil || path == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(path), ".txt") {
		path += ".txt"
	}
	text := v.data.Text
	if runtime.GOOS == "windows" {
		// Notepad of older Windows shows lines ended by a lone LF as one
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// detailsPath tells whether a path is a file made for a window of details: in the temporary folder,
// by its name; the window takes no other.
func detailsPath(path string) bool {
	clean := filepath.Clean(path)
	return filepath.Dir(clean) == filepath.Clean(os.TempDir()) && strings.HasPrefix(filepath.Base(clean), detailsPrefix)
}

// writeDetails makes the file that tells a window of details what to show, and gives its path.
func writeDetails(data DetailsData) (string, error) {
	if len(data.Text) > maxDetails {
		return "", errors.New("the details are too long for a window")
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", detailsPrefix+"*.json")
	if err != nil {
		return "", err
	}
	_, err = file.Write(encoded)
	if closed := file.Close(); err == nil {
		err = closed
	}
	if err != nil {
		os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

// readDetails reads the file made for a window of details, and removes it.
func readDetails(path string) (DetailsData, error) {
	var data DetailsData
	if !detailsPath(path) {
		return data, fmt.Errorf("%s is not a file of details", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return data, err
	}
	encoded, err := io.ReadAll(io.LimitReader(file, maxDetails+(64<<10)))
	file.Close()
	os.Remove(path)
	if err != nil {
		return data, err
	}
	if err := json.Unmarshal(encoded, &data); err != nil {
		return data, err
	}
	return data, nil
}

// OpenDetailsWindow shows the details of a file in a window of its own, out of this one.
func (a *App) OpenDetailsWindow(title string, text string, copyLabel string, copiedLabel string, exportLabel string, savedLabel string, fileName string) error {
	path, err := writeDetails(DetailsData{Title: title, Text: text, Copy: copyLabel, Copied: copiedLabel, Export: exportLabel, Saved: savedLabel, FileName: fileName})
	if err != nil {
		return err
	}
	program, err := system.Executable()
	if err == nil {
		err = system.StartDetached(program, "--details-view", path)
	}
	if err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

// runDetailsView is the window of details a file tells (see OpenDetailsWindow), and gives the exit
// code once it is closed.
func runDetailsView(path string) int {
	data, err := readDetails(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nxpress:", err)
		return 2
	}
	assets, err := fs.Sub(detailsAssets, "detailsview")
	if err != nil {
		fmt.Fprintln(os.Stderr, "nxpress:", err)
		return 1
	}
	title := strings.TrimSpace(data.Title)
	if title == "" {
		title = "NXPress"
	}
	view := &DetailsView{data: data}
	err = wails.Run(&wailsoptions.App{
		Title:            title,
		Width:            860,
		Height:           640,
		MinWidth:         420,
		MinHeight:        280,
		BackgroundColour: &wailsoptions.RGBA{R: 24, G: 24, B: 27, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        func(ctx context.Context) { view.ctx = ctx },
		Bind:             []any{view},
		Linux:            &linux.Options{Icon: appIcon, ProgramName: "nxpress"},
		Windows:          &windows.Options{WebviewUserDataPath: portable.WebviewFolder()},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "nxpress:", err)
		return 1
	}
	return 0
}

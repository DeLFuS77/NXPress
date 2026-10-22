package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	wailsoptions "github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend
var frontend embed.FS

// the icon of the window on Linux; Windows takes it from the resources of the program, macOS
// from its app
//
//go:embed build/appicon.png
var appIcon []byte

// runWindow opens the window with the files given, and gives the exit code once it is closed.
func runWindow(files []string) int {
	app := &App{files: files}
	err := wails.Run(&wailsoptions.App{
		Title:            "NXPress",
		Width:            1120,
		Height:           720,
		MinWidth:         900,
		MinHeight:        520,
		BackgroundColour: &wailsoptions.RGBA{R: 24, G: 24, B: 27, A: 255},
		AssetServer:      &assetserver.Options{Assets: frontend},
		OnStartup:        app.startup,
		OnShutdown:       func(context.Context) { app.Cancel() },
		DragAndDrop:      &wailsoptions.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true},
		Bind:             []any{app},
		Linux:            &linux.Options{Icon: appIcon, ProgramName: "nxpress"},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "nxpress:", err)
		return 1
	}
	return 0
}

// App is what the window asks for: its methods are called from the page.
type App struct {
	ctx    context.Context
	mutex  sync.Mutex
	keys   string
	cancel context.CancelFunc
	// the files NXPress was started with
	files []string
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// the keys of the usual places are read at once, as the command line does
	if path, err := loadKeys(""); err == nil {
		a.keys = path
	}
}

// Setup is what the page shows before anything is chosen.
type Setup struct {
	Version  string `json:"version"`
	Keys     string `json:"keys"`
	Threads  int    `json:"threads"`
	Cpus     int    `json:"cpus"`
	Level    int    `json:"level"`
	MaxLevel int    `json:"maxLevel"`
	// the files NXPress was started with, and the mode they suit
	Files []string `json:"files"`
	Mode  string   `json:"mode"`
	// whether "Send to" can hold NXPress, and holds it
	SendTo        bool `json:"sendTo"`
	SendToEnabled bool `json:"sendToEnabled"`
}

func (a *App) Setup() Setup {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	setup := Setup{
		Version: version, Keys: a.keys, Threads: max(1, min(8, runtime.NumCPU()/2)), Cpus: runtime.NumCPU(),
		Level: defaultLevel, MaxLevel: maxLevel, Files: a.files, SendTo: sendToFolder() != "",
	}
	setup.SendToEnabled = setup.SendTo && sendToEnabled()
	if len(a.files) > 0 {
		setup.Mode = "decompress"
		if files, _ := inputs(a.files, []string{".nsp", ".xci"}); len(files) > 0 {
			setup.Mode = "compress"
		}
	}
	return setup
}

// SetSendTo puts NXPress in the "Send to" menu of the Explorer, or takes it away.
func (a *App) SetSendTo(on bool) error {
	if on {
		return enableSendTo()
	}
	return disableSendTo()
}

// ChooseKeys asks for a prod.keys or a keys.txt, and makes it active.
func (a *App) ChooseKeys() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "prod.keys",
		Filters: []wailsruntime.FileFilter{{DisplayName: "Keys (*.keys, *.txt)", Pattern: "*.keys;*.txt"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if _, err := loadKeys(path); err != nil {
		return "", err
	}
	a.mutex.Lock()
	a.keys = path
	a.mutex.Unlock()
	return path, nil
}

// Item is a file of the list.
type Item struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// extensions are the files a mode takes.
func extensions(mode string) []string {
	if mode == "pack" {
		return []string{".nsp", ".nsz", ".xci", ".xcz"}
	}
	return takes[mode]
}

// Expand gives the files a mode takes among the paths given, the files of the folders given
// included.
func (a *App) Expand(paths []string, mode string) []Item {
	files, _ := inputs(paths, extensions(mode))
	items := make([]Item, 0, len(files))
	for _, path := range files {
		item := Item{Path: path, Name: filepath.Base(path)}
		if info, err := os.Stat(path); err == nil {
			item.Size = info.Size()
		}
		items = append(items, item)
	}
	return items
}

// ChooseFiles asks for files of the mode.
func (a *App) ChooseFiles(mode string) ([]Item, error) {
	patterns := []string{}
	for _, extension := range extensions(mode) {
		patterns = append(patterns, "*"+extension)
	}
	paths, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Filters: []wailsruntime.FileFilter{{DisplayName: strings.ToUpper(strings.Join(patterns, ", ")), Pattern: strings.Join(patterns, ";")}},
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	return a.Expand(paths, mode), nil
}

// ChooseFolder asks for a folder, and gives the files of the mode in it.
func (a *App) ChooseFolder(mode string) ([]Item, error) {
	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{})
	if err != nil || path == "" {
		return nil, err
	}
	return a.Expand([]string{path}, mode), nil
}

// ChooseOutput asks for the folder of the new files.
func (a *App) ChooseOutput() (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{CanCreateDirectories: true})
}

// ChoosePack asks for the name of the multi-title file a pack writes.
func (a *App) ChoosePack() (string, error) {
	return wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename: "Pack.nsp",
		Filters:         []wailsruntime.FileFilter{{DisplayName: "NSP, NSZ, XCI, XCZ", Pattern: "*.nsp;*.nsz;*.xci;*.xcz"}},
	})
}

// ChooseDat asks for a DAT file.
func (a *App) ChooseDat() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Filters: []wailsruntime.FileFilter{{DisplayName: "DAT (*.dat, *.xml)", Pattern: "*.dat;*.xml"}},
	})
}

// Request is the work the page asks for.
type Request struct {
	Mode         string   `json:"mode"`
	Files        []string `json:"files"`
	Level        int      `json:"level"`
	Block        bool     `json:"block"`
	Threads      int      `json:"threads"`
	Multi        int      `json:"multi"`
	Output       string   `json:"output"`
	Overwrite    bool     `json:"overwrite"`
	RemoveSource bool     `json:"removeSource"`
	Pack         string   `json:"pack"`
	Dat          string   `json:"dat"`
}

// Start does the work in the background: the page follows it by the events "progress", "done"
// and "say", and "finished" once every file is done.
func (a *App) Start(request Request) error {
	o := options{
		level: request.Level, block: request.Block, threads: request.Threads, multi: max(1, request.Multi),
		output: request.Output, overwrite: request.Overwrite, rmSource: request.RemoveSource, files: request.Files,
	}
	if request.Mode == "verify" {
		o.dat = request.Dat
	}
	switch request.Mode {
	case "compress":
		o.compress = true
	case "decompress":
		o.decompress = true
	case "convert":
		o.convert = true
	case "recompress":
		o.recompress = true
	case "trim":
		o.trim = true
	case "estimate":
		o.estimate = true
	case "split":
		o.split = true
	case "info":
		o.info = true
	case "pack":
		if request.Pack == "" {
			return errors.New("choose the name of the new file")
		}
		o.pack = request.Pack
	default:
		o.verify = true
	}
	if err := o.check(); err != nil {
		return err
	}
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.keys == "" && !o.recompress {
		return errors.New("no prod.keys: choose one")
	}
	if a.cancel != nil {
		return errors.New("the files are still at work")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancel = cancel
	go func() {
		started := time.Now()
		failed := runJobs(ctx, o, &window{ctx: a.ctx})
		a.mutex.Lock()
		a.cancel = nil
		a.mutex.Unlock()
		wailsruntime.EventsEmit(a.ctx, "finished", map[string]any{
			"failed": failed, "cancelled": ctx.Err() != nil, "seconds": int(time.Since(started).Seconds()),
		})
		cancel()
	}()
	return nil
}

// Cancel stops the files at work; their half written files are removed.
func (a *App) Cancel() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

// ShowFile opens the folder of a file in the file manager.
func (a *App) ShowFile(path string) {
	showFile(path)
}

// window sends the progress and the results of the files to the page.
type window struct {
	ctx context.Context
}

// how often the progress of a file is sent at most, unless its percentage changes
const windowEvery = 500 * time.Millisecond

func (w *window) progress(path string, step string) func(done int64, total int64) {
	var (
		mutex     sync.Mutex
		last      time.Time
		lastShare = int64(-1)
	)
	return func(done int64, total int64) {
		share := int64(0)
		if total > 0 {
			share = done * 1000 / total
		}
		mutex.Lock()
		now := time.Now()
		if share == lastShare || share < 1000 && now.Sub(last) < windowEvery && share-lastShare < 10 {
			mutex.Unlock()
			return
		}
		last, lastShare = now, share
		mutex.Unlock()
		wailsruntime.EventsEmit(w.ctx, "progress", map[string]any{"file": path, "step": step, "done": done, "total": total})
	}
}

func (w *window) done(path string, output string, note string, err error) {
	result := map[string]any{"file": path, "output": output, "note": note}
	// the size of the new file, for the savings the page adds up
	if info, err := os.Stat(output); output != "" && err == nil {
		result["size"] = info.Size()
	}
	if err != nil {
		result["error"] = err.Error()
		if errors.Is(err, context.Canceled) {
			result["error"] = "cancelled"
		}
	}
	wailsruntime.EventsEmit(w.ctx, "done", result)
}

func (w *window) say(format string, args ...any) {
	wailsruntime.EventsEmit(w.ctx, "say", fmt.Sprintf(format, args...))
}

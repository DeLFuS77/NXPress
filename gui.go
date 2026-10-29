package main

import (
	"context"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
	"github.com/wailsapp/wails/v2"
	wailsoptions "github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
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
	// the window opens where it was left and as big (see windowplace.go)
	place, _ := loadWindowPlace()
	// on Windows 11 the menu shows the Mica of the system through a window left clear
	mica := micaWindow()
	background := &wailsoptions.RGBA{R: 24, G: 24, B: 27, A: 255}
	backdrop := windows.Auto
	if mica {
		background.A = 0
		backdrop = windows.Mica
	}
	err := wails.Run(&wailsoptions.App{
		Title:            "NXPress " + version,
		Width:            place.Width,
		Height:           place.Height,
		MinWidth:         windowMinWidth,
		MinHeight:        windowMinHeight,
		StartHidden:      true,
		BackgroundColour: background,
		AssetServer:      &assetserver.Options{Assets: frontend},
		OnStartup:        app.startup,
		OnDomReady:       placeWindow,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.closed,
		DragAndDrop:      &wailsoptions.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true},
		Bind:             []any{app},
		Linux:            &linux.Options{Icon: appIcon, ProgramName: "nxpress"},
		// portable (see portable.go): the page keeps its settings next to the program too
		Windows: &windows.Options{
			WebviewUserDataPath:  webviewFolder(),
			WebviewIsTransparent: mica, WindowIsTranslucent: mica, BackdropType: backdrop,
		},
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
	// closed once the work stopped, its half written files removed
	finished chan struct{}
	// the files NXPress was started with
	files []string
	// the work waits while paused
	paused atomic.Bool
	// the turning off asked for once the files are done, until it is cancelled
	shutdown *time.Timer
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// the notices of the system, to tell when long work is done
	wailsruntime.InitializeNotifications(ctx)
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
	// the system and its processor, for reports
	Platform string `json:"platform"`
	// the files NXPress was started with, and the mode they suit
	Files []string `json:"files"`
	Mode  string   `json:"mode"`
	// whether "Send to" can hold NXPress, and holds it
	SendTo        bool `json:"sendTo"`
	SendToEnabled bool `json:"sendToEnabled"`
	// the window is clear behind the menu, for the Mica of Windows 11
	Mica bool `json:"mica"`
}

func (a *App) Setup() Setup {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	setup := Setup{
		Version: version, Keys: a.keys, Threads: defaultThreads(1), Cpus: runtime.NumCPU(),
		Level: defaultLevel, MaxLevel: maxLevel, Files: a.files, Platform: runtime.GOOS + "/" + runtime.GOARCH, SendTo: sendToFolder() != "",
		Mica: micaWindow(),
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

// Describe gives the main title of a file, with its icon, to show it in the list.
func (a *App) Describe(path string) (gameTitle, error) {
	if _, ok := splitContainer(path); ok {
		return gameTitle{}, errors.New("a file in parts")
	}
	return readTitle(path, true)
}

// FileCard is what Info shows of a file beside its row: its kind, its size and the one as NSP
// or XCI, how its NCZ files are compressed, and what trimming takes away.
type FileCard struct {
	Kind         string `json:"kind"`
	Size         int64  `json:"size"`
	Decompressed int64  `json:"decompressed"`
	Solid        int    `json:"solid"`
	Blocks       int    `json:"blocks"`
	Trimmable    int64  `json:"trimmable"`
}

// Card tells how a file is stored, for its card in Info.
func (a *App) Card(path string) (FileCard, error) {
	facts, err := switchfs.DescribeFile(path)
	if err != nil {
		return FileCard{}, err
	}
	return FileCard{Kind: facts.Kind, Size: facts.Size, Decompressed: facts.DecompressedSize, Solid: facts.Solid, Blocks: facts.Blocks, Trimmable: facts.Padding + facts.DeltaFragments}, nil
}

// NewNames gives the names the files would get by a template, "" for a file that cannot be read;
// with keepNamed, a file whose name has its title ID keeps it.
func (a *App) NewNames(paths []string, template string, keepNamed bool) []string {
	names := make([]string, len(paths))
	for i, path := range paths {
		if title, err := readTitle(path, false); err == nil {
			names[i] = newName(path, title, template)
			if keepNamed && keepsName(path, title.TitleID) {
				names[i] = filepath.Base(path)
			}
		}
	}
	return names
}

// PackGame is a game of the packs by game, as the window shows it before they are made.
type PackGame struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Files  []string `json:"files"`
	Packed string   `json:"packed"`
}

// PackPlan tells the packs by game the files would make: the files of each game, and the pack
// made before when there is one where its pack would go (output, or the folder of the game).
func (a *App) PackPlan(files []string, output string) []PackGame {
	sources, _ := inputs(files, []string{".nsp", ".nsz", ".xci", ".xcz"})
	order, groups, _ := gameGroups(sources)
	o := options{output: output}
	plan := []PackGame{}
	for _, id := range order {
		game := PackGame{ID: fmt.Sprintf("%016X", id), Files: groups[id]}
		if title, err := readTitle(groups[id][0], false); err == nil {
			game.Name = title.Name
		}
		if game.Name == "" {
			game.Name = filepath.Base(groups[id][0])
		}
		if packed := packedAlready(o, id, groups[id]); packed != "" {
			game.Packed = filepath.Base(packed)
		}
		plan = append(plan, game)
	}
	return plan
}

// DefaultPackTemplate is the name of the pack of each game when none is chosen.
func (a *App) DefaultPackTemplate() string {
	return defaultPackTemplate
}

// DefaultTemplate is the template of names offered first.
func (a *App) DefaultTemplate() string {
	return defaultTemplate
}

// ChooseDat asks for a DAT file.
func (a *App) ChooseDat() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Filters: []wailsruntime.FileFilter{{DisplayName: "DAT (*.dat, *.xml)", Pattern: "*.dat;*.xml"}},
	})
}

// ChooseSound asks for the sound played when the work is done.
func (a *App) ChooseSound() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Filters: []wailsruntime.FileFilter{{DisplayName: "Sound (*.wav, *.mp3, *.ogg, *.m4a)", Pattern: "*.wav;*.mp3;*.ogg;*.m4a"}},
	})
}

// maxSoundSize keeps the sound chosen short: the page gets it whole.
const maxSoundSize = 5 << 20

// soundTypes are the sound files the page plays, by their extension.
var soundTypes = map[string]string{".wav": "audio/wav", ".mp3": "audio/mpeg", ".ogg": "audio/ogg", ".m4a": "audio/mp4"}

// Sound gives a sound file as a data URL the page can play, since the page cannot open the files
// of the computer itself.
func (a *App) Sound(path string) (string, error) {
	kind := soundTypes[strings.ToLower(filepath.Ext(path))]
	if kind == "" {
		return "", errors.New("not a sound NXPress plays")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > maxSoundSize {
		return "", fmt.Errorf("the sound is bigger than %d MB", maxSoundSize>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(data), nil
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
	PackGames    bool     `json:"packGames"`
	PackTemplate string   `json:"packTemplate"`
	Dat          string   `json:"dat"`
	Template     string   `json:"template"`
	Watch        bool     `json:"watch"`
	NoCache      bool     `json:"noCache"`
	KeepNamed    bool     `json:"keepNamed"`
	SkipGames    []string `json:"skipGames"`
	VerifyLevel  string   `json:"verifyLevel"`
	Levels       bool     `json:"levels"`
	LowPriority  bool     `json:"lowPriority"`
}

// Start does the work in the background: the page follows it by the events "progress", "done"
// and "say", and "finished" once every file is done.
func (a *App) Start(request Request) error {
	o := options{
		level: request.Level, block: request.Block, threads: request.Threads, multi: max(1, request.Multi),
		output: request.Output, overwrite: request.Overwrite, rmSource: request.RemoveSource, files: request.Files,
	}
	if request.Mode == "verify" {
		o.verifyLevel = request.VerifyLevel
		o.dat = request.Dat
	}
	o.watch = request.Watch
	o.noCache = request.NoCache
	o.levels = request.Levels
	// below the normal priority when asked, normal again when not (Linux may refuse to raise
	// it back: NXPress then stays low until it is opened again)
	if err := setLowPriority(request.LowPriority); err != nil && request.LowPriority {
		emit(a.ctx, "say", "the priority could not be lowered: "+err.Error())
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
	case "measure":
		o.measure = true
	case "fat32":
		o.fat32 = true
	case "rename":
		o.template = request.Template
		o.keepNamed = request.KeepNamed
		if o.template == "" {
			o.template = defaultTemplate
		}
	case "split":
		o.split = true
	case "info":
		o.info = true
	case "pack":
		o.skipGames = map[string]bool{}
		for _, id := range request.SkipGames {
			o.skipGames[strings.ToUpper(id)] = true
		}
		// a SuperNSP per game, named by its template; else one file with everything
		if request.PackGames {
			o.packGames, o.packName = true, request.PackTemplate
			break
		}
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
	if a.keys == "" && !o.recompress && !o.fat32 {
		return errors.New("no prod.keys: choose one")
	}
	if a.cancel != nil {
		return errors.New("the files are still at work")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	finished := make(chan struct{})
	a.cancel, a.finished = cancel, finished
	go func() {
		started := time.Now()
		a.paused.Store(false)
		jobs := runJobs
		if o.watch {
			jobs = func(ctx context.Context, o options, out output) int { return watchJobs(ctx, o, out, watchEvery) }
		}
		failed := jobs(ctx, o, &window{ctx: a.ctx, job: ctx, app: a})
		a.mutex.Lock()
		a.cancel, a.finished = nil, nil
		a.mutex.Unlock()
		close(finished)
		emit(a.ctx, "finished", map[string]any{
			"failed": failed, "cancelled": ctx.Err() != nil, "seconds": int(time.Since(started).Seconds()),
		})
		cancel()
	}()
	return nil
}

// closeWait is how long NXPress waits as it closes for the files at work to stop, so their half
// written files are removed (big ones take a while on a slow disk).
var closeWait = 10 * time.Second

// beforeClose asks, while files are at work (or paused), whether to stop them and close; the
// window stays open (true) unless the answer is yes.
func (a *App) beforeClose(ctx context.Context) bool {
	if a.working() {
		answer, err := wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
			Type:    wailsruntime.QuestionDialog,
			Title:   "NXPress",
			Message: "Files are still at work. Stop them and close NXPress?",
			Buttons: []string{"Yes", "No"}, DefaultButton: "No", CancelButton: "No",
		})
		if err == nil && answer != "Yes" {
			return true
		}
	}
	keepWindowPlace(ctx)
	return false
}

// closed stops the files at work as NXPress closes, and waits for them to remove what they had
// half written: NXPress ends right after, and its work with it.
func (a *App) closed(context.Context) {
	a.mutex.Lock()
	finished := a.finished
	a.mutex.Unlock()
	a.Cancel()
	if finished == nil {
		return
	}
	select {
	case <-finished:
	case <-time.After(closeWait):
	}
}

// working tells whether files are at work, paused or not.
func (a *App) working() bool {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.cancel != nil
}

// Cancel stops the files at work; their half written files are removed.
func (a *App) Cancel() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.cancel != nil {
		a.paused.Store(false)
		a.cancel()
	}
}

// SetPaused stops the work where it is, or lets it go on.
func (a *App) SetPaused(paused bool) {
	a.paused.Store(paused)
}

// SetTaskbar shows the progress of the work on the button of NXPress in the taskbar (#31):
// state "none", "normal", "paused" or "error", done of total.
func (a *App) SetTaskbar(state string, done float64, total float64) {
	setTaskbarProgress(state, uint64(max(done, 0)), uint64(max(total, 0)))
}

// Notify shows a notice of the system, with the texts of the page.
func (a *App) Notify(title string, body string) {
	if wailsruntime.IsNotificationAvailable(a.ctx) {
		wailsruntime.SendNotification(a.ctx, wailsruntime.NotificationOptions{ID: "nxpress-done", Title: title, Body: body})
	}
}

// shutdownWait is how long the computer has to turn off once asked: NXPress still running after
// it means something kept it on, a program with work not saved most often.
const shutdownWait = 90 * time.Second

// ShutDownIn turns the computer off in the seconds given, unless CancelShutDown comes first;
// forced, the programs left open are closed without saving. When it does not turn off, the page
// is told ("shutdownFailed"), to say so.
func (a *App) ShutDownIn(seconds int, force bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.shutdown != nil {
		a.shutdown.Stop()
	}
	a.shutdown = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		if err := shutDown(force); err != nil {
			emit(a.ctx, "shutdownFailed", err.Error())
			return
		}
		time.AfterFunc(shutdownWait, func() { emit(a.ctx, "shutdownFailed", "") })
	})
}

// CancelShutDown keeps the computer on.
func (a *App) CancelShutDown() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.shutdown != nil {
		a.shutdown.Stop()
		a.shutdown = nil
	}
}

// ShowFile opens the folder of a file in the file manager.
func (a *App) ShowFile(path string) {
	showFile(path)
}

// emit sends an event to the page.
var emit = wailsruntime.EventsEmit

// window sends the progress and the results of the files to the page.
type window struct {
	ctx context.Context
	// the work, and the app that pauses it
	job context.Context
	app *App
}

// how often the progress of a file is sent at most, unless its percentage changes
const windowEvery = 500 * time.Millisecond

// waitWhilePaused holds the work while it is paused, until it goes on or is cancelled: before
// each file, and at every step that tells its progress.
func (w *window) waitWhilePaused() {
	for w.app != nil && w.app.paused.Load() && w.job.Err() == nil {
		time.Sleep(200 * time.Millisecond)
	}
}

func (w *window) progress(path string, step string) func(done int64, total int64) {
	var (
		mutex     sync.Mutex
		last      time.Time
		lastShare = int64(-1)
	)
	return func(done int64, total int64) {
		w.waitWhilePaused()
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
		emit(w.ctx, "progress", map[string]any{"file": path, "step": step, "done": done, "total": total})
	}
}

func (w *window) done(path string, output string, note string, err error) {
	result := map[string]any{"file": path, "output": output, "note": note}
	// the size of the new file, for the savings the page adds up
	if info, err := os.Stat(output); output != "" && err == nil {
		result["size"] = info.Size()
	}
	switch {
	case isSkipped(err):
		result["skipped"], result["note"] = true, err.Error()
	case err != nil:
		result["error"] = err.Error()
		if errors.Is(err, context.Canceled) {
			result["error"] = "cancelled"
		}
	}
	emit(w.ctx, "done", result)
}

func (w *window) say(format string, args ...any) {
	emit(w.ctx, "say", fmt.Sprintf(format, args...))
}

// measured gives the page what a measure found: the setting chosen, to put in its fields, and
// every setting tried, to show them.
func (w *window) measured(report measureReport) {
	trials := []map[string]any{}
	for _, t := range report.trials {
		trials = append(trials, map[string]any{"threads": t.Threads, "multi": t.Files, "speed": t.Speed, "best": t == report.best})
	}
	// a faster setting less than 5% faster than the one chosen: the window tells why it was not
	var faster any
	if report.faster != nil {
		faster = map[string]any{"threads": report.faster.Threads, "multi": report.faster.Files, "speed": report.faster.Speed}
	}
	emit(w.ctx, "measured", map[string]any{
		"threads": report.best.Threads, "multi": report.best.Files, "speed": report.best.Speed,
		"processors": report.processors, "level": report.level, "block": report.block,
		"seconds10GB": int(report.per10GB.Seconds()), "trials": trials, "faster": faster,
	})
}

// openURL opens a page in the browser of the system.
func openURL(ctx context.Context, url string) {
	wailsruntime.BrowserOpenURL(ctx, url)
}

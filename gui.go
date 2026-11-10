package main

import (
	"context"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DeLFuS77/NXPress/internal/jobs"
	"github.com/DeLFuS77/NXPress/internal/portable"
	"github.com/DeLFuS77/NXPress/internal/selfupdate"
	"github.com/DeLFuS77/NXPress/internal/system"
	"github.com/DeLFuS77/NXPress/internal/titledb"
	"github.com/DeLFuS77/NXPress/internal/windowplace"

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

// singleWindow is the lock that keeps NXPress to one window on Windows: the files the Explorer
// gives to another NXPress go to the one open (#87, #88).
const singleWindow = "nxpress-window-1"

// runWindow opens the window with the files given, and gives the exit code once it is closed.
func runWindow(start launch) int {
	app := &App{files: start.files, section: start.section, open: start.open}
	var single *wailsoptions.SingleInstanceLock
	if runtime.GOOS == "windows" {
		// the files go to the NXPress open, which comes to the front; one starting meanwhile
		// is waited for, until its window takes files (see startup)
		var another bool
		app.started, another = system.StartWindow(singleWindow)
		defer app.started()
		if another {
			system.AllowForeground()
		}
		single = &wailsoptions.SingleInstanceLock{UniqueId: singleWindow, OnSecondInstanceLaunch: app.secondInstance}
	}
	// the window opens where it was left and as big (see windowplace.go)
	place, _ := windowplace.LoadWindowPlace()
	// on Windows 11 the menu shows the Mica of the system through a window left clear
	mica := system.MicaWindow()
	background := &wailsoptions.RGBA{R: 24, G: 24, B: 27, A: 255}
	backdrop := windows.Auto
	if mica {
		background.A = 0
		backdrop = windows.Mica
	}
	err := wails.Run(&wailsoptions.App{
		Title:              "NXPress " + version,
		Width:              place.Width,
		Height:             place.Height,
		MinWidth:           windowplace.WindowMinWidth,
		MinHeight:          windowplace.WindowMinHeight,
		StartHidden:        true,
		BackgroundColour:   background,
		AssetServer:        &assetserver.Options{Assets: frontend},
		OnStartup:          app.startup,
		OnDomReady:         windowplace.PlaceWindow,
		OnBeforeClose:      app.beforeClose,
		OnShutdown:         app.closed,
		DragAndDrop:        &wailsoptions.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true},
		SingleInstanceLock: single,
		Bind:               []any{app},
		Linux:              &linux.Options{Icon: appIcon, ProgramName: "nxpress"},
		// portable (see portable.go): the page keeps its settings next to the program too
		Windows: &windows.Options{
			WebviewUserDataPath:  portable.WebviewFolder(),
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
	// the files NXPress was started with, the section the Explorer asked for (#88), and whether
	// they were opened with a double click (#87)
	files   []string
	section string
	open    bool
	// the files the Explorer gave to another NXPress meanwhile, until the page takes them, and
	// the context of the window, once it has one
	openedMutex sync.Mutex
	opened      []Opened
	openedCtx   context.Context
	// a call that brings this window to the front is going on
	fronting atomic.Bool
	// lets the next NXPress start, once this window takes files
	started func()
	// the new version CheckUpdate found, the cancel of its installation going on, and the
	// version installed in place of this one, to open (#91)
	updateMutex  sync.Mutex
	release      selfupdate.Release
	updateCancel context.CancelFunc
	installed    string
	// NXPress restarts into the version installed: no work starts any more (with mutex); and
	// the chores going on, which the restart and the close wait for (see chore)
	restarting bool
	chores     int
	// the work waits while paused
	paused atomic.Bool
	// the turning off asked for once the files are done, until it is cancelled, and which time
	// it was asked
	shutdown      *time.Timer
	shutdownRound int
	// below the normal priority, as the work asked for last
	lowPriority bool
	// while NXPress is low, stops keeping the processes of the window low and gives them back
	// their priority
	childrenBack func()
	// the notices of the system, readied as the first is sent, and whether they can be
	notices      sync.Once
	noticesReady bool
	// the question asked as the window closes with files at work, in the language of the page
	closeAsk, closeYes, closeNo string
	// the versions published of each game, once the page asked for them (see published.go)
	versions      atomic.Pointer[titledb.Versions]
	versionsMutex sync.Mutex
	// the log of the works, written one change at a time (see worklog.go)
	logMutex sync.Mutex
	// the images of the last search of the eShop, by address, so the one chosen is not downloaded
	// again (see covers.go)
	coversMutex sync.Mutex
	covers      map[string][]byte
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.openedMutex.Lock()
	a.openedCtx = ctx
	a.openedMutex.Unlock()
	if a.started != nil {
		a.started()
	}
	// the keys of the usual places are read at once, as the command line does
	if path, err := jobs.LoadKeys(""); err == nil {
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
	// the files NXPress was started with, the mode they open in, and whether they were opened
	// with a double click (#87)
	Files []string `json:"files"`
	Mode  string   `json:"mode"`
	Open  bool     `json:"open"`
	// whether "Send to" can hold NXPress, and holds it
	SendTo        bool `json:"sendTo"`
	SendToEnabled bool `json:"sendToEnabled"`
	// the window is clear behind the menu, for the Mica of Windows 11
	Mica bool `json:"mica"`
	// the computer can hibernate when the work is done (#62)
	Hibernate bool `json:"hibernate"`
	// the language chosen in the installer of Windows, for the page until one is chosen in it
	Language string `json:"language"`
}

func (a *App) Setup() Setup {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	setup := Setup{
		Version: version, Keys: a.keys, Threads: jobs.DefaultThreads(1), Cpus: runtime.NumCPU(),
		Level: jobs.DefaultLevel, MaxLevel: jobs.MaxLevel, Files: a.files, Platform: runtime.GOOS + "/" + runtime.GOARCH, SendTo: system.SendToFolder() != "",
		Mica: system.MicaWindow(), Hibernate: system.CanHibernate(), Language: system.InstallerLanguage(),
	}
	setup.SendToEnabled = setup.SendTo && system.SendToEnabled()
	if len(a.files) > 0 {
		setup.Mode = openedSection(launch{files: a.files, section: a.section, open: a.open})
		setup.Open = a.open
	}
	return setup
}

// Opened are files the Explorer gave to another NXPress while this one was open: dropped on it,
// sent to it, opened with a double click (#87) or from its menu (#88).
type Opened struct {
	Files   []string `json:"files"`
	Section string   `json:"section"`
	Open    bool     `json:"open"`
}

// openedSection is the section files open in: the one the menu asked for, Verify for a double
// click (#87), else Compress when they hold files to compress, else Decompress.
func openedSection(start launch) string {
	switch {
	case start.section != "":
		return start.section
	case start.open:
		return "verify"
	}
	if files, _ := jobs.Inputs(start.files, []string{".nsp", ".xci"}); len(files) > 0 {
		return "compress"
	}
	return "decompress"
}

// bringToFront puts this window in front of the others (a variable, for the tests).
var bringToFront = system.BringToFront

// secondInstance takes what another NXPress was started with: this window comes to the front,
// and the page is told of the files, which it takes with TakeOpened. It returns at once: Wails
// calls it from the one goroutine that takes, one by one, what the window hands it through a
// queue of one, and the window that comes to the front answers that goroutine, so the two would
// wait for each other for ever when many files came at once, the window minimized.
func (a *App) secondInstance(data wailsoptions.SecondInstanceData) {
	if a.fronting.CompareAndSwap(false, true) {
		go func() {
			defer a.fronting.Store(false)
			bringToFront()
		}()
	}
	start, ok := windowLaunch(absolute(data.Args, data.WorkingDirectory))
	if !ok || len(start.files) == 0 {
		return
	}
	a.openedMutex.Lock()
	a.opened = append(a.opened, Opened{Files: start.files, Section: openedSection(start), Open: start.open})
	ctx := a.openedCtx
	a.openedMutex.Unlock()
	if ctx != nil {
		wailsruntime.EventsEmit(ctx, "opened")
	}
}

// TakeOpened gives the files other NXPress were given since the last time, and forgets them.
func (a *App) TakeOpened() []Opened {
	a.openedMutex.Lock()
	defer a.openedMutex.Unlock()
	taken := a.opened
	a.opened = nil
	if taken == nil {
		return []Opened{}
	}
	return taken
}

// ExplorerPlaces are the places of NXPress in the Explorer of Windows, for the settings: its
// menu (#88), and whether it opens its files (#87), for this copy of NXPress or another one
// ("here", "moved", or "" when not there), with the types another program still opens.
type ExplorerPlaces struct {
	Menu   string   `json:"menu"`
	Files  string   `json:"files"`
	Others []string `json:"others"`
}

func (a *App) ExplorerPlaces() ExplorerPlaces {
	places := ExplorerPlaces{Others: []string{}}
	executable, err := system.Executable()
	if err != nil || !system.ExplorerPlaces() {
		return places
	}
	places.Menu = system.MenuPlace(executable)
	var others []string
	places.Files, others = system.FilesPlace(executable)
	for _, extension := range others {
		places.Others = append(places.Others, strings.ToUpper(strings.TrimPrefix(extension, ".")))
	}
	return places
}

// SetExplorerMenu puts the menu of NXPress in the Explorer, with the texts of the page, or takes
// it away (#88).
func (a *App) SetExplorerMenu(on bool, texts system.ExplorerMenu) (ExplorerPlaces, error) {
	executable, err := system.Executable()
	if err != nil {
		return a.ExplorerPlaces(), err
	}
	if on {
		err = system.AddMenu(executable, texts)
	} else {
		err = system.RemoveMenu()
	}
	return a.ExplorerPlaces(), err
}

// RelabelExplorerMenu writes the menu again in the language of the page, when it is there.
func (a *App) RelabelExplorerMenu(texts system.ExplorerMenu) {
	if executable, err := system.Executable(); err == nil {
		system.RelabelMenu(executable, texts)
	}
}

// SetExplorerFiles makes NXPress the program of its files, or takes it away (#87).
func (a *App) SetExplorerFiles(on bool) (ExplorerPlaces, error) {
	executable, err := system.Executable()
	if err != nil {
		return a.ExplorerPlaces(), err
	}
	if on {
		err = system.AddFiles(executable)
	} else {
		err = system.RemoveFiles()
	}
	return a.ExplorerPlaces(), err
}

// SetSendTo puts NXPress in the "Send to" menu of the Explorer, or takes it away.
func (a *App) SetSendTo(on bool) error {
	if on {
		return system.EnableSendTo()
	}
	return system.DisableSendTo()
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
	return a.useKeys(path)
}

// useKeys makes the keys of a file the active ones; when they cannot be, the ones before stay.
func (a *App) useKeys(path string) (string, error) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if _, err := jobs.LoadKeys(path); err != nil {
		if a.keys != "" {
			if _, err := jobs.LoadKeys(a.keys); err != nil {
				a.keys = ""
			}
		}
		return "", err
	}
	a.keys = path
	return path, nil
}

// Item is a file of the list.
type Item struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	// a dump folder, which Combine packs into a NUP
	Dump bool `json:"dump,omitempty"`
}

// extensions are the files a mode takes.
func extensions(mode string) []string {
	return jobs.Takes[mode]
}

// Expand gives the files a mode takes among the paths given, the files of the folders given
// included. A folder that cannot be read is told on the page: its games would else be left out
// without a word.
func (a *App) Expand(paths []string, mode string) []Item {
	files, missing := jobs.Inputs(paths, extensions(mode))
	for _, path := range missing {
		if a.ctx != nil {
			emit(a.ctx, "say", path+" cannot be read: its files are left out")
		}
	}
	// a NUP dropped on a work that does not take it is told, not left out without a word
	if !jobs.TakesNup(mode) && a.ctx != nil {
		for _, path := range jobs.NupFiles(paths) {
			emit(a.ctx, "say", filepath.Base(path)+": "+jobs.NupNotDone)
		}
	}
	items := make([]Item, 0, len(files))
	for _, path := range files {
		item := Item{Path: path, Name: filepath.Base(path), Dump: jobs.IsDumpFolder(path)}
		if info, err := os.Stat(path); err == nil && !item.Dump {
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
		// the folders a mode takes (parts, dumps) are chosen with Add folder
		if strings.HasPrefix(extension, ".") {
			patterns = append(patterns, "*"+extension)
		}
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

// Describe gives the main title of a file, with its icon, to show it in the list.
func (a *App) Describe(path string) (jobs.GameTitle, error) {
	if _, ok := jobs.SplitContainer(path); ok {
		return jobs.GameTitle{}, errors.New("a file in parts")
	}
	return jobs.ReadTitle(path, true)
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
	// the experimental formats of NXPress its NCZ files are in, which only NXPress reads (#58)
	Format string `json:"format,omitempty"`
	// the packages of a NUP: its game, its updates and its DLC, with their files, and whether
	// Eglantine and Lichii open it, as a note
	Packages    []jobs.NupPackage `json:"packages,omitempty"`
	Emulators   string            `json:"emulators,omitempty"`
	EmulatorsOK bool              `json:"emulatorsOk,omitempty"`
	ZeroBased   bool              `json:"zeroBased,omitempty"`
	// the icon a NUP carries, as an address of data for the card; noIcon when it carries none
	Icon   string `json:"icon,omitempty"`
	NoIcon bool   `json:"noIcon,omitempty"`
	// it carries the icons of its game itself, which are kept: no other is given it (#103)
	GameIcons bool `json:"gameIcons,omitempty"`
}

// Card tells how a file is stored, for its card in Info.
func (a *App) Card(path string) (FileCard, error) {
	if strings.EqualFold(filepath.Ext(path), ".nup") {
		details, err := jobs.NupCard(path)
		if err != nil {
			return FileCard{}, err
		}
		card := FileCard{Kind: "NUP", Size: details.Size, Decompressed: details.Uncompressed, Packages: details.Packages,
			Emulators: details.Emulators, EmulatorsOK: details.EmulatorsOK, ZeroBased: details.ZeroBased, NoIcon: details.Icon == nil,
			GameIcons: details.GameIcons}
		if details.Icon != nil {
			card.Icon = "data:" + http.DetectContentType(details.Icon) + ";base64," + base64.StdEncoding.EncodeToString(details.Icon)
		}
		return card, nil
	}
	facts, err := switchfs.DescribeFile(path)
	if err != nil {
		return FileCard{}, err
	}
	return FileCard{Kind: facts.Kind, Size: facts.Size, Decompressed: facts.DecompressedSize, Solid: facts.Solid, Blocks: facts.Blocks, Trimmable: facts.Padding + facts.DeltaFragments, Format: jobs.FormatOf(facts)}, nil
}

// FixNupMeta writes again the metadata of a NUP counted from 0, from the button of its card in
// Details, and tells what it did; not while a work is going on.
func (a *App) FixNupMeta(path string) (string, error) {
	if a.working() {
		return "", errors.New("wait for the work to end")
	}
	done, err := a.chore(true)
	if err != nil {
		return "", err
	}
	defer done()
	return fixNupMeta(path)
}

// fixNupMeta is jobs.FixNupMeta, which the tests stand in for
var fixNupMeta = jobs.FixNupMeta

// FixNupIcon gives a NUP without an icon the one of its game in titledb, from the button of its
// card, and tells what it did; with replace, a NUP given one already has it changed (#103). Not
// while a work is going on.
func (a *App) FixNupIcon(path string, replace bool) (string, error) {
	if a.working() {
		return "", errors.New("wait for the work to end")
	}
	done, err := a.chore(true)
	if err != nil {
		return "", err
	}
	defer done()
	return fixNupIcon(a.ctx, path, jobs.Options{ReplaceNupIcon: replace})
}

// ChooseNupIcon asks for an image (PNG, JPEG, WEBP or BMP), with a title that tells the size an
// icon is best made of, in the language of the page, and gives it to a NUP without an icon, cut
// square at its middle and made 256×256, from the button of its card; "" when none is chosen.
// With replace, a NUP given one already has it changed (#103).
func (a *App) ChooseNupIcon(path string, title string, replace bool) (string, error) {
	if a.working() {
		return "", errors.New("wait for the work to end")
	}
	image, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   title,
		Filters: []wailsruntime.FileFilter{{DisplayName: "Image (*.png, *.jpg, *.jpeg, *.webp, *.bmp)", Pattern: "*.png;*.jpg;*.jpeg;*.webp;*.bmp"}},
	})
	if err != nil || image == "" {
		return "", err
	}
	done, err := a.chore(true)
	if err != nil {
		return "", err
	}
	defer done()
	return fixNupIcon(a.ctx, path, jobs.Options{NupIcon: image, ReplaceNupIcon: replace})
}

// fixNupIcon is jobs.FixNupIcon, which the tests stand in for
var fixNupIcon = jobs.FixNupIcon

// NewNames gives the names the files would get by a template, "" for a file that cannot be read;
// with keepNamed, a file whose name has its title ID keeps it.
func (a *App) NewNames(paths []string, template string, keepNamed bool) []string {
	names := make([]string, len(paths))
	for i, path := range paths {
		if title, err := jobs.ReadTitle(path, false); err == nil {
			names[i] = jobs.NewName(path, title, template)
			if keepNamed && jobs.KeepsName(path, title.TitleID) {
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
// made before when there is one where its pack would go (output, or next to the files or to the
// folder given they came in), or among the files given under the name its pack would get, as the
// work then tells it. origins are the files and folders the list came from.
func (a *App) PackPlan(files []string, origins []string, output string, template string) []PackGame {
	sources, _ := jobs.Inputs(files, []string{".nsp", ".nsz", ".xci", ".xcz"})
	order, groups, _ := jobs.GameGroups(sources)
	o := jobs.Options{Output: output, Files: files, Origins: origins}
	folders := jobs.GivenFolders(o)
	if template == "" {
		template = jobs.DefaultPackTemplate
	}
	plan := []PackGame{}
	for _, id := range order {
		game := PackGame{ID: fmt.Sprintf("%016X", id), Files: groups[id]}
		if title, err := jobs.ReadTitle(groups[id][0], false); err == nil {
			game.Name = title.Name
		}
		if game.Name == "" {
			game.Name = filepath.Base(groups[id][0])
		}
		next := jobs.PackFolder(groups[id], sources, folders)
		if packed := jobs.PackedAlready(o, id, groups[id], next); packed != "" {
			game.Packed = filepath.Base(packed)
		} else if extension, err := jobs.PackKind(groups[id]); err == nil {
			if again := jobs.PackedAmong(o, groups[id], template, extension, next); again != "" {
				game.Packed = filepath.Base(again)
			}
		}
		plan = append(plan, game)
	}
	return plan
}

// DefaultPackTemplate is the name of the pack of each game when none is chosen.
func (a *App) DefaultPackTemplate() string {
	return jobs.DefaultPackTemplate
}

// DefaultTemplate is the template of names offered first.
func (a *App) DefaultTemplate() string {
	return jobs.DefaultTemplate
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
	// the dump folders too, once their NUP is checked: in Combine, with Delete originals (#79)
	RemoveDumps bool `json:"removeDumps"`
	// the files of a NUP over 256 MiB stored as they are, for Eglantine and Lichii
	NupRawBig    bool     `json:"nupRawBig"`
	Pack         string   `json:"pack"`
	PackGames    bool     `json:"packGames"`
	PackTemplate string   `json:"packTemplate"`
	Origins      []string `json:"origins"`
	Dat          string   `json:"dat"`
	Template     string   `json:"template"`
	Watch        bool     `json:"watch"`
	NoCache      bool     `json:"noCache"`
	KeepNamed    bool     `json:"keepNamed"`
	SkipGames    []string `json:"skipGames"`
	VerifyLevel  string   `json:"verifyLevel"`
	Levels       bool     `json:"levels"`
	LowPriority  bool     `json:"lowPriority"`
	Sparse       bool     `json:"sparse"`
	Archive      bool     `json:"archive"`
	// Details tells the newest versions published too, as the setting allows it
	Published bool `json:"published"`
}

// Start does the work in the background: the page follows it by the events "progress", "done"
// and "say", and "finished" once every file is done.
func (a *App) Start(request Request) error {
	o := jobs.Options{
		Level: request.Level, Block: request.Block, Threads: request.Threads, Multi: max(1, request.Multi),
		Output: request.Output, Overwrite: request.Overwrite, RmSource: request.RemoveSource, Files: request.Files,
	}
	if request.Mode == "verify" {
		o.VerifyLevel = request.VerifyLevel
		o.Dat = request.Dat
	}
	o.Watch = request.Watch
	o.NoCache = request.NoCache
	o.Levels = request.Levels
	o.Sparse = request.Sparse
	o.RmDumps = request.RemoveDumps && request.RemoveSource && request.Mode == "pack"
	o.NupZstdBig = !request.NupRawBig
	o.Archive = request.Archive
	switch request.Mode {
	case "compress":
		o.Compress = true
	case "decompress":
		o.Decompress = true
	case "convert":
		o.Convert = true
	case "recompress":
		o.Recompress = true
	case "trim":
		o.Trim = true
	case "estimate":
		o.Estimate = true
	case "measure":
		o.Measure = true
	case "fat32":
		o.Fat32 = true
	case "rename":
		o.Template = request.Template
		o.KeepNamed = request.KeepNamed
		if o.Template == "" {
			o.Template = jobs.DefaultTemplate
		}
	case "split":
		o.Split = true
	case "info":
		o.Info = true
		if request.Published {
			o.Versions = a.versions.Load()
		}
	case "pack":
		o.SkipGames = map[string]bool{}
		for _, id := range request.SkipGames {
			o.SkipGames[strings.ToUpper(id)] = true
		}
		// a SuperNSP per game, or one file with everything, named by its template next to the
		// files or in Save to; a file chosen by name is a work of a window before 1.3.0, resumed
		o.Origins = request.Origins
		switch {
		case request.PackGames:
			o.PackGames, o.PackName = true, request.PackTemplate
		case request.Pack != "":
			o.Pack = request.Pack
		default:
			o.PackOne, o.PackName = true, request.PackTemplate
		}
	default:
		o.Verify = true
	}
	if err := o.Check(); err != nil {
		return err
	}
	keys := jobs.NeedsKeys(o)
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.keys == "" && keys {
		return errors.New("no prod.keys: choose one")
	}
	if a.cancel != nil {
		return errors.New("the files are still at work")
	}
	if a.restarting {
		return errRestarting
	}
	ctx, cancel := context.WithCancel(a.ctx)
	finished := make(chan struct{})
	a.cancel, a.finished = cancel, finished
	// a turning off still to come was for the work before: it would cut this one off
	stopped := a.stopShutdown()
	priority := a.setPriority(request.LowPriority)
	go func() {
		if stopped {
			emit(a.ctx, "shutdownCancelled")
		}
		if priority != "" {
			emit(a.ctx, "say", priority)
		}
		started := time.Now()
		a.setPaused(false)
		work := jobs.RunJobs
		if o.Watch {
			work = func(ctx context.Context, o jobs.Options, out jobs.Output) int {
				return jobs.WatchJobs(ctx, o, out, jobs.WatchEvery)
			}
		}
		failed := work(ctx, o, &window{ctx: a.ctx, job: ctx, app: a})
		cancelled := ctx.Err() != nil
		a.mutex.Lock()
		a.cancel, a.finished = nil, nil
		// the taskbar ends as the work did: red while there are errors to look at, else nothing
		if failed > 0 && !cancelled {
			system.SetTaskbarProgress("error", 1, 1)
		} else {
			system.SetTaskbarProgress("none", 1, 1)
		}
		a.mutex.Unlock()
		close(finished)
		emit(a.ctx, "finished", map[string]any{
			"failed": failed, "cancelled": cancelled, "seconds": int(time.Since(started).Seconds()),
		})
		cancel()
	}()
	return nil
}

// changePriority puts NXPress below the normal priority, or back to it.
var changePriority = system.SetLowPriority

// setPriority puts NXPress below the normal priority when asked, and back to it when not, as the
// work asked for is taken, and only when that changes; it gives a note when it could not.
// Linux and macOS may refuse to raise it back: NXPress then stays low until it is opened again.
// The mutex is held.
func (a *App) setPriority(low bool) string {
	if low == a.lowPriority {
		return ""
	}
	a.lowPriority = low
	err := changePriority(low)
	a.keepChildrenLow(low)
	switch {
	case err == nil:
		return ""
	case low:
		return "the priority could not be lowered: " + err.Error()
	}
	return "the priority could not go back to normal: " + err.Error()
}

// changeChildrenPriority puts the processes of the window below the normal priority with NXPress,
// or back; childrenEvery is how often it is done again while NXPress is low.
var (
	changeChildrenPriority = system.SetChildrenLowPriority
	childrenEvery          = 10 * time.Second
)

// keepChildrenLow puts the processes of the window, WebView2, below the normal priority with
// NXPress and keeps them there while it is low, as WebView2 raises them again when the window
// shows (#70); or gives them back theirs. The mutex is held.
func (a *App) keepChildrenLow(low bool) {
	if !low {
		if a.childrenBack != nil {
			a.childrenBack()
			a.childrenBack = nil
		}
		return
	}
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		every := time.NewTicker(childrenEvery)
		defer every.Stop()
		for {
			changeChildrenPriority(true)
			select {
			case <-stop:
				changeChildrenPriority(false)
				return
			case <-every.C:
			}
		}
	}()
	a.childrenBack = func() {
		close(stop)
		<-stopped
	}
}

// closeWait is how long NXPress waits as it closes for the files at work to stop, so their half
// written files are removed (big ones take a while on a slow disk).
var closeWait = 10 * time.Second

// beforeClose asks, while files are at work (or paused), whether to stop them and close; the
// window stays open (true) unless the answer is yes.
func (a *App) beforeClose(ctx context.Context) bool {
	if a.working() {
		question, yes, no := a.closeQuestion()
		// Windows names the buttons in its own language and answers "Yes"; the others give back
		// the button chosen
		answer, err := wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
			Type:    wailsruntime.QuestionDialog,
			Title:   "NXPress",
			Message: question,
			Buttons: []string{yes, no}, DefaultButton: no, CancelButton: no,
		})
		if err == nil && answer != yes && answer != "Yes" {
			return true
		}
	}
	windowplace.KeepWindowPlace(ctx)
	return false
}

// CloseTexts are the words of the question asked as the window closes with files at work, in the
// language of the page: it sends them each time its language is set.
func (a *App) CloseTexts(question string, yes string, no string) {
	if question == "" || yes == "" || no == "" {
		return
	}
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.closeAsk, a.closeYes, a.closeNo = question, yes, no
}

// closeQuestion is the question asked as the window closes, in English until the page told its own.
func (a *App) closeQuestion() (question string, yes string, no string) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.closeAsk == "" {
		return "Files are still at work. Stop them and close NXPress?", "Yes", "No"
	}
	return a.closeAsk, a.closeYes, a.closeNo
}

// closed stops the files at work as NXPress closes, and waits for them to remove what they had
// half written: NXPress ends right after, and its work with it.
func (a *App) closed(context.Context) {
	a.mutex.Lock()
	finished := a.finished
	a.mutex.Unlock()
	a.Cancel()
	if finished != nil {
		select {
		case <-finished:
		case <-time.After(closeWait):
		}
	}
	// a chore going on ends whole: the log of the work that just ended, the metadata of a NUP
	a.choresOver(closeWait)
}

// working tells whether files are at work, paused or not.
func (a *App) working() bool {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.cancel != nil
}

// errRestarting answers what is asked while NXPress restarts into its new version (#91).
var errRestarting = errors.New("restarting: NXPress is restarting into its new version")

// chore marks a short work done outside Start that writes a file (the metadata of a NUP, an
// export, the log of the works, the versions published): the restart into a new version and the
// close wait for it to end. One the user asks for (refuse) is not done while NXPress restarts;
// the log of a work that just ended is still written. The function given ends it.
func (a *App) chore(refuse bool) (func(), error) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if refuse && a.restarting {
		return nil, errRestarting
	}
	a.chores++
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mutex.Lock()
			a.chores--
			a.mutex.Unlock()
		})
	}, nil
}

// choresOver waits for the chores going on to end, for wait at most, and tells whether they did.
func (a *App) choresOver(wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for {
		a.mutex.Lock()
		left := a.chores
		a.mutex.Unlock()
		if left == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(choreTick)
	}
}

// how often choresOver looks whether the chores ended
const choreTick = 20 * time.Millisecond

// Cancel stops the files at work; their half written files are removed.
func (a *App) Cancel() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.cancel != nil {
		a.setPaused(false)
		a.cancel()
	}
}

// SetPaused stops the work where it is, or lets it go on.
func (a *App) SetPaused(paused bool) {
	a.setPaused(paused)
}

// setPaused pauses the work or lets it go on; paused, the computer may go to sleep.
func (a *App) setPaused(paused bool) {
	a.paused.Store(paused)
	system.PauseAwake(paused)
}

// SetTaskbar shows the progress of the work on the button of NXPress in the taskbar (#31):
// state "none", "normal", "paused" or "error", done of total. Progress told once the work ended
// (it comes late, or for work that ended at once) is left out: the state the work ended with
// stays.
func (a *App) SetTaskbar(state string, done float64, total float64) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if (state == "normal" || state == "paused") && a.cancel == nil {
		return
	}
	system.SetTaskbarProgress(state, uint64(max(done, 0)), uint64(max(total, 0)))
}

// Notify shows a notice of the system, with the texts of the page, to tell when long work is
// done.
func (a *App) Notify(title string, body string) {
	a.notices.Do(func() { a.noticesReady = a.readyNotices() })
	if a.noticesReady {
		wailsruntime.SendNotification(a.ctx, wailsruntime.NotificationOptions{ID: "nxpress-done", Title: title, Body: body})
	}
}

// readyNotices readies the notices of the system as the first one is sent, not at every start,
// and tells whether they can be sent. On Windows that writes NXPress in the registry (and its
// icon in the temporary folder), which a portable NXPress does not do: it sends none. macOS asks
// the user first.
func (a *App) readyNotices() bool {
	if runtime.GOOS == "windows" && portable.Folder() != "" {
		return false
	}
	wailsruntime.InitializeNotifications(a.ctx)
	if !wailsruntime.IsNotificationAvailable(a.ctx) {
		return false
	}
	if runtime.GOOS == "darwin" {
		if allowed, err := wailsruntime.CheckNotificationAuthorization(a.ctx); err != nil || !allowed {
			allowed, err = wailsruntime.RequestNotificationAuthorization(a.ctx)
			return err == nil && allowed
		}
	}
	return true
}

// shutdownWait is how long the computer has to turn off once asked: NXPress still running after
// it means something kept it on, a program with work not saved most often.
const shutdownWait = 90 * time.Second

// ShutDownIn turns the computer off in the seconds given, unless CancelShutDown or new work
// comes first; forced, the programs left open are closed without saving. When it does not turn
// off, the page is told ("shutdownFailed"), to say so.
func (a *App) ShutDownIn(seconds int, force bool) {
	a.powerOffIn(seconds, func() error { return system.ShutDown(force) }, true)
}

// HibernateIn hibernates the computer in the seconds given, as ShutDownIn turns it off: what is
// open waits on the disk, so no program is closed and there is nothing to force (#62).
func (a *App) HibernateIn(seconds int) {
	a.powerOffIn(seconds, hibernate, false)
}

// hibernate is system.Hibernate, which the tests stand in for
var hibernate = system.Hibernate

// powerOffIn does what turns the computer off or hibernates it in the seconds given, unless
// CancelShutDown or new work comes first. Turning off is watched: NXPress still running
// shutdownWait later means it did not happen. A computer that hibernated comes back to NXPress as
// it was, so the page is only told the count is over.
func (a *App) powerOffIn(seconds int, do func() error, watch bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.stopShutdown()
	a.shutdownRound++
	round := a.shutdownRound
	a.shutdown = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		if !a.shutdownDue(round) {
			return
		}
		if err := do(); err != nil {
			emit(a.ctx, "shutdownFailed", err.Error())
			return
		}
		if watch {
			time.AfterFunc(shutdownWait, func() { emit(a.ctx, "shutdownFailed", "") })
			return
		}
		emit(a.ctx, "shutdownCancelled")
	})
}

// shutdownDue tells, as its time comes, whether the turning off asked for the time given is still
// to be done: not once cancelled or asked again, nor with files at work, which it would cut off
// (the page is told it is cancelled then).
func (a *App) shutdownDue(round int) bool {
	a.mutex.Lock()
	asked := a.shutdown != nil && a.shutdownRound == round
	working := a.cancel != nil
	if asked {
		a.shutdown = nil
	}
	a.mutex.Unlock()
	if asked && working {
		emit(a.ctx, "shutdownCancelled")
	}
	return asked && !working
}

// stopShutdown stops the turning off still to come, and tells whether there was one; the mutex
// is held.
func (a *App) stopShutdown() bool {
	if a.shutdown == nil {
		return false
	}
	a.shutdown.Stop()
	a.shutdown = nil
	return true
}

// CancelShutDown keeps the computer on.
func (a *App) CancelShutDown() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.stopShutdown()
}

// ShowFile opens the folder of a file in the file manager.
func (a *App) ShowFile(path string) {
	system.ShowFile(path)
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

// WaitWhilePaused holds the work while it is paused, until it goes on or is cancelled: before
// each file, and at every step that tells its progress.
func (w *window) WaitWhilePaused() {
	for w.app != nil && w.app.paused.Load() && w.job.Err() == nil {
		time.Sleep(200 * time.Millisecond)
	}
}

func (w *window) Progress(path string, step string) func(done int64, total int64) {
	var (
		mutex     sync.Mutex
		last      time.Time
		lastShare = int64(-1)
	)
	return func(done int64, total int64) {
		w.WaitWhilePaused()
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

func (w *window) Done(path string, output string, note string, err error) {
	result := map[string]any{"file": path, "output": output, "note": note}
	// the size of the new file, for the savings the page adds up
	if info, err := os.Stat(output); output != "" && err == nil {
		result["size"] = info.Size()
	}
	switch {
	case jobs.IsSkipped(err):
		result["skipped"], result["note"] = true, err.Error()
	case jobs.IsModified(err):
		result["modified"], result["note"] = true, err.Error()
	case err != nil:
		result["error"] = err.Error()
		if errors.Is(err, context.Canceled) {
			result["error"] = "cancelled"
		}
	}
	emit(w.ctx, "done", result)
}

func (w *window) Say(format string, args ...any) {
	emit(w.ctx, "say", fmt.Sprintf(format, args...))
}

// measured gives the page what a measure found: the setting chosen, to put in its fields, and
// every setting tried, to show them.
func (w *window) measured(report jobs.MeasureReport) {
	trials := []map[string]any{}
	for _, t := range report.Trials {
		trials = append(trials, map[string]any{"threads": t.Threads, "multi": t.Files, "speed": t.Speed, "best": t == report.Best})
	}
	// a faster setting less than 5% faster than the one chosen: the window tells why it was not
	var faster any
	if report.Faster != nil {
		faster = map[string]any{"threads": report.Faster.Threads, "multi": report.Faster.Files, "speed": report.Faster.Speed}
	}
	emit(w.ctx, "measured", map[string]any{
		"threads": report.Best.Threads, "multi": report.Best.Files, "speed": report.Best.Speed,
		"processors": report.Processors, "level": report.Level, "block": report.Block,
		"seconds10GB": int(report.Per10GB.Seconds()), "trials": trials, "faster": faster,
	})
}

// openURL opens a page in the browser of the system.
func openURL(ctx context.Context, url string) {
	wailsruntime.BrowserOpenURL(ctx, url)
}

package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DeLFuS77/NXPress/internal/edensaves"
	"github.com/DeLFuS77/NXPress/internal/portable"
	"github.com/dtrunk90/switch-library-manager-web/edensync"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The saves of Eden with Switch Library Manager Web (see internal/edensaves), in Settings: the
// passes run from the start of the window until it closes, never while NXPress is closed. The
// page is told the status as it changes ("edenSaves") and what a pass brought or found
// ("edenNotice"), which it tells with a notice of the system.

// how long the page waits for the library to answer a pairing, and for the network to be looked at
const (
	edenAskWait  = 15 * time.Second
	edenFindWait = 45 * time.Second
)

// the answers of the page, which it tells in its language: "" for done
const (
	edenNoFolder   = "noSettings"
	edenNotLibrary = "notLibrary"
	edenNotEden    = "notEden"
	edenKeepOld    = "keepOld"
	edenGone       = "gone"
)

// edenService is the service of the saves of Eden, made once; nil without a folder for the
// settings.
func (a *App) edenService() *edensaves.Service {
	a.edenOnce.Do(func() {
		dir, err := portable.DataFolder()
		if err != nil {
			return
		}
		a.eden = &edensaves.Service{
			Dir:     dir,
			Changed: func(status edensaves.Status) { a.edenSend("edenSaves", status) },
			Noticed: func(notice edensaves.Notice) { a.edenSend("edenNotice", notice) },
			// a pass that writes in the folder of the emulator is waited for as NXPress closes
			// or restarts into a new version, and none starts meanwhile
			Busy: func() (func(), error) { return a.chore(true) },
		}
	})
	return a.eden
}

func (a *App) edenSend(name string, data any) {
	if a.ctx != nil {
		emit(a.ctx, name, data)
	}
}

// startEden runs the passes while the window is open.
func (a *App) startEden(ctx context.Context) {
	service := a.edenService()
	if service == nil {
		return
	}
	run, stop := context.WithCancel(ctx)
	a.mutex.Lock()
	a.edenStop = stop
	a.mutex.Unlock()
	go service.Run(run)
}

// stopEden stops the passes as NXPress closes: a pass going on ends what it writes (see Busy),
// and the lock of the folder of the emulator is given up, so Switch Library Manager Web may keep
// the saves in step.
func (a *App) stopEden() {
	a.mutex.Lock()
	stop := a.edenStop
	a.edenStop = nil
	a.mutex.Unlock()
	if stop != nil {
		stop()
		a.eden.Release()
	}
}

// edenAnswer is an error as the page tells it: a kind it knows, else "other: " and the error.
func edenAnswer(err error) string {
	var status *edensync.StatusError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, edensaves.ErrNotLibrary):
		return edenNotLibrary
	case errors.Is(err, edensync.ErrNotFound):
		return edenNotEden
	case errors.Is(err, edensaves.ErrKeepUnknown):
		return edenKeepOld
	case errors.Is(err, edensaves.ErrConflictGone):
		return edenGone
	case errors.As(err, &status) && status.Status == 401:
		return edensaves.ErrorKey
	case errors.As(err, &status) && edensaves.IsNetOff(status):
		return edensaves.ErrorNetOff
	}
	return "other: " + strings.TrimSpace(err.Error())
}

// EdenSaves is the status of the saves of Eden, for Settings.
func (a *App) EdenSaves() edensaves.Status {
	service := a.edenService()
	if service == nil {
		return edensaves.Status{Profiles: []edensaves.Profile{}, Accounts: []string{}, Games: []edensaves.Game{}, Conflicts: []edensaves.Conflict{}}
	}
	return service.Status()
}

// EdenRefresh looks at the saves again, as Settings is opened: the status comes as "edenSaves".
func (a *App) EdenRefresh() {
	if service := a.edenService(); service != nil {
		service.Wake(false)
	}
}

// EdenFind looks for the libraries on the local network, as NXDock does.
func (a *App) EdenFind() []edensaves.Found {
	ctx, cancel := context.WithTimeout(a.edenContext(), edenFindWait)
	defer cancel()
	return edensaves.Discover(ctx)
}

// edenContext is the context of the window, or one of its own before it has one.
func (a *App) edenContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// EdenPair asks the library at an address (found, or typed) to pair this computer: the code to
// show comes with the status.
func (a *App) EdenPair(address string) string {
	service := a.edenService()
	if service == nil {
		return edenNoFolder
	}
	ctx, cancel := context.WithTimeout(a.edenContext(), edenAskWait)
	defer cancel()
	_, err := service.Pair(ctx, address)
	return edenAnswer(err)
}

// EdenCancelPairing stops waiting for the pairing asked.
func (a *App) EdenCancelPairing() {
	if service := a.edenService(); service != nil {
		service.CancelPairing()
	}
}

// EdenForget forgets the library paired and its key.
func (a *App) EdenForget() string {
	service := a.edenService()
	if service == nil {
		return edenNoFolder
	}
	return edenAnswer(service.Forget())
}

// EdenSetOn turns the passes on or off.
func (a *App) EdenSetOn(on bool) string {
	service := a.edenService()
	if service == nil {
		return edenNoFolder
	}
	return edenAnswer(service.SetOn(on))
}

// EdenChooseFolder takes the data folder of the emulator chosen in a dialog; "" when none was
// chosen or it was taken.
func (a *App) EdenChooseFolder(title string) string {
	service := a.edenService()
	if service == nil {
		return edenNoFolder
	}
	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: title})
	if err != nil || path == "" {
		return ""
	}
	return edenAnswer(service.UseFolder(path))
}

// EdenFindFolder goes back to the data folder of the emulator found by itself.
func (a *App) EdenFindFolder() string {
	service := a.edenService()
	if service == nil {
		return edenNoFolder
	}
	return edenAnswer(service.UseFolder(""))
}

// EdenSetProfile pairs a profile of the emulator with an account of the console ("" for none).
func (a *App) EdenSetProfile(profile string, account string) string {
	service := a.edenService()
	if service == nil {
		return edenNoFolder
	}
	ctx, cancel := context.WithTimeout(a.edenContext(), edenAskWait)
	defer cancel()
	return edenAnswer(service.SetProfile(ctx, profile, account))
}

// EdenKeep tells which save the emulator keeps of a conflict: "console" or "emulator".
func (a *App) EdenKeep(id string, keep string) string {
	service := a.edenService()
	if service == nil {
		return edenNoFolder
	}
	ctx, cancel := context.WithTimeout(a.edenContext(), edenAskWait)
	defer cancel()
	return edenAnswer(service.Keep(ctx, id, keep))
}

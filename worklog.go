package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/DeLFuS77/NXPress/internal/portable"
	"github.com/DeLFuS77/NXPress/internal/system"
	"github.com/DeLFuS77/NXPress/internal/worklog"
)

// The log of the works (#69): the page gives each work as it ends, and shows the list kept, in
// the folder of the settings with a text file of it (see internal/worklog).

// Log gives the works kept, the oldest first.
func (a *App) Log() []worklog.Work {
	a.logMutex.Lock()
	defer a.logMutex.Unlock()
	folder, err := portable.DataFolder()
	if err != nil {
		return []worklog.Work{}
	}
	return worklog.Read(folder)
}

// AddToLog keeps a work that ended, with keep works at most (no limit when 0), and gives the
// works kept.
func (a *App) AddToLog(work worklog.Work, keep int) ([]worklog.Work, error) {
	a.logMutex.Lock()
	defer a.logMutex.Unlock()
	folder, err := portable.DataFolder()
	if err != nil {
		return nil, err
	}
	return worklog.Add(folder, work, keep)
}

// KeepLog leaves out the oldest works past keep, as its setting goes down, and gives the works
// kept.
func (a *App) KeepLog(keep int) ([]worklog.Work, error) {
	a.logMutex.Lock()
	defer a.logMutex.Unlock()
	folder, err := portable.DataFolder()
	if err != nil {
		return nil, err
	}
	works := worklog.Read(folder)
	kept := worklog.Trim(works, keep)
	if len(kept) == len(works) {
		return works, nil
	}
	return kept, worklog.Write(folder, kept)
}

// ClearLog removes the log, both its files.
func (a *App) ClearLog() error {
	a.logMutex.Lock()
	defer a.logMutex.Unlock()
	folder, err := portable.DataFolder()
	if err != nil {
		return err
	}
	return worklog.Clear(folder)
}

// WriteLog puts back the works given, as Undo does once the log was cleared.
func (a *App) WriteLog(works []worklog.Work) error {
	a.logMutex.Lock()
	defer a.logMutex.Unlock()
	folder, err := portable.DataFolder()
	if err != nil {
		return err
	}
	return worklog.Write(folder, works)
}

// LogFile gives where the text file of the log is, whether or not there is one yet.
func (a *App) LogFile() string {
	folder, err := portable.DataFolder()
	if err != nil {
		return ""
	}
	return filepath.Join(folder, worklog.TextName)
}

// OpenLog opens the text file of the log in the editor of the system.
func (a *App) OpenLog() error {
	path := a.LogFile()
	if _, err := os.Stat(path); err != nil {
		return errors.New("the log is empty")
	}
	return system.OpenFile(path)
}

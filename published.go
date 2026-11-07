package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/DeLFuS77/NXPress/internal/jobs"
	"github.com/DeLFuS77/NXPress/internal/portable"
	"github.com/DeLFuS77/NXPress/internal/titledb"
)

// how long the versions published have to come, from every server tried
const versionsWait = 2 * time.Minute

// LoadVersions reads the versions published of each game and DLC, downloading them again when the
// copy kept is a day old (see titledb.Refresh): only from the window, when its settings allow it.
// The names of the games of the Switch 2 come with them, for the dump folders without control
// data Combine packs (see titledb.RefreshNames).
// It gives the day of the list (2026-11-02), "" when there is none; with no connection it is
// the copy kept, and nothing is said.
func (a *App) LoadVersions() string {
	a.versionsMutex.Lock()
	defer a.versionsMutex.Unlock()
	folder, err := portable.DataFolder()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionsWait)
	defer cancel()
	versions, _ := titledb.Refresh(ctx, folder, http.DefaultClient, time.Now())
	a.versions.Store(versions)
	names, _ := titledb.RefreshNames(ctx, folder, http.DefaultClient, time.Now())
	jobs.UseNames(names)
	if versions == nil {
		return ""
	}
	return versions.Date.Format(time.DateOnly)
}

// ForgetVersions leaves the versions published, and removes the copy kept, as their setting is
// turned off: the command line then tells them no more either.
func (a *App) ForgetVersions() {
	a.versionsMutex.Lock()
	defer a.versionsMutex.Unlock()
	a.versions.Store(nil)
	jobs.UseNames(nil)
	if folder, err := portable.DataFolder(); err == nil {
		_ = titledb.Forget(folder)
		_ = titledb.ForgetNames(folder)
	}
}

// Published gives the newest version published of each title ID asked (16 hexadecimal digits),
// of the ones the list knows.
func (a *App) Published(ids []string) map[string]titledb.Published {
	versions := a.versions.Load()
	found := map[string]titledb.Published{}
	for _, id := range ids {
		number, err := strconv.ParseUint(id, 16, 64)
		if err != nil || len(id) != 16 {
			continue
		}
		if published, ok := versions.Newest(number); ok {
			found[id] = published
		}
	}
	return found
}

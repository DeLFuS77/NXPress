package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DeLFuS77/NXPress/internal/selfupdate"
	"github.com/DeLFuS77/NXPress/internal/system"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// the latest release of NXPress, as GitHub tells it
var latestRelease = "https://api.github.com/repos/DeLFuS77/NXPress/releases/latest"

// Update is a newer version of NXPress, when there is one, and whether NXPress can install it by
// itself (#91): it is signed, it has the program of this system, and NXPress can write in its
// folder; else why not ("unsigned", "unsupported", "write").
type Update struct {
	Version     string `json:"version"`
	URL         string `json:"url"`
	Installable bool   `json:"installable"`
	Reason      string `json:"reason,omitempty"`
	// what is new in it, the notes of the release without how to download it
	Notes string `json:"notes"`
	// GitHub answered: with no version, this one is the newest (Check now, #106)
	Checked bool `json:"checked,omitempty"`
}

// CheckUpdate asks GitHub for the latest version of NXPress: one of the two times NXPress goes on
// the internet, with the versions published of the games (see published.go), and only from the
// window, when its settings allow it. Builds that are not a version do not ask.
func (a *App) CheckUpdate() Update {
	if _, ok := versionNumbers(version); !ok {
		return Update{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, latestRelease, nil)
	if err != nil {
		return Update{}
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "NXPress/"+version)
	// the answer of a moment ago is not wanted: a version just out is told at once (#106)
	request.Header.Set("Cache-Control", "no-cache")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Update{}
	}
	defer response.Body.Close()
	var release struct {
		Tag        string `json:"tag_name"`
		URL        string `json:"html_url"`
		Body       string `json:"body"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&release) != nil {
		return Update{}
	}
	if !newer(release.Tag, version) || !strings.HasPrefix(release.URL, "https://github.com/") {
		return Update{Checked: true}
	}
	found := selfupdate.Release{Tag: release.Tag, Prerelease: release.Prerelease, Assets: map[string]string{}}
	for _, asset := range release.Assets {
		found.Assets[asset.Name] = asset.URL
	}
	a.updateMutex.Lock()
	a.release = found
	a.updateMutex.Unlock()
	update := Update{Version: release.Tag, URL: release.URL, Notes: releaseNotes(release.Body), Checked: true}
	update.Reason = installReason(found)
	update.Installable = update.Reason == ""
	return update
}

// installReason tells why NXPress cannot install a release by itself, "" when it can: a system
// it does not replace itself on, a release without signature, or a folder it cannot write in.
func installReason(release selfupdate.Release) string {
	switch {
	case selfupdate.AssetName(runtime.GOOS, runtime.GOARCH, release.Tag) == "" || len(selfupdate.Keys()) == 0:
		return "unsupported"
	case !selfupdate.Installable(release, selfupdate.Keys(), runtime.GOOS, runtime.GOARCH):
		return "unsigned"
	case !writableBeside():
		return "write"
	}
	return ""
}

// writableBeside tells whether NXPress can write in the folder of its program: not in Program
// Files, for one.
func writableBeside() bool {
	program, err := system.Executable()
	if err != nil {
		return false
	}
	probe, err := os.CreateTemp(filepath.Dir(program), ".nxpress-update-probe-*")
	if err != nil {
		return false
	}
	probe.Close()
	os.Remove(probe.Name())
	return true
}

// releaseNotes are the notes of a release as the window shows them: the section of the
// CHANGELOG, without the table of the downloads the release adds under it.
func releaseNotes(body string) string {
	if cut := strings.Index(body, "### Downloads"); cut >= 0 {
		body = body[:cut]
	}
	return strings.TrimSpace(body)
}

// InstallUpdate installs the version CheckUpdate found in place of this NXPress (#91): it opens
// the next time NXPress starts, or now with RestartUpdated. A work going on goes on, as the
// program running is not touched. Its error starts with its reason, as selfupdate.Code names it;
// the progress goes to the page as "update-progress".
func (a *App) InstallUpdate() error {
	a.updateMutex.Lock()
	release := a.release
	if a.updateCancel != nil || release.Tag == "" {
		a.updateMutex.Unlock()
		return errors.New("failed: no update to install")
	}
	// installed already: it only waits to be opened
	if a.installed == release.Tag {
		a.updateMutex.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.updateCancel = cancel
	a.updateMutex.Unlock()
	defer func() {
		a.updateMutex.Lock()
		a.updateCancel = nil
		a.updateMutex.Unlock()
		cancel()
	}()
	program, err := system.Executable()
	if err != nil {
		return errors.New("failed: " + err.Error())
	}
	installer := selfupdate.Installer{
		Program: program, Version: version, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Keys: selfupdate.Keys(),
		Progress: func(step string, done, total int64) {
			wailsruntime.EventsEmit(a.ctx, "update-progress", map[string]any{"step": step, "done": done, "total": total})
		},
	}
	if err := installer.Install(ctx, release); err != nil {
		return errors.New(selfupdate.Code(err) + ": " + err.Error())
	}
	a.updateMutex.Lock()
	a.installed = release.Tag
	a.updateMutex.Unlock()
	return nil
}

// RestartUpdated opens the version installed, with the files of the list in their section, and
// closes this NXPress. Every work must have ended first: not during a work of the files (a watch
// of folders included) nor with a turning off of the computer still to come ("work": the page
// waits for its end); from here on no work starts, and the chores going on (the log of the work
// that just ended, the metadata of a NUP being written) end before it restarts.
func (a *App) RestartUpdated(mode string, files []string) error {
	a.updateMutex.Lock()
	installed := a.installed
	a.updateMutex.Unlock()
	if installed == "" {
		return errors.New("failed: no new version installed")
	}
	program, err := system.Executable()
	if err != nil {
		return errors.New("restart: " + err.Error())
	}
	a.mutex.Lock()
	if a.cancel != nil || a.shutdown != nil {
		a.mutex.Unlock()
		return errors.New("work: a work is going on")
	}
	a.restarting = true
	a.mutex.Unlock()
	if !a.choresOver(choresWait) {
		a.endRestart()
		return errors.New("work: a work is going on")
	}
	// the new one waits for this one to stop, then opens with the list
	args := []string{"--after-update", strconv.Itoa(os.Getpid())}
	if len(files) > 0 && slices.Contains(sections, mode) {
		args = append(append(args, "--section", mode), files...)
	}
	if err := startDetached(program, args...); err != nil {
		a.endRestart()
		return errors.New("restart: " + err.Error())
	}
	quitWindow(a.ctx)
	return nil
}

// endRestart lets the works start again: the restart did not happen.
func (a *App) endRestart() {
	a.mutex.Lock()
	a.restarting = false
	a.mutex.Unlock()
}

// how long the restart waits for the chores going on: the longest is the download of the
// versions published, which gives up after versionsWait
const choresWait = versionsWait + 30*time.Second

// how the new version is started and this window closed; variables for the tests
var (
	startDetached = system.StartDetached
	quitWindow    = wailsruntime.Quit
)

// CancelUpdate stops the download of a new version: nothing is replaced.
func (a *App) CancelUpdate() {
	a.updateMutex.Lock()
	defer a.updateMutex.Unlock()
	if a.updateCancel != nil {
		a.updateCancel()
	}
}

// discordInvite is the invite to the Discord server NXPress shares with Switch Library Manager
// Web (#86): it does not expire.
const discordInvite = "https://discord.gg/acQVZSEBR"

// OpenURL opens a page in the browser of the system: the ones of NXPress on GitHub, or its
// Discord server, and no other.
func (a *App) OpenURL(url string) {
	if pageOfNXPress(url) {
		openURL(a.ctx, url)
	}
}

// pageOfNXPress tells whether the page can open from the window.
func pageOfNXPress(url string) bool {
	return url == discordInvite || url == "https://github.com/DeLFuS77/NXPress" || strings.HasPrefix(url, "https://github.com/DeLFuS77/NXPress/")
}

// versionNumbers reads a version such as 1.2.3.
func versionNumbers(v string) ([3]int, bool) {
	var numbers [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return numbers, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return numbers, false
		}
		numbers[i] = n
	}
	return numbers, true
}

// newer tells whether version a comes after version b.
func newer(a string, b string) bool {
	x, okA := versionNumbers(a)
	y, okB := versionNumbers(b)
	if !okA || !okB {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

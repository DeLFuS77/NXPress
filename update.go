package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// the latest release of NXPress, as GitHub tells it
const latestRelease = "https://api.github.com/repos/DeLFuS77/NXPress/releases/latest"

// Update is a newer version of NXPress, when there is one.
type Update struct {
	Version string `json:"version"`
	URL     string `json:"url"`
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
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Update{}
	}
	defer response.Body.Close()
	var release struct {
		Tag string `json:"tag_name"`
		URL string `json:"html_url"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&release) != nil {
		return Update{}
	}
	if newer(release.Tag, version) && strings.HasPrefix(release.URL, "https://github.com/") {
		return Update{Version: release.Tag, URL: release.URL}
	}
	return Update{}
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

package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// the latest release of NXPress, as GitHub tells it: never a beta (a pre-release); and the last
// releases, betas too, which a beta asks for, so it hears of the next beta as of the next version
const (
	latestRelease = "https://api.github.com/repos/DeLFuS77/NXPress/releases/latest"
	lastReleases  = "https://api.github.com/repos/DeLFuS77/NXPress/releases?per_page=10"
)

// Update is a newer version of NXPress, when there is one.
type Update struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// CheckUpdate asks GitHub for the latest version of NXPress: one of the two times NXPress goes on
// the internet, with the versions published of the games (see published.go), and only from the
// window, when its settings allow it. Builds that are not a version do not ask.
func (a *App) CheckUpdate() Update {
	running, ok := versionNumbers(version)
	if !ok {
		return Update{}
	}
	url := latestRelease
	if running[3] != releaseRank {
		url = lastReleases
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	type release struct {
		Tag string `json:"tag_name"`
		URL string `json:"html_url"`
	}
	var releases []release
	if response.StatusCode != http.StatusOK {
		return Update{}
	}
	if url == latestRelease {
		var one release
		if json.NewDecoder(response.Body).Decode(&one) != nil {
			return Update{}
		}
		releases = []release{one}
	} else if json.NewDecoder(response.Body).Decode(&releases) != nil {
		return Update{}
	}
	best := Update{}
	for _, r := range releases {
		if newer(r.Tag, version) && (best.Version == "" || newer(r.Tag, best.Version)) && strings.HasPrefix(r.URL, "https://github.com/") {
			best = Update{Version: r.Tag, URL: r.URL}
		}
	}
	return best
}

// OpenURL opens a page in the browser of the system.
func (a *App) OpenURL(url string) {
	if strings.HasPrefix(url, "https://github.com/DeLFuS77/NXPress") {
		openURL(a.ctx, url)
	}
}

// releaseRank is the fourth number of a version that is not a beta: it comes after its betas.
const releaseRank = math.MaxInt

// versionNumbers reads a version such as 1.2.3, or a beta such as 1.2.3-beta.4: the fourth
// number is the one of the beta, releaseRank for a version.
func versionNumbers(v string) ([4]int, bool) {
	var numbers [4]int
	core, beta, isBeta := strings.Cut(strings.TrimPrefix(v, "v"), "-beta.")
	parts := strings.Split(core, ".")
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
	numbers[3] = releaseRank
	if isBeta {
		n, err := strconv.Atoi(beta)
		if err != nil || n < 1 {
			return numbers, false
		}
		numbers[3] = n
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

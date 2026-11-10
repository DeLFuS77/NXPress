// Package eshop finds the images of the games in the eShop of Nintendo, by their name, for a NUP
// whose game titledb has no icon of (#102): the games of the Switch 2 are often missing there.
// The search of the eShop of Europe is asked, which gives each game with a square image of 500×500;
// the name is the one the game carries, or one written, as file names and title IDs are often
// wrong. Nothing is asked until a search is started from the window.
package eshop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// SearchAddress is the search of the eShop of Europe, which the tests stand in for.
var SearchAddress = "https://searching.nintendo-europe.com/en/select"

// UserAgent is told to the servers.
var UserAgent = "NXPress"

const (
	// the most games a search gives
	MaxResults = 24
	// the time a search has, and an image
	searchWait = 20 * time.Second
	imageWait  = 30 * time.Second
	// an answer of the search is far smaller than this, and an image of 500×500 some 100 KB
	maxAnswer = 4 << 20
	MaxImage  = 8 << 20
	// a name longer than this is no name of a game
	maxQuery = 200
)

// Game is a game the eShop gives for a search: its name, the consoles it is for, and the address
// of its square image.
type Game struct {
	Title   string   `json:"title"`
	Systems []string `json:"systems"`
	Image   string   `json:"image"`
}

// tags are the marks file names carry after the name of a game: [title ID], [v65536], (EU)...
var tags = regexp.MustCompile(`\s*[\[(][^\])]*[\])]`)

// Query is what is searched for a name: without the marks of file names, the extension and the
// signs that are no part of a name.
func Query(name string) string {
	name = strings.TrimSpace(name)
	for _, ext := range []string{".nup", ".nsp", ".nsz", ".xci", ".xcz"} {
		if strings.HasSuffix(strings.ToLower(name), ext) {
			name = name[:len(name)-len(ext)]
		}
	}
	name = tags.ReplaceAllString(name, " ")
	name = strings.NewReplacer("_", " ", "™", "", "®", "").Replace(name)
	return strings.Join(strings.Fields(name), " ")
}

// Search asks the eShop for the games of a name, the ones of the Switch 2 only when asked; the
// games without a square image are left out.
func Search(ctx context.Context, client *http.Client, name string, onlySwitch2 bool) ([]Game, error) {
	query := Query(name)
	if query == "" {
		return nil, errors.New("write the name of the game")
	}
	if len(query) > maxQuery {
		return nil, errors.New("the name is too long")
	}
	filter := "type:GAME"
	if onlySwitch2 {
		filter += " AND system_type:nintendoswitch2"
	}
	values := url.Values{
		"q":  {query},
		"fq": {filter},
		"fl": {"title,system_names_txt,image_url_sq_s"},
		// a few more than shown, as some have no image
		"rows": {fmt.Sprint(MaxResults + 8)},
		"wt":   {"json"},
	}
	ctx, cancel := context.WithTimeout(ctx, searchWait)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, SearchAddress+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", UserAgent)
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("the eShop cannot be reached: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the eShop answered %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxAnswer+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAnswer {
		return nil, errors.New("the answer of the eShop is too large")
	}
	var answer struct {
		Response struct {
			Docs []struct {
				Title   string   `json:"title"`
				Systems []string `json:"system_names_txt"`
				Image   string   `json:"image_url_sq_s"`
			} `json:"docs"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &answer); err != nil {
		return nil, fmt.Errorf("the answer of the eShop cannot be read: %w", err)
	}
	games := []Game{}
	seen := map[string]bool{}
	for _, doc := range answer.Response.Docs {
		image := strings.TrimSpace(doc.Image)
		if !ImageAddress(image) || seen[image] || len(games) == MaxResults {
			continue
		}
		seen[image] = true
		games = append(games, Game{Title: strings.TrimSpace(doc.Title), Systems: doc.Systems, Image: image})
	}
	return games, nil
}

// ImageHosts are the servers the images of the eShop come from, and their subdomains; the tests
// add theirs.
var ImageHosts = []string{"nintendo.com", "nintendo-europe.com", "nintendo.net"}

// ImageAddress tells whether an address is of an image of the eShop: one of its servers, over
// HTTPS, so the window downloads nothing else from what a search gives.
func ImageAddress(address string) bool {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || len(address) > 2048 {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range ImageHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// DownloadImage downloads an image of the eShop.
func DownloadImage(ctx context.Context, client *http.Client, address string) ([]byte, error) {
	if !ImageAddress(address) {
		return nil, errors.New("not an image of the eShop")
	}
	ctx, cancel := context.WithTimeout(ctx, imageWait)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", UserAgent)
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("the image cannot be downloaded: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the image cannot be downloaded: %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxImage+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxImage {
		return nil, errors.New("the image is too large")
	}
	return data, nil
}

// WebSearch is the address of a search of images on the web for a name, for the games the eShop
// does not have: DuckDuckGo, which keeps no record of who searches.
func WebSearch(name string) string {
	query := Query(name)
	if query == "" {
		query = "Nintendo Switch 2"
	}
	return "https://duckduckgo.com/?" + url.Values{"q": {query + " Switch 2 cover"}, "ia": {"images"}, "iax": {"images"}}.Encode()
}

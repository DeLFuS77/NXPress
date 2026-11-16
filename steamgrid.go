package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The images of SteamGridDB (steamgriddb.com) for the cover of a NUP in Combine: the games of a name,
// then the icons, grids, heroes or logos of one, searched by its API (v2), which asks for a key: a
// free one, from the profile of the person on the site, that the window keeps and gives with each
// call. Like the eShop, the page loads no image from the internet: they come as addresses of data.

// steamGridAPI is where the API of SteamGridDB is.
const steamGridAPI = "https://www.steamgriddb.com/api/v2"

// steamClient is the client SteamGridDB is asked with.
var steamClient = &http.Client{Timeout: 40 * time.Second}

// errSteamKey is what a key SteamGridDB does not take gives; the page tells the person by it.
var errSteamKey = errors.New("SteamGridDB did not accept the API key")

// steamDownloads is how many thumbnails of a search are downloaded at once.
const steamDownloads = 6

// SteamGame is a game SteamGridDB found by a name.
type SteamGame struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Verified bool   `json:"verified"`
}

// SteamImage is an image of a game: how it is, its thumbnail as an address of data for the page, and
// the address of the image itself, to choose it by.
type SteamImage struct {
	ID      int    `json:"id"`
	Style   string `json:"style"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Mime    string `json:"mime"`
	Address string `json:"address"`
	Thumb   string `json:"thumb"`
}

// steamAddress tells whether an address is one of an image of SteamGridDB: https, on its site or the
// ones of its own, so that no other place is asked for what the page names.
func steamAddress(address string) bool {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "steamgriddb.com" || strings.HasSuffix(host, ".steamgriddb.com")
}

// steamGet asks the API for a path and gives the data of its answer.
func steamGet(ctx context.Context, key string, path string) (json.RawMessage, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errSteamKey
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, steamGridAPI+path, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Accept", "application/json")
	response, err := steamClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("SteamGridDB cannot be reached: %w", err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, errSteamKey
	case http.StatusNotFound:
		// a game it has nothing of
		return json.RawMessage("[]"), nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("SteamGridDB cannot be read: %w", err)
	}
	var reply struct {
		Success bool            `json:"success"`
		Errors  []string        `json:"errors"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		return nil, fmt.Errorf("SteamGridDB answered %s, which cannot be read", response.Status)
	}
	if response.StatusCode != http.StatusOK || !reply.Success {
		if len(reply.Errors) > 0 {
			return nil, fmt.Errorf("SteamGridDB: %s", reply.Errors[0])
		}
		return nil, fmt.Errorf("SteamGridDB answered %s", response.Status)
	}
	return reply.Data, nil
}

// steamImage downloads an image of SteamGridDB, no bigger than limit bytes.
func steamImage(ctx context.Context, address string, limit int64) ([]byte, error) {
	if !steamAddress(address) {
		return nil, errors.New("not an image of SteamGridDB")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "image/*")
	response, err := steamClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("the image cannot be downloaded: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the image cannot be downloaded: %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("the image cannot be downloaded: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, errors.New("the image is too big")
	}
	return data, nil
}

// dataAddress is an image as an address of data the page can show.
func dataAddress(data []byte) string {
	return "data:" + http.DetectContentType(data) + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// SearchSteamGrid asks SteamGridDB for the games of a name.
func (a *App) SearchSteamGrid(key string, name string) ([]SteamGame, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("write the name of a game")
	}
	if len(name) > 200 {
		name = name[:200]
	}
	data, err := steamGet(a.context(), key, "/search/autocomplete/"+url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	games := []SteamGame{}
	if err := json.Unmarshal(data, &games); err != nil {
		return nil, errors.New("SteamGridDB gave a list of games that cannot be read")
	}
	if len(games) > 20 {
		games = games[:20]
	}
	return games, nil
}

// SteamGridImages gives a page of the images of a kind (icons, grids, heroes or logos) of a game, the
// first ones when SteamGridDB sorts them: with their thumbnails, a game whose thumbnail does not come
// left out.
func (a *App) SteamGridImages(key string, game int, kind string, page int) ([]SteamImage, error) {
	switch kind {
	case "icons", "grids", "heroes", "logos":
	default:
		return nil, errors.New("not a kind of image of SteamGridDB")
	}
	if game <= 0 {
		return nil, errors.New("not a game of SteamGridDB")
	}
	if page < 0 {
		page = 0
	}
	data, err := steamGet(a.context(), key, fmt.Sprintf("/%s/game/%d?nsfw=false&humor=false&page=%s", kind, game, strconv.Itoa(page)))
	if err != nil {
		return nil, err
	}
	var items []struct {
		ID     int    `json:"id"`
		Style  string `json:"style"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Mime   string `json:"mime"`
		URL    string `json:"url"`
		Thumb  string `json:"thumb"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, errors.New("SteamGridDB gave a list of images that cannot be read")
	}
	if len(items) > 50 {
		items = items[:50]
	}
	thumbs := make([][]byte, len(items))
	var wait sync.WaitGroup
	slots := make(chan struct{}, steamDownloads)
	for i, item := range items {
		wait.Add(1)
		go func() {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			address := item.Thumb
			if address == "" {
				address = item.URL
			}
			thumbs[i], _ = steamImage(a.context(), address, 3<<20)
		}()
	}
	wait.Wait()
	images := []SteamImage{}
	failed := 0
	for i, item := range items {
		if len(thumbs[i]) == 0 || !steamAddress(item.URL) {
			failed++
			continue
		}
		images = append(images, SteamImage{ID: item.ID, Style: item.Style, Width: item.Width, Height: item.Height,
			Mime: item.Mime, Address: item.URL, Thumb: dataAddress(thumbs[i])})
	}
	if len(images) == 0 && failed > 0 {
		return nil, errors.New("the images of SteamGridDB cannot be downloaded: the network may not let them through")
	}
	return images, nil
}

// SteamGridPicture downloads the image chosen, as an address of data the page can cut.
func (a *App) SteamGridPicture(address string) (string, error) {
	data, err := steamImage(a.context(), address, 25<<20)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		return "", errors.New("what SteamGridDB gave is not an image")
	}
	return dataAddress(data), nil
}

// steamGridSite is the page of the site that searches a game by its name, which the person sees in the
// browser with no key: the image she chooses there is copied and pasted in the window.
func steamGridSite(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > 200 {
		name = name[:200]
	}
	return "https://www.steamgriddb.com/search/grids?term=" + strings.ReplaceAll(url.QueryEscape(name), "+", "%20")
}

// OpenSteamGridSite opens, in the browser, the search of SteamGridDB for the name of a game.
func (a *App) OpenSteamGridSite(name string) {
	openURL(a.ctx, steamGridSite(name))
}

// OpenSteamGridKeyPage opens, in the browser, the page of the profile where the API key is.
func (a *App) OpenSteamGridKeyPage() {
	openURL(a.ctx, "https://www.steamgriddb.com/profile/preferences/api")
}

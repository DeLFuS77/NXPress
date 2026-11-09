package titledb

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dtrunk90/switch-library-manager-web/settings"
	"github.com/klauspost/compress/zstd"
)

// The icons of the games, from titles.json of the data release of the library app: a NUP made
// from a dump folder without control data carries no icon, and gets the one titledb has of its
// game. titles.json is large (23 MB compressed, 125 MB as it is), so it is read as it downloads
// and only the address of the icon of each title is kept, in titles.icons.json (a few MB). It is
// downloaded only when a NUP needs an icon, once a week at most.
const (
	IconsFile  = "titles.icons.json"
	iconsState = "titles.icons.state.json"
)

// IconsEvery is how long the list of the icons kept is used as it is, before the server is
// asked again: the icons of the games seldom change.
const IconsEvery = 7 * 24 * time.Hour

// IconAddresses are where titles.json is downloaded from, compressed first.
var IconAddresses = settings.CompressedFirst(settings.DEFAULT_TITLES_JSON_URL)

const (
	// the time titles.json has to come whole, and an icon
	titlesWait = 5 * time.Minute
	iconWait   = 30 * time.Second
	// titles.json as it is, and compressed, is far smaller than this
	maxTitlesSize = 1 << 30
	// an icon of the eShop is some 100 KB
	maxIconDownload = 16 << 20
)

// Icons are the addresses of the icons of the games, by title ID.
type Icons struct {
	urls map[uint64]string
}

// GameID is the title ID of the game of a title: the one of the game for itself, its updates
// (the game's with 0x800) and its DLC (the game's with 0x1000 and the number of the DLC).
func GameID(id uint64) uint64 {
	return id &^ 0x1FFF
}

// URL gives the address of the icon of the game of a title, when the list knows it.
func (i *Icons) URL(id uint64) (string, bool) {
	if i == nil {
		return "", false
	}
	address, ok := i.urls[GameID(id)]
	return address, ok
}

// LoadIcons reads the list kept in a folder, without going on the internet; it fails when there
// is none.
func LoadIcons(folder string) (*Icons, error) {
	data, err := os.ReadFile(filepath.Join(folder, IconsFile))
	if err != nil {
		return nil, err
	}
	urls, err := parseIcons(data)
	if err != nil {
		return nil, err
	}
	return &Icons{urls: urls}, nil
}

// ForgetIcons removes the list kept and what was remembered of it.
func ForgetIcons(folder string) error {
	return errors.Join(remove(filepath.Join(folder, IconsFile)), remove(filepath.Join(folder, iconsState)))
}

// RefreshIcons gives the icons of the games, downloading titles.json again when the list kept is a
// week old or more (or there is none), asking the server that gave it whether it changed. When no
// server gives it, with no connection most often, the list kept is used and the error tells why;
// with no list either, the icons are nil.
func RefreshIcons(ctx context.Context, folder string, client *http.Client, now time.Time) (*Icons, error) {
	kept, keptErr := LoadIcons(folder)
	known := readStateOf(folder, iconsState)
	if keptErr == nil && !known.Checked.After(now) && now.Sub(known.Checked) < IconsEvery {
		return kept, nil
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return kept, err
	}
	failures := []error{}
	for _, address := range IconAddresses {
		ask := state{}
		if keptErr == nil && known.Address == address {
			ask = known
		}
		urls, answer, err := fetchIcons(ctx, client, address, ask)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", address, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		answer.Checked = now
		if urls == nil {
			return kept, save(folder, iconsState, answer)
		}
		data, err := json.Marshal(iconsText(urls))
		if err != nil {
			return kept, err
		}
		if err := write(folder, IconsFile, data); err != nil {
			return kept, err
		}
		return &Icons{urls: urls}, save(folder, iconsState, answer)
	}
	return kept, errors.Join(failures...)
}

// fetchIcons downloads titles.json from an address and keeps the icon of each title; no icons
// when the server says the copy kept (named by ask) has not changed.
func fetchIcons(ctx context.Context, client *http.Client, address string, ask state) (map[uint64]string, state, error) {
	ctx, cancel := context.WithTimeout(ctx, titlesWait)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, state{}, err
	}
	request.Header.Set("User-Agent", UserAgent)
	if ask.ETag != "" {
		request.Header.Set("If-None-Match", ask.ETag)
	}
	if ask.Modified != "" {
		request.Header.Set("If-Modified-Since", ask.Modified)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, state{}, err
	}
	defer response.Body.Close()
	answer := state{Address: address, ETag: response.Header.Get("ETag"), Modified: response.Header.Get("Last-Modified")}
	switch response.StatusCode {
	case http.StatusNotModified:
		if answer.ETag == "" {
			answer.ETag = ask.ETag
		}
		if answer.Modified == "" {
			answer.Modified = ask.Modified
		}
		return nil, answer, nil
	case http.StatusOK:
	default:
		return nil, state{}, errors.New(response.Status)
	}
	var body io.Reader = &capped{reader: response.Body, left: maxTitlesSize}
	if strings.HasSuffix(address, settings.COMPRESSED_SUFFIX) {
		decoder, err := zstd.NewReader(body, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(1<<27))
		if err != nil {
			return nil, state{}, err
		}
		defer decoder.Close()
		body = &capped{reader: decoder, left: maxTitlesSize}
	}
	urls, err := readTitleIcons(body)
	if err != nil {
		return nil, state{}, err
	}
	return urls, answer, nil
}

// capped reads at most left bytes, and fails past them.
type capped struct {
	reader io.Reader
	left   int64
}

func (c *capped) Read(p []byte) (int, error) {
	if c.left <= 0 {
		return 0, errors.New("the file is too large")
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.reader.Read(p)
	c.left -= int64(n)
	return n, err
}

// readTitleIcons reads titles.json entry by entry, {"<title ID>": {"id": ..., "iconUrl": ...}},
// and keeps the icon of each game.
func readTitleIcons(reader io.Reader) (map[uint64]string, error) {
	decoder := json.NewDecoder(bufio.NewReaderSize(reader, 1<<20))
	if token, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("titles.json cannot be read: %w", err)
	} else if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("titles.json cannot be read: it is not a JSON object")
	}
	urls, fromGame := map[uint64]string{}, map[uint64]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("titles.json cannot be read: %w", err)
		}
		key, _ := token.(string)
		var title struct {
			ID      string `json:"id"`
			IconURL string `json:"iconUrl"`
		}
		if err := decoder.Decode(&title); err != nil {
			return nil, fmt.Errorf("titles.json cannot be read: %w", err)
		}
		if title.ID == "" {
			title.ID = key
		}
		id, err := strconv.ParseUint(title.ID, 16, 64)
		if err != nil || len(title.ID) != 16 || !webAddress(title.IconURL) {
			continue
		}
		// the icon of the game, else the first of its DLC that has one
		game := GameID(id)
		switch _, found := urls[game]; {
		case fromGame[game]:
		case id == game:
			urls[game], fromGame[game] = title.IconURL, true
		case !found:
			urls[game] = title.IconURL
		}
	}
	if len(urls) == 0 {
		return nil, errors.New("titles.json lists no icon")
	}
	return urls, nil
}

// webAddress tells an address of the web, as the icons of titledb are.
func webAddress(address string) bool {
	parsed, err := url.Parse(address)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && len(address) <= 2048
}

// iconsText is the list of the icons as it is kept: by title ID in 16 hexadecimal digits.
func iconsText(urls map[uint64]string) map[string]string {
	text := make(map[string]string, len(urls))
	for id, address := range urls {
		text[fmt.Sprintf("%016X", id)] = address
	}
	return text
}

// parseIcons reads the list kept.
func parseIcons(data []byte) (map[uint64]string, error) {
	var text map[string]string
	if err := json.Unmarshal(data, &text); err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", IconsFile, err)
	}
	urls := make(map[uint64]string, len(text))
	for key, address := range text {
		id, err := strconv.ParseUint(key, 16, 64)
		if err != nil || len(key) != 16 || !webAddress(address) {
			continue
		}
		urls[GameID(id)] = address
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("%s lists no icon", IconsFile)
	}
	return urls, nil
}

// DownloadIcon downloads the icon of an address: the image as the server gives it.
func DownloadIcon(ctx context.Context, client *http.Client, address string) ([]byte, error) {
	if !webAddress(address) {
		return nil, errors.New("not an address of the web")
	}
	ctx, cancel := context.WithTimeout(ctx, iconWait)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", UserAgent)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New(response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxIconDownload+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxIconDownload {
		return nil, errors.New("the icon is too large")
	}
	return data, nil
}

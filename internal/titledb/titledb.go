// Package titledb keeps the versions published of each game and DLC, from the versions.json the
// library app (Switch Library Manager Web) downloads: NXPress tells with them the updates that
// are out and not in its list. The window downloads it, once a day at most; the command line
// only reads the copy kept.
package titledb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DeLFuS77/NXPress/internal/durable"

	"github.com/dtrunk90/switch-library-manager-web/settings"
	"github.com/klauspost/compress/zstd"
)

// FileName is the copy kept in the folder of NXPress (see portable.DataFolder), and stateName
// what NXPress remembers of its download: when the server was asked, and how it named its copy.
const (
	FileName  = settings.VERSIONS_JSON_FILENAME
	stateName = "versions.state.json"
)

// Every is how long the copy kept is used as it is, before the server is asked again.
const Every = 24 * time.Hour

// Addresses are where versions.json is downloaded from, in order, as the library app tries them:
// the one of its data release compressed (a seventh of its size), the same as it is, and the one
// of blawar/titledb.
var Addresses = settings.CompressedFirst(settings.DEFAULT_VERSIONS_JSON_URL, settings.BLAWAR_VERSIONS_JSON_URL)

// UserAgent is how NXPress names itself to the server: main adds its version.
var UserAgent = "NXPress"

const (
	// the time each server has to give the file: it is 200 KB compressed, 1.2 MB as it is
	askWait = 30 * time.Second
	// versions.json is far smaller than this, as it is or decompressed: a server that sends more
	// is not giving it
	maxSize = 64 << 20
)

// Published is the newest version published of a title, and the day it was (2026-10-21).
type Published struct {
	Version int    `json:"version"`
	Date    string `json:"date"`
}

// Versions are the newest versions published, by title ID: the one of a game for its updates,
// the one of each DLC for its own.
type Versions struct {
	newest map[uint64]Published
	// the day of the list: as the server dated it, else the one it was downloaded
	Date time.Time
}

// Newest gives the newest version published of a title, when the list knows it.
func (v *Versions) Newest(id uint64) (Published, bool) {
	if v == nil {
		return Published{}, false
	}
	published, ok := v.newest[id]
	return published, ok
}

// state is what NXPress remembers of the copy kept: when the server last answered, which address
// gave the copy, and how the server named that copy, to ask it whether it changed.
type state struct {
	Checked  time.Time `json:"checked"`
	Address  string    `json:"address"`
	ETag     string    `json:"etag,omitempty"`
	Modified string    `json:"modified,omitempty"`
}

func readState(folder string) state {
	var kept state
	if data, err := os.ReadFile(filepath.Join(folder, stateName)); err == nil {
		_ = json.Unmarshal(data, &kept)
	}
	return kept
}

// Load reads the copy kept in a folder, without going on the internet; it fails when there is
// none.
func Load(folder string) (*Versions, error) {
	path := filepath.Join(folder, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	newest, err := parse(data)
	if err != nil {
		return nil, err
	}
	versions := &Versions{newest: newest}
	kept := readState(folder)
	switch day, err := http.ParseTime(kept.Modified); {
	case err == nil:
		versions.Date = day
	case !kept.Checked.IsZero():
		versions.Date = kept.Checked
	default:
		if info, err := os.Stat(path); err == nil {
			versions.Date = info.ModTime()
		}
	}
	return versions, nil
}

// Forget removes the copy kept and what was remembered of it.
func Forget(folder string) error {
	return errors.Join(remove(filepath.Join(folder, FileName)), remove(filepath.Join(folder, stateName)))
}

func remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Refresh gives the versions published, downloading them again into folder when the copy kept is
// a day old or more: the server that gave it is asked whether it changed (If-None-Match,
// If-Modified-Since) and sends it only then. When no server gives a good copy, with no connection
// most often, the copy kept is used and the error tells why; with no copy either, the versions
// are nil.
func Refresh(ctx context.Context, folder string, client *http.Client, now time.Time) (*Versions, error) {
	kept, err := Load(folder)
	known := readState(folder)
	if err == nil && !known.Checked.After(now) && now.Sub(known.Checked) < Every {
		return kept, nil
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return kept, err
	}
	failures := []error{}
	for _, address := range Addresses {
		// what the server named the copy kept tells only of the address it came from
		ask := state{}
		if kept != nil && known.Address == address {
			ask = known
		}
		data, answer, err := fetch(ctx, client, address, ask)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", address, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		answer.Checked = now
		if data == nil {
			// not changed: the copy kept is good for a day more
			return kept, save(folder, stateName, answer)
		}
		newest, err := parse(data)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", address, err))
			continue
		}
		if err := write(folder, FileName, data); err != nil {
			return kept, err
		}
		versions := &Versions{newest: newest, Date: now}
		if day, err := http.ParseTime(answer.Modified); err == nil {
			versions.Date = day
		}
		return versions, save(folder, stateName, answer)
	}
	return kept, errors.Join(failures...)
}

// fetch downloads versions.json from an address, decompressed when the address is of the file
// compressed; it gives no data when the server says the copy kept (named by ask) has not changed.
func fetch(ctx context.Context, client *http.Client, address string, ask state) ([]byte, state, error) {
	ctx, cancel := context.WithTimeout(ctx, askWait)
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
		// a name of the copy the server leaves out is the one it gave before (GitHub does not
		// give its day again)
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
	data, err := readAtMost(response.Body)
	if err != nil {
		return nil, state{}, err
	}
	if strings.HasSuffix(address, settings.COMPRESSED_SUFFIX) {
		decoder, err := zstd.NewReader(bytes.NewReader(data), zstd.WithDecoderConcurrency(1))
		if err != nil {
			return nil, state{}, err
		}
		defer decoder.Close()
		if data, err = readAtMost(decoder); err != nil {
			return nil, state{}, err
		}
	}
	return data, answer, nil
}

// readAtMost reads all of a reader, failing when it gives more than maxSize.
func readAtMost(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize {
		return nil, errors.New("the file is too large")
	}
	return data, nil
}

// parse reads versions.json: for each title ID, its versions and the day each was published, of
// which the newest is kept.
func parse(data []byte) (map[uint64]Published, error) {
	var all map[string]map[string]string
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("versions.json cannot be read: %w", err)
	}
	newest := make(map[uint64]Published, len(all))
	for key, versions := range all {
		id, err := strconv.ParseUint(key, 16, 64)
		if err != nil || len(key) != 16 {
			continue
		}
		best := Published{Version: -1}
		for number, date := range versions {
			version, err := strconv.Atoi(number)
			if err != nil || version <= best.Version {
				continue
			}
			// a day that is not one is left out, the version kept
			if _, err := time.Parse(time.DateOnly, date); err != nil {
				date = ""
			}
			best = Published{Version: version, Date: date}
		}
		if best.Version >= 0 {
			newest[id] = best
		}
	}
	if len(newest) == 0 {
		return nil, errors.New("versions.json lists no title")
	}
	return newest, nil
}

// save keeps the state of the copy.
func save(folder string, name string, kept state) error {
	data, err := json.MarshalIndent(kept, "", "  ")
	if err != nil {
		return err
	}
	return write(folder, name, data)
}

// write gives a file of the folder new contents whole: a power cut leaves the old ones.
func write(folder string, name string, data []byte) error {
	file, err := os.CreateTemp(folder, "."+name+".*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	_, err = file.Write(data)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = durable.CommitFile(temporary, filepath.Join(folder, name))
	}
	if err != nil {
		os.Remove(temporary)
	}
	return err
}

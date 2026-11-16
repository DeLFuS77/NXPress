package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/DeLFuS77/NXPress/internal/selfupdate"
)

// Whether the program running is an official version of NXPress: one built by the workflow of its
// repository and published there, with its checksums signed with the key of NXPress (see
// TRADEMARKS.md). The window tells a build that is not one, so a problem with it goes to whoever
// made it, and the report of a work says it too.
//
// It is known by the checksums of the version the program says it is: the program of Windows is a
// file of the release on its own (nxpress.exe), and the programs inside the archives of macOS and
// Linux are in nxpress-<version>-programs.sha256, a file of the release whose checksum is signed
// with the others. It is asked of GitHub only when the window asks for updates.

// Official is what is known of the program running.
type Official struct {
	// "official", "unofficial" (a version with no such program, or no such version), "dev" (a
	// build that is no version) or "unknown" (not known: offline, or a release before the list of
	// programs)
	State string `json:"state"`
}

const officialDownloads = "https://github.com/DeLFuS77/NXPress/releases/download/"

var (
	officialOnce  sync.Once
	officialState Official
)

// Official tells whether the program running is an official version, asked once.
func (a *App) Official() Official {
	officialOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		officialState = checkOfficial(ctx, version, runningProgram, selfupdate.Keys(), officialGet)
		// a failure is asked again the next time
		if officialState.State == "unknown" {
			officialOnce = sync.Once{}
		}
	})
	return officialState
}

// errNotFound is a file the release does not have
var errNotFound = errors.New("not found")

// officialGet reads a small file of a release.
func officialGet(ctx context.Context, address string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "NXPress/"+version)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 1<<20))
}

// runningProgram is the checksum of the program running.
func runningProgram() (string, error) {
	program, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(program); err == nil {
		program = resolved
	}
	file, err := os.Open(program)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// programsName is the file of a release with the checksums of the programs inside its archives.
func programsName(tag string) string {
	return "nxpress-" + tag + "-programs.sha256"
}

func checkOfficial(ctx context.Context, tag string, program func() (string, error), keys []selfupdate.PublicKey, get func(context.Context, string) ([]byte, error)) Official {
	if _, ok := versionNumbers(tag); !ok {
		return Official{State: "dev"}
	}
	if len(keys) == 0 {
		return Official{State: "unknown"}
	}
	base := officialDownloads + tag + "/"
	sums, err := get(ctx, base+selfupdate.SumsName)
	if errors.Is(err, errNotFound) {
		// no version of that number was published
		return Official{State: "unofficial"}
	}
	if err != nil {
		return Official{State: "unknown"}
	}
	signature, err := get(ctx, base+selfupdate.SignatureName)
	if err != nil {
		return Official{State: "unknown"}
	}
	if _, err := selfupdate.Verify(sums, signature, keys); err != nil {
		return Official{State: "unknown"}
	}
	own, err := program()
	if err != nil {
		return Official{State: "unknown"}
	}
	listed := selfupdate.ParseSums(sums)
	for _, sum := range listed {
		if sum == own {
			return Official{State: "official"}
		}
	}
	name := programsName(tag)
	want, ok := listed[name]
	if !ok {
		// a release before the list of programs: only the program of Windows is a file of its own
		if runtime.GOOS == "windows" {
			return Official{State: "unofficial"}
		}
		return Official{State: "unknown"}
	}
	programs, err := get(ctx, base+name)
	if err != nil {
		return Official{State: "unknown"}
	}
	if sum := sha256.Sum256(programs); hex.EncodeToString(sum[:]) != want {
		return Official{State: "unknown"}
	}
	for _, sum := range selfupdate.ParseSums(programs) {
		if sum == own {
			return Official{State: "official"}
		}
	}
	return Official{State: "unofficial"}
}

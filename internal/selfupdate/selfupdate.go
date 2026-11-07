package selfupdate

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/hex"
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

	"crypto/sha256"
)

// releaseKeys are the public keys of the project the releases are signed with: written by the
// tool that makes the key (tools/releasekey), which puts the private one in the secret of the
// releases. With none, NXPress only tells of a new version, as before.
//
//go:embed release.pub
var releaseKeys string

// Keys are the public keys of the project, the ones NXPress takes a release signed with.
func Keys() []PublicKey {
	keys, err := ParsePublicKeys(releaseKeys)
	if err != nil {
		return nil
	}
	return keys
}

// The files of a release the update reads: the checksums of all its files, and their signature.
const (
	SumsName      = "SHA256SUMS.txt"
	SignatureName = SumsName + ".minisig"
)

// Release is a version of NXPress on GitHub, with the addresses of its files by name.
type Release struct {
	Tag        string
	Prerelease bool
	Assets     map[string]string
}

// What keeps an update from being installed: each told in the language of the window.
var (
	ErrUnsigned    = errors.New("the version carries no signed checksums")
	ErrUnsupported = errors.New("the version has no program for this system that NXPress can put in place of itself")
	ErrChecksum    = errors.New("the file downloaded does not match its signed checksum")
	ErrNoWrite     = errors.New("NXPress cannot write in its folder")
	ErrNotNewer    = errors.New("the version is not newer than this one")
	ErrPrerelease  = errors.New("the version is a beta, and this NXPress is not")
)

// Code is the reason of an error, as the window names it.
func Code(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, ErrSignature):
		return "signature"
	case errors.Is(err, ErrUnsigned):
		return "unsigned"
	case errors.Is(err, ErrChecksum):
		return "checksum"
	case errors.Is(err, ErrNoWrite):
		return "write"
	case errors.Is(err, ErrUnsupported):
		return "unsupported"
	case errors.Is(err, ErrNotNewer):
		return "version"
	case errors.Is(err, ErrPrerelease):
		return "beta"
	}
	return "failed"
}

// AssetName is the file of a release with the program of a system: the program of Windows on
// its own, the archive of Linux. macOS keeps its program in an app, which NXPress does not
// replace while it runs: there it only tells of the new version.
func AssetName(goos, goarch, tag string) string {
	switch {
	case goos == "windows" && goarch == "amd64":
		return "nxpress.exe"
	case goos == "linux" && goarch == "amd64":
		return "nxpress-" + tag + "-linux-x64.tar.gz"
	case goos == "linux" && goarch == "arm64":
		return "nxpress-" + tag + "-linux-arm64.tar.gz"
	}
	return ""
}

// Installable tells whether a release can be installed here: it is signed, there are keys to
// check it with, and it has the program of this system.
func Installable(release Release, keys []PublicKey, goos, goarch string) bool {
	name := AssetName(goos, goarch, release.Tag)
	return len(keys) > 0 && name != "" && release.Assets[SumsName] != "" && release.Assets[SignatureName] != "" && release.Assets[name] != ""
}

// Newer tells whether version a comes after b: 1.2.3 after 1.2.2, and a version after its own
// betas (1.2.3 after 1.2.3-beta.4, which comes after 1.2.3-beta.3).
func Newer(a, b string) bool {
	numbersA, betaA, okA := parseVersion(a)
	numbersB, betaB, okB := parseVersion(b)
	if !okA || !okB {
		return false
	}
	for i := range numbersA {
		if numbersA[i] != numbersB[i] {
			return numbersA[i] > numbersB[i]
		}
	}
	switch {
	case betaA == betaB:
		return false
	case betaA == 0:
		return true
	case betaB == 0:
		return false
	}
	return betaA > betaB
}

// IsBeta tells a version of the beta line: 1.2.3-beta.4.
func IsBeta(v string) bool {
	_, beta, ok := parseVersion(v)
	return ok && beta > 0
}

// parseVersion reads 1.2.3, or 1.2.3-beta.4 (beta 4; 0 for no beta).
func parseVersion(v string) ([3]int, int, bool) {
	var numbers [3]int
	v = strings.TrimPrefix(v, "v")
	main, beta, isBeta := strings.Cut(v, "-beta.")
	parts := strings.Split(main, ".")
	if len(parts) != 3 {
		return numbers, 0, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return numbers, 0, false
		}
		numbers[i] = n
	}
	if !isBeta {
		return numbers, 0, true
	}
	n, err := strconv.Atoi(beta)
	if err != nil || n <= 0 {
		return numbers, 0, false
	}
	return numbers, n, true
}

// the hosts the files of a release come from: GitHub, and the ones it sends their downloads to
var hosts = map[string]bool{"github.com": true, "objects.githubusercontent.com": true, "release-assets.githubusercontent.com": true}

// fromGitHub tells an address NXPress downloads a release from.
func fromGitHub(u *url.URL) bool {
	return u.Scheme == "https" && hosts[u.Hostname()]
}

// Installer puts a release in place of the running NXPress.
type Installer struct {
	// the program running, its version, its system, and the keys it checks releases with
	Program, Version, GOOS, GOARCH string
	Keys                           []PublicKey
	// the client of the downloads, and the addresses it takes (tests give their own)
	Client  *http.Client
	Allowed func(*url.URL) bool
	// told as it goes: "download" with the bytes done of the total, then "check" and "replace"
	Progress func(step string, done, total int64)
}

// the most each file may weigh: the checksums and their signature are small, a program is not
// bigger than this
const (
	smallLimit   = 1 << 20
	programLimit = 512 << 20
)

// Install downloads the program of a release for this system and puts it in place of the
// running one, when everything checks: the release is newer, a beta only for a beta, its
// checksums carry the signature of the project for this version, and the file downloaded has
// its checksum. Nothing is replaced before; a failure leaves NXPress as it was.
func (in Installer) Install(ctx context.Context, release Release) error {
	if !Newer(release.Tag, in.Version) {
		return ErrNotNewer
	}
	if release.Prerelease && !IsBeta(in.Version) {
		return ErrPrerelease
	}
	name := AssetName(in.GOOS, in.GOARCH, release.Tag)
	if name == "" || release.Assets[name] == "" {
		return ErrUnsupported
	}
	if release.Assets[SumsName] == "" || release.Assets[SignatureName] == "" || len(in.Keys) == 0 {
		return ErrUnsigned
	}
	sums, err := in.get(ctx, release.Assets[SumsName], smallLimit)
	if err != nil {
		return err
	}
	signature, err := in.get(ctx, release.Assets[SignatureName], smallLimit)
	if err != nil {
		return err
	}
	// the signature tells the version it was made for: checksums signed for another one, an
	// older release given as this one, do not pass
	comment, err := Verify(sums, signature, in.Keys)
	if err != nil {
		return err
	}
	if comment != "NXPress "+release.Tag {
		return fmt.Errorf("%w: signed for %q", ErrSignature, comment)
	}
	want, ok := ParseSums(sums)[name]
	if !ok {
		return ErrUnsigned
	}

	folder := filepath.Dir(in.Program)
	part := filepath.Join(folder, ".nxpress-update-"+release.Tag+".part")
	defer os.Remove(part)
	if err := in.download(ctx, release.Assets[name], part, want); err != nil {
		return err
	}
	in.progress("check", 0, 0)
	fresh := part
	if strings.HasSuffix(name, ".tar.gz") {
		fresh = filepath.Join(folder, ".nxpress-update-"+release.Tag)
		defer os.Remove(fresh)
		if err := extractProgram(part, fresh); err != nil {
			return err
		}
	}
	in.progress("replace", 0, 0)
	return Replace(in.Program, fresh)
}

func (in Installer) progress(step string, done, total int64) {
	if in.Progress != nil {
		in.Progress(step, done, total)
	}
}

func (in Installer) client() *http.Client {
	client := http.DefaultClient
	if in.Client != nil {
		client = in.Client
	}
	allowed := in.Allowed
	if allowed == nil {
		allowed = fromGitHub
	}
	// every redirection stays on the hosts of GitHub
	checked := *client
	checked.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !allowed(request.URL) {
			return errors.New("a download that leaves GitHub")
		}
		return nil
	}
	return &checked
}

func (in Installer) open(ctx context.Context, address string) (*http.Response, error) {
	u, err := url.Parse(address)
	allowed := in.Allowed
	if allowed == nil {
		allowed = fromGitHub
	}
	if err != nil || !allowed(u) {
		return nil, fmt.Errorf("not an address of GitHub: %s", address)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "NXPress/"+in.Version)
	response, err := in.client().Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("GitHub answered %s", response.Status)
	}
	return response, nil
}

// get reads a small file of a release.
func (in Installer) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	response, err := in.open(ctx, address)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("a file of the release is too big")
	}
	return data, nil
}

// download writes a file of a release to path, and checks its SHA-256 as it comes.
func (in Installer) download(ctx context.Context, address string, path string, want string) error {
	output, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNoWrite, err)
	}
	defer output.Close()
	response, err := in.open(ctx, address)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	hash := sha256.New()
	total := response.ContentLength
	var done int64
	buffer := make([]byte, 256<<10)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			done += int64(n)
			if done > programLimit {
				return errors.New("the file of the release is too big")
			}
			hash.Write(buffer[:n])
			if _, err := output.Write(buffer[:n]); err != nil {
				return fmt.Errorf("%w: %v", ErrNoWrite, err)
			}
			in.progress("download", done, total)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), want) {
		return ErrChecksum
	}
	if err := output.Sync(); err != nil {
		return fmt.Errorf("%w: %v", ErrNoWrite, err)
	}
	return output.Close()
}

// ParseSums reads a file of checksums as sha256sum writes it: the checksum, two spaces (or a
// space and a star) and the name, by name.
func ParseSums(data []byte) map[string]string {
	sums := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		sum, name, ok := strings.Cut(strings.TrimSpace(scanner.Text()), " ")
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if ok && len(sum) == 64 && name != "" {
			if _, err := hex.DecodeString(sum); err == nil {
				sums[name] = strings.ToLower(sum)
			}
		}
	}
	return sums
}

// extractProgram takes the program of NXPress out of the archive of Linux, NXPress/nxpress, to
// path.
func extractProgram(archive string, path string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	unzipped, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	reader := tar.NewReader(unzipped)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return ErrUnsupported
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg || strings.TrimPrefix(header.Name, "./") != "NXPress/nxpress" {
			continue
		}
		output, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrNoWrite, err)
		}
		if _, err := io.Copy(output, io.LimitReader(reader, programLimit)); err != nil {
			output.Close()
			return err
		}
		if err := output.Sync(); err != nil {
			output.Close()
			return err
		}
		return output.Close()
	}
}

// Replace puts fresh in place of the program: the program is renamed out of the way, which a
// running program allows, then fresh takes its name; when that fails, the program gets its name
// back. What was renamed is deleted by CleanOld once it stops.
func Replace(program string, fresh string) error {
	old := program + ".old"
	if os.Remove(old) != nil {
		if _, err := os.Lstat(old); err == nil {
			old = fmt.Sprintf("%s.%d.old", program, time.Now().UnixNano())
		}
	}
	if err := os.Rename(program, old); err != nil {
		return fmt.Errorf("%w: %v", ErrNoWrite, err)
	}
	if err := os.Rename(fresh, program); err != nil {
		if back := os.Rename(old, program); back != nil {
			return fmt.Errorf("%w: %v, and the program could not get its name back: it is %s", ErrNoWrite, err, old)
		}
		return fmt.Errorf("%w: %v", ErrNoWrite, err)
	}
	return nil
}

// CleanOld deletes what an update left beside the program: the program it replaced, once it has
// stopped (Windows keeps a running program from being deleted), and downloads that did not end,
// an hour old at least, so not one going on.
func CleanOld(program string) {
	folder := filepath.Dir(program)
	leftovers, _ := filepath.Glob(filepath.Join(folder, filepath.Base(program)+"*.old"))
	for _, path := range leftovers {
		os.Remove(path)
	}
	parts, _ := filepath.Glob(filepath.Join(folder, ".nxpress-update-*"))
	for _, path := range parts {
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > time.Hour {
			os.Remove(path)
		}
	}
}

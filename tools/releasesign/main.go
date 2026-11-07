// Command releasesign signs a release of NXPress on the computer of the maintainer (#91), so the
// key NXPress trusts is never on GitHub: a stolen token or a workflow that runs other code can
// make a release, but not one NXPress installs by itself. The build of a version tag makes its
// release as a draft, with the programs, SHA256SUMS.txt and their provenance. Then, from the
// folder of NXPress, with gh signed in and the tag in it:
//
//	go run ./tools/releasesign 1.7.4
//
// It downloads the files of the draft, checks each against SHA256SUMS.txt, and that each was
// built by the build workflow from the commit of the tag of this repository (gh attestation
// verify), which is the commit of the tag on GitHub too. Then it asks for the private key, which
// is not shown as it is typed or pasted, signs SHA256SUMS.txt for that version
// (SHA256SUMS.txt.minisig, as minisign does, with the version in its trusted comment), checks the
// signature with the public keys NXPress is built with, adds it to the draft and, once told so,
// publishes the release. The key is never printed, written nor kept. The library app signs its
// releases the same way.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/selfupdate"
)

const (
	defaultRepo     = "DeLFuS77/NXPress"
	defaultWorkflow = ".github/workflows/build.yml"
	sumsName        = "SHA256SUMS.txt"
	signatureName   = sumsName + ".minisig"
)

var versionTag = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)

// tools are the programs and the questions releasesign uses; variables, for the tests.
type tools struct {
	// gh runs gh with the arguments and gives what it printed
	gh func(args ...string) ([]byte, error)
	// git runs git in the folder of NXPress
	git func(args ...string) ([]byte, error)
	// readKey asks for the private key, not shown as it is typed
	readKey func(prompt string) (string, error)
	// confirm asks a question to answer yes or no
	confirm func(prompt string) bool
	out     io.Writer
}

type options struct {
	repo, workflow, version string
	// publish the release once signed without asking; sign a release published already; put a
	// new signature in place of the one the release has
	publish, published, replace bool
}

func main() {
	o := options{}
	flag.StringVar(&o.repo, "repo", defaultRepo, "the repository of the release")
	flag.StringVar(&o.workflow, "workflow", defaultWorkflow, "the workflow that builds the releases")
	flag.BoolVar(&o.publish, "publish", false, "publish the release once it is signed, without asking")
	flag.BoolVar(&o.published, "published", false, "sign a release that is published already")
	flag.BoolVar(&o.replace, "replace", false, "replace the signature the release has")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: releasesign [-publish] 1.2.3")
		os.Exit(2)
	}
	o.version = flag.Arg(0)
	t := tools{gh: command("gh"), git: command("git"), readKey: readHidden, confirm: confirm, out: os.Stdout}
	if err := run(t, o, selfupdate.Keys()); err != nil {
		fmt.Fprintln(os.Stderr, "releasesign:", err)
		os.Exit(1)
	}
}

// command runs a program and gives what it printed; what it says on errors goes to the screen.
func command(name string) func(args ...string) ([]byte, error) {
	return func(args ...string) ([]byte, error) {
		cmd := exec.Command(name, args...)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			return out.Bytes(), fmt.Errorf("%s %s: %w", name, args[0], err)
		}
		return out.Bytes(), nil
	}
}

// stdin is what is typed, for the key and the questions alike
var stdin = bufio.NewReader(os.Stdin)

func confirm(prompt string) bool {
	fmt.Print(prompt)
	answer, _ := stdin.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

type release struct {
	IsDraft bool `json:"isDraft"`
	Assets  []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

func run(t tools, o options, keys []selfupdate.PublicKey) error {
	if !versionTag.MatchString(o.version) {
		return fmt.Errorf("%q is not the tag of a version (1.2.3)", o.version)
	}
	if len(keys) == 0 {
		return errors.New("NXPress has no public key (internal/selfupdate/release.pub): make one with tools/releasekey")
	}

	// the commit of the tag here, which the tag on GitHub and the provenance must have
	tag := "refs/tags/" + o.version
	local, err := t.git("rev-parse", "--verify", "--quiet", tag+"^{commit}")
	if err != nil {
		return fmt.Errorf("the tag %s is not in this repository: sign from the folder the tag was made in", o.version)
	}
	commit := strings.TrimSpace(string(local))
	remote, err := t.git("ls-remote", "--tags", "https://github.com/"+o.repo+".git", tag, tag+"^{}")
	if err != nil {
		return err
	}
	if remoteCommit := peeled(string(remote), tag); remoteCommit != commit {
		return fmt.Errorf("the tag %s on GitHub is not the one of this repository (%s, not %s)", o.version, orNone(remoteCommit), commit)
	}

	view, err := t.gh("release", "view", o.version, "-R", o.repo, "--json", "isDraft,assets")
	if err != nil {
		return fmt.Errorf("the release %s cannot be read: %w", o.version, err)
	}
	var r release
	if err := json.Unmarshal(view, &r); err != nil {
		return fmt.Errorf("the release %s cannot be read: %w", o.version, err)
	}
	if !r.IsDraft && !o.published {
		return fmt.Errorf("the release %s is published already: sign it as a draft, or give -published", o.version)
	}
	for _, asset := range r.Assets {
		if asset.Name == signatureName && !o.replace {
			return fmt.Errorf("the release %s has a signature already: give -replace to sign it again", o.version)
		}
	}

	folder, err := os.MkdirTemp("", "releasesign-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(folder)
	if _, err := t.gh("release", "download", o.version, "-R", o.repo, "-D", folder); err != nil {
		return fmt.Errorf("the files of the release %s cannot be downloaded: %w", o.version, err)
	}
	sums, err := os.ReadFile(filepath.Join(folder, sumsName))
	if err != nil {
		return fmt.Errorf("the release %s has no %s", o.version, sumsName)
	}
	programs, err := checkSums(folder, sums, r)
	if err != nil {
		return err
	}
	for _, name := range programs {
		_, err := t.gh("attestation", "verify", filepath.Join(folder, name), "-R", o.repo,
			"--signer-workflow", o.repo+"/"+o.workflow, "--source-ref", tag, "--source-digest", commit,
			"--deny-self-hosted-runners")
		if err != nil {
			return fmt.Errorf("%s was not built by %s from the commit of the tag %s: nothing is signed", name, o.workflow, o.version)
		}
	}
	fmt.Fprintf(t.out, "The %d files of %s match %s and were built by %s from %s, the commit of the tag.\n",
		len(programs), o.version, sumsName, o.workflow, commit)

	text, err := t.readKey("The private key of the releases (it is not shown): ")
	if err != nil {
		return err
	}
	private, id, err := selfupdate.ParsePrivateKey(text)
	if err != nil {
		return errors.New("that is not a private key of NXPress: nothing is signed")
	}
	signature := selfupdate.Sign(private, id, sums, "NXPress "+o.version)
	clear(private)
	if _, err := selfupdate.Verify(sums, signature, keys); err != nil {
		return errors.New("that key is not the one NXPress is built with (internal/selfupdate/release.pub): nothing is signed")
	}
	path := filepath.Join(folder, signatureName)
	if err := os.WriteFile(path, signature, 0o644); err != nil {
		return err
	}
	upload := []string{"release", "upload", o.version, path, "-R", o.repo}
	if o.replace {
		upload = append(upload, "--clobber")
	}
	if _, err := t.gh(upload...); err != nil {
		return fmt.Errorf("the signature could not be added to the release: %w", err)
	}
	fmt.Fprintf(t.out, "%s is signed for version %s: %s is in the release.\n", sumsName, o.version, signatureName)

	if !r.IsDraft {
		return nil
	}
	if !o.publish && !t.confirm(fmt.Sprintf("Publish the release %s now? [y/N] ", o.version)) {
		fmt.Fprintf(t.out, "It stays a draft: publish it on GitHub, or with gh release edit %s -R %s --draft=false\n", o.version, o.repo)
		return nil
	}
	if _, err := t.gh("release", "edit", o.version, "-R", o.repo, "--draft=false"); err != nil {
		return fmt.Errorf("the release could not be published: %w", err)
	}
	fmt.Fprintf(t.out, "The release %s is published: https://github.com/%s/releases/tag/%s\n", o.version, o.repo, o.version)
	return nil
}

// peeled is the commit a tag of git ls-remote points to: the line of the tag with ^{} for an
// annotated tag, else the line of the tag.
func peeled(listing string, tag string) string {
	commit := ""
	for _, line := range strings.Split(strings.TrimSpace(listing), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch fields[1] {
		case tag + "^{}":
			return fields[0]
		case tag:
			commit = fields[0]
		}
	}
	return commit
}

func orNone(commit string) string {
	if commit == "" {
		return "none"
	}
	return commit
}

// checkSums checks that each file of the release, but the checksums, their signature and the
// provenance, is in the checksums, that each line of them is a file of the release, and that each
// file downloaded matches its line. It gives the names of the files.
func checkSums(folder string, sums []byte, r release) ([]string, error) {
	listed := map[string]string{}
	order := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(sums)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			return nil, fmt.Errorf("%s has a line that is not a checksum: %q", sumsName, line)
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
			return nil, fmt.Errorf("%s names a file in a folder: %q", sumsName, name)
		}
		if _, twice := listed[name]; twice {
			return nil, fmt.Errorf("%s names %s twice", sumsName, name)
		}
		listed[name] = strings.ToLower(fields[0])
		order = append(order, name)
	}
	assets := map[string]bool{}
	for _, asset := range r.Assets {
		assets[asset.Name] = true
		if asset.Name == sumsName || asset.Name == signatureName || strings.HasSuffix(asset.Name, ".intoto.jsonl") {
			continue
		}
		if _, ok := listed[asset.Name]; !ok {
			return nil, fmt.Errorf("%s is in the release but not in %s", asset.Name, sumsName)
		}
	}
	for _, name := range order {
		if !assets[name] {
			return nil, fmt.Errorf("%s names %s, which the release does not have", sumsName, name)
		}
		file, err := os.Open(filepath.Join(folder, name))
		if err != nil {
			return nil, fmt.Errorf("%s was not downloaded: %w", name, err)
		}
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		file.Close()
		if err != nil {
			return nil, err
		}
		if hex.EncodeToString(hash.Sum(nil)) != listed[name] {
			return nil, fmt.Errorf("%s does not match its checksum in %s: nothing is signed", name, sumsName)
		}
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("%s is empty", sumsName)
	}
	return order, nil
}

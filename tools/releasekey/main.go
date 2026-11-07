// Command releasekey makes the key the releases of NXPress are signed with (#91). The maintainer
// runs it once, on their own computer:
//
//	go run ./tools/releasekey -backup FILE
//
// The private key is written to FILE only, outside the repository, for the maintainer to keep in
// a password manager and then delete FILE: tools/releasesign asks for it to sign each release on
// that computer, and it is never on GitHub nor on the screen. The public key is written to
// internal/selfupdate/release.pub, which NXPress is built with (commit it). Once the private key
// is lost, a new one is made, and the copies of NXPress built with the old public key only tell
// of the next versions.
//
// With -add, a spare key is made beside the one NXPress has: release.pub keeps both, and NXPress
// installs a release signed with either. Its private key is kept apart from the first (printed,
// or on a USB stick in a drawer): if the first is lost, the releases are signed with the spare one
// and every copy of NXPress still updates by itself.
//
//	go run ./tools/releasekey -add -backup FILE
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/selfupdate"
)

func main() {
	backup := flag.String("backup", "", "the file the private key is written to, outside the repository")
	force := flag.Bool("force", false, "make a new key though NXPress has one, in place of it")
	add := flag.Bool("add", false, "make a spare key beside the one NXPress has")
	flag.Parse()
	if err := run(*backup, *force, *add); err != nil {
		fmt.Fprintln(os.Stderr, "releasekey:", err)
		os.Exit(1)
	}
}

// checkBackup tells whether the backup can be written where it is asked, before anything is
// made: outside the repository, in a folder that is there and takes files, and not over a file.
func checkBackup(backup string, repository string) error {
	whole, err := filepath.Abs(backup)
	if err != nil {
		return err
	}
	if strings.HasPrefix(strings.ToLower(whole), strings.ToLower(repository)+string(filepath.Separator)) {
		return fmt.Errorf("keep the backup outside the repository, not in %s", backup)
	}
	folder := filepath.Dir(whole)
	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		return fmt.Errorf("the folder of the backup is not there: %s", folder)
	}
	if _, err := os.Lstat(whole); err == nil {
		return fmt.Errorf("%s is there already: give another name", backup)
	}
	probe, err := os.CreateTemp(folder, ".releasekey-*")
	if err != nil {
		return fmt.Errorf("the backup cannot be written in %s: %w", folder, err)
	}
	probe.Close()
	os.Remove(probe.Name())
	return nil
}

func run(backup string, force bool, add bool) error {
	keysFile := filepath.Join("internal", "selfupdate", "release.pub")
	current, err := os.ReadFile(keysFile)
	if err != nil {
		return fmt.Errorf("run it from the folder of NXPress: %w", err)
	}
	keys, err := selfupdate.ParsePublicKeys(string(current))
	switch {
	case add && force:
		return errors.New("give -add for a spare key, or -force for a new key in place of the one there: not both")
	case add && (err != nil || len(keys) == 0):
		return fmt.Errorf("NXPress has no key yet (%s): make the first one without -add", keysFile)
	case !add && len(keys) > 0 && !force:
		return fmt.Errorf("NXPress has a key already (%s): give -add to make a spare one beside it; a new one in its place (-force) leaves the copies built with it telling of new versions only", keysFile)
	}
	if backup == "" {
		return errors.New("give -backup FILE: the private key is written there, and nowhere else")
	}
	here, _ := filepath.Abs(".")
	if err := checkBackup(backup, here); err != nil {
		return err
	}

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	// an ID of its own: NXPress tells the keys apart by it
	var id [8]byte
	for taken := true; taken; {
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		taken = false
		for _, key := range keys {
			taken = taken || key.ID == id
		}
	}
	if err := os.WriteFile(backup, []byte(selfupdate.PrivateKeyText(private, id)+"\n"), 0o600); err != nil {
		return fmt.Errorf("the private key could not be written, nothing was changed: %w", err)
	}
	clear(private)
	key := selfupdate.PublicKey{ID: id, Key: public}
	text := "# The public keys the releases of NXPress are signed with (minisign), written by\n# tools/releasekey. With none, NXPress only tells of a new version.\n" + key.Text()
	if add {
		text = strings.TrimRight(string(current), "\r\n") + "\n# a spare key, kept apart from the first: it signs the releases if the first is lost\n" + key.Text()
	}
	if err := os.WriteFile(keysFile, []byte(text), 0o644); err != nil {
		os.Remove(backup)
		return fmt.Errorf("%s could not be written, nothing was changed: %w", keysFile, err)
	}
	if add {
		fmt.Printf("The spare private key is in %s: keep it apart from the first one (printed, or on a USB stick in a drawer), then delete the file.\n", backup)
		fmt.Println("tools/releasesign takes either key; neither is ever put on GitHub.")
		fmt.Printf("NXPress now trusts both keys: commit %s before the next release.\n", keysFile)
		return nil
	}
	fmt.Printf("The private key is in %s: keep it in your password manager, then delete the file.\n", backup)
	fmt.Println("tools/releasesign asks for it to sign each release; it is never put on GitHub.")
	fmt.Printf("The public key is in %s: commit it.\n", keysFile)
	return nil
}

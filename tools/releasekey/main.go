// Command releasekey makes the key the releases of NXPress are signed with (#91). The maintainer
// runs it once, on their own computer, with gh signed in:
//
//	go run ./tools/releasekey
//
// The private key goes straight into the secret NXPRESS_SIGNING_KEY of the repository, through
// gh, and is never shown nor kept; only the public key is written, to
// internal/selfupdate/release.pub, which NXPress is built with (commit it). With -backup FILE the
// private key is written to FILE too, outside the repository, for the maintainer to keep somewhere
// safe: with it a lost secret can be set again; without it, a new key is made, and the copies of
// NXPress built with the old public key only tell of the next versions.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DeLFuS77/NXPress/internal/selfupdate"
)

func main() {
	repo := flag.String("repo", "DeLFuS77/NXPress", "the repository whose secret takes the key")
	backup := flag.String("backup", "", "also write the private key to this file, outside the repository")
	force := flag.Bool("force", false, "make a new key though NXPress has one")
	flag.Parse()
	if err := run(*repo, *backup, *force); err != nil {
		fmt.Fprintln(os.Stderr, "releasekey:", err)
		os.Exit(1)
	}
}

func run(repo, backup string, force bool) error {
	keysFile := filepath.Join("internal", "selfupdate", "release.pub")
	current, err := os.ReadFile(keysFile)
	if err != nil {
		return fmt.Errorf("run it from the folder of NXPress: %w", err)
	}
	if keys, _ := selfupdate.ParsePublicKeys(string(current)); len(keys) > 0 && !force {
		return fmt.Errorf("NXPress has a key already (%s): a new one leaves the copies built with it telling of new versions only; give -force to make one all the same", keysFile)
	}
	if backup != "" {
		whole, _ := filepath.Abs(backup)
		here, _ := filepath.Abs(".")
		if strings.HasPrefix(strings.ToLower(whole), strings.ToLower(here)+string(filepath.Separator)) {
			return fmt.Errorf("keep the backup outside the repository, not in %s", backup)
		}
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh is needed to put the key in the secret: %w", err)
	}

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	secret := selfupdate.PrivateKeyText(private, id)

	// the private key goes to gh by its input, never on the command line nor on the screen
	command := exec.Command("gh", "secret", "set", "NXPRESS_SIGNING_KEY", "--repo", repo)
	command.Stdin = strings.NewReader(secret)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("the secret could not be set, nothing was changed: %w", err)
	}
	if backup != "" {
		if err := os.WriteFile(backup, []byte(secret+"\n"), 0o600); err != nil {
			return fmt.Errorf("the secret is set, but the backup could not be written: %w", err)
		}
	}
	key := selfupdate.PublicKey{ID: id, Key: public}
	header := "# The public keys the releases of NXPress are signed with (minisign), written by\n# tools/releasekey. With none, NXPress only tells of a new version.\n"
	if err := os.WriteFile(keysFile, []byte(header+key.Text()), 0o644); err != nil {
		return fmt.Errorf("the secret is set, but %s could not be written: %w", keysFile, err)
	}
	fmt.Printf("The private key is in the secret NXPRESS_SIGNING_KEY of %s.\n", repo)
	if backup != "" {
		fmt.Printf("Its backup is %s: keep it somewhere safe, and never in the repository.\n", backup)
	}
	fmt.Printf("The public key is in %s: commit it, and the next release is signed.\n", keysFile)
	return nil
}

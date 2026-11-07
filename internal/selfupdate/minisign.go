// Package selfupdate installs a new version of NXPress from the window, when asked (#91): only a
// version whose checksums carry the signature of the project, the file checked against them,
// and NXPress put in place of itself without ever leaving it half replaced.
package selfupdate

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// The signatures are the ones of minisign (https://jedisct1.github.io/minisign/), so anyone can
// check a release with its public key and that tool too: Ed25519, over the BLAKE2b-512 of the
// file ("ED"), with a trusted comment signed as well, which tells the version.

// PublicKey is a public key of the project: its ID and its Ed25519 key.
type PublicKey struct {
	ID  [8]byte
	Key ed25519.PublicKey
}

// ErrSignature is a signature that is not the one of the project over the file.
var ErrSignature = errors.New("the signature is not the one of NXPress")

// ParsePublicKeys reads the public keys of a file of minisign keys, one or more, each the line
// in base64 after its comment; lines that are no key are left out.
func ParsePublicKeys(text string) ([]PublicKey, error) {
	keys := []PublicKey{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "untrusted comment:") || strings.HasPrefix(line, "#") {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(line)
		if err != nil || len(data) != 2+8+ed25519.PublicKeySize || string(data[:2]) != "Ed" {
			return nil, fmt.Errorf("not a minisign public key: %q", line)
		}
		key := PublicKey{Key: ed25519.PublicKey(append([]byte{}, data[10:]...))}
		copy(key.ID[:], data[2:10])
		keys = append(keys, key)
	}
	return keys, nil
}

// Text is the key as minisign writes it, with its comment.
func (k PublicKey) Text() string {
	data := append(append([]byte("Ed"), k.ID[:]...), k.Key...)
	return fmt.Sprintf("untrusted comment: minisign public key %X\n%s\n", reverse(k.ID), base64.StdEncoding.EncodeToString(data))
}

// reverse is the ID as minisign shows it: as a number, written in little-endian.
func reverse(id [8]byte) []byte {
	out := make([]byte, 8)
	for i := range id {
		out[7-i] = id[i]
	}
	return out
}

// signature is a signature of minisign: its algorithm, the ID of its key, the signature of the
// file, the trusted comment and the signature of both.
type signature struct {
	algorithm string
	keyID     [8]byte
	file      []byte
	comment   string
	global    []byte
}

func parseSignature(data []byte) (signature, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) < 4 || !strings.HasPrefix(lines[0], "untrusted comment:") || !strings.HasPrefix(lines[2], "trusted comment: ") {
		return signature{}, ErrSignature
	}
	head, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil || len(head) != 2+8+ed25519.SignatureSize {
		return signature{}, ErrSignature
	}
	global, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[3]))
	if err != nil || len(global) != ed25519.SignatureSize {
		return signature{}, ErrSignature
	}
	s := signature{algorithm: string(head[:2]), file: head[10:], comment: strings.TrimPrefix(lines[2], "trusted comment: "), global: global}
	copy(s.keyID[:], head[2:10])
	return s, nil
}

// Verify checks a signature of minisign over data with the keys given, and gives its trusted
// comment: only a signature of one of the keys, over these bytes, with its comment unchanged.
func Verify(data []byte, sig []byte, keys []PublicKey) (string, error) {
	s, err := parseSignature(sig)
	if err != nil {
		return "", err
	}
	signed := data
	switch s.algorithm {
	case "ED":
		sum := blake2b.Sum512(data)
		signed = sum[:]
	case "Ed":
	default:
		return "", ErrSignature
	}
	for _, key := range keys {
		if key.ID != s.keyID {
			continue
		}
		if !ed25519.Verify(key.Key, signed, s.file) || !ed25519.Verify(key.Key, append(append([]byte{}, s.file...), s.comment...), s.global) {
			return "", ErrSignature
		}
		return s.comment, nil
	}
	return "", ErrSignature
}

// Sign signs data as minisign does ("ED"), with a trusted comment: what the release signs its
// checksums with, and the tests.
func Sign(private ed25519.PrivateKey, keyID [8]byte, data []byte, comment string) []byte {
	sum := blake2b.Sum512(data)
	file := ed25519.Sign(private, sum[:])
	global := ed25519.Sign(private, append(append([]byte{}, file...), comment...))
	head := append(append([]byte("ED"), keyID[:]...), file...)
	var out bytes.Buffer
	fmt.Fprintf(&out, "untrusted comment: signature from the key of NXPress\n%s\ntrusted comment: %s\n%s\n",
		base64.StdEncoding.EncodeToString(head), comment, base64.StdEncoding.EncodeToString(global))
	return out.Bytes()
}

// The private key the releases are signed with, which the maintainer keeps in a password manager
// and gives to tools/releasesign on their own computer: its ID and the seed of its Ed25519 key,
// in base64. It never goes on GitHub, in a file of NXPress, nor in its log.

// ParsePrivateKey reads the private key of the releases.
func ParsePrivateKey(text string) (ed25519.PrivateKey, [8]byte, error) {
	var id [8]byte
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(data) != 8+ed25519.SeedSize {
		return nil, id, errors.New("the signing key is not one of NXPress")
	}
	copy(id[:], data[:8])
	return ed25519.NewKeyFromSeed(data[8:]), id, nil
}

// PrivateKeyText is the private key as the maintainer keeps it.
func PrivateKeyText(private ed25519.PrivateKey, id [8]byte) string {
	return base64.StdEncoding.EncodeToString(append(append([]byte{}, id[:]...), private.Seed()...))
}

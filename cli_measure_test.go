package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/settings"
	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// keys made up for the tests, never the real ones
const (
	measureTestHeaderKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	measureTestAreaKey   = "1f1e1d1c1b1a19181716151413121110"
	measureTestNcaKey    = "00112233445566778899aabbccddeeff"
)

// measureTestKeys makes the made-up keys active.
func measureTestKeys(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prod.keys")
	keys := "header_key = " + measureTestHeaderKey + "\nkey_area_key_application_00 = " + measureTestAreaKey + "\nkey_area_key_application_02 = " + measureTestAreaKey + "\n"
	if err := os.WriteFile(path, []byte(keys), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := settings.UseSwitchKeys(path); err != nil {
		t.Fatal(err)
	}
}

// measureTestHeader encrypts the header of an NCA with the made-up header key (AES-XTS, by
// sectors of 0x200 bytes).
func measureTestHeader(plain []byte) []byte {
	key, _ := hex.DecodeString(measureTestHeaderKey)
	data, _ := aes.NewCipher(key[:16])
	tweakCipher, _ := aes.NewCipher(key[16:])
	result := make([]byte, len(plain))
	for sector := 0; sector < len(plain)/0x200; sector++ {
		tweak := make([]byte, 16)
		binary.BigEndian.PutUint64(tweak[8:], uint64(sector))
		tweakCipher.Encrypt(tweak, tweak)
		for at := sector * 0x200; at < (sector+1)*0x200; at += 16 {
			block := make([]byte, 16)
			for i := range block {
				block[i] = plain[at+i] ^ tweak[i]
			}
			data.Encrypt(block, block)
			for i := range block {
				result[at+i] = block[i] ^ tweak[i]
			}
			// the tweak times two in GF(2^128)
			carry := byte(0)
			for i := range tweak {
				next := tweak[i] >> 7
				tweak[i] = tweak[i]<<1 | carry
				carry = next
			}
			if carry != 0 {
				tweak[0] ^= 0x87
			}
		}
	}
	return result
}

// measureTestGame writes an NSP holding a program NCA of size bytes of game data; sparse makes
// its section one in sparse storage, which no compressor reads.
func measureTestGame(t *testing.T, path string, size int, sparse bool) string {
	t.Helper()
	text := "Synthetic game data, the same words again and again. "
	plain := make([]byte, size)
	for i := range plain {
		plain[i] = text[i%len(text)] ^ byte(i/4096)
	}
	areaKey, _ := hex.DecodeString(measureTestAreaKey)
	contentKey, _ := hex.DecodeString(measureTestNcaKey)
	header := make([]byte, 0xC00)
	copy(header[0x200:], "NCA3")
	header[0x205] = switchfs.NcaContentType_Program
	binary.LittleEndian.PutUint32(header[0x240:], 0x20)
	binary.LittleEndian.PutUint32(header[0x244:], uint32(0x20+len(plain)/0x200))
	header[0x402], header[0x403], header[0x404] = 1, 2, 3
	binary.LittleEndian.PutUint32(header[0x540:], 1)
	binary.LittleEndian.PutUint32(header[0x544:], 0x55667788)
	if sparse {
		// the size of the table of a sparse section
		header[0x400+0x150] = 0x10
	}
	fsHash := sha256.Sum256(header[0x400:0x600])
	copy(header[0x280:], fsHash[:])
	areaCipher, _ := aes.NewCipher(areaKey)
	areaCipher.Encrypt(header[0x320:0x330], contentKey)
	nca := make([]byte, 0x4000)
	copy(nca, measureTestHeader(header))
	section := append([]byte(nil), plain...)
	block, _ := aes.NewCipher(contentKey)
	counter := []byte{0x55, 0x66, 0x77, 0x88, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint64(counter[8:], 0x4000>>4)
	cipher.NewCTR(block, counter).XORKeyStream(section, section)
	nca = append(nca, section...)

	// a PFS0 with the NCA alone
	name := "0123456789abcdef0123456789abcdef.nca\x00"
	pfs0 := make([]byte, 0x10+0x18)
	copy(pfs0, "PFS0")
	binary.LittleEndian.PutUint32(pfs0[4:], 1)
	binary.LittleEndian.PutUint32(pfs0[8:], uint32(len(name)))
	binary.LittleEndian.PutUint64(pfs0[0x18:], uint64(len(nca)))
	pfs0 = append(append(pfs0, name...), nca...)
	if err := os.WriteFile(path, pfs0, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The measure tries its settings on the biggest game of the list it can compress, not the
// first: a small one is read whole in a trial, which then measures the start of a compression
// more than the compression.
func TestMeasureTakesTheBiggestGame(t *testing.T) {
	measureTestKeys(t)
	folder := t.TempDir()
	small := measureTestGame(t, filepath.Join(folder, "small.nsp"), 0x10000, false)
	big := measureTestGame(t, filepath.Join(folder, "big.nsp"), 0x40000, false)
	sparse := measureTestGame(t, filepath.Join(folder, "sparse.nsp"), 0x80000, true)
	o := options{level: 3}
	if got, err := measurableGame(context.Background(), []string{small, sparse, big}, o); err != nil || got != big {
		t.Fatalf("the biggest that can be compressed: %s %v", filepath.Base(got), err)
	}
	if got, err := measurableGame(context.Background(), []string{sparse, small}, o); err != nil || got != small {
		t.Fatalf("one that can be compressed: %s %v", filepath.Base(got), err)
	}
}

// A game in sparse storage is left out of the measure with that reason, not a vague one.
func TestMeasureTellsWhyAGameIsLeftOut(t *testing.T) {
	measureTestKeys(t)
	sparse := measureTestGame(t, filepath.Join(t.TempDir(), "sparse.nsp"), 0x10000, true)
	_, err := measurableGame(context.Background(), []string{sparse}, options{level: 3})
	want := "nothing in it can be compressed, it cannot be measured with: it uses sparse storage, which cannot be compressed yet"
	if !isSkipped(err) || err.Error() != want {
		t.Fatalf("%v", err)
	}
	if !strings.HasPrefix(notMeasurable(sparse).Error(), want) {
		t.Fatalf("%v", notMeasurable(sparse))
	}
}

package main

// Synthetic games for the tests of the safety of the files, made with keys of their own:
// never the keys of the user.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/settings"
	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

const (
	safetyHeaderKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	safetyAreaKey   = "1f1e1d1c1b1a19181716151413121110"
	safetyNcaKey    = "00112233445566778899aabbccddeeff"
)

// safetyIsolate keeps a test away from the cache of the files checked and from the keys of the
// user: the cache starts empty in a folder of the test, and the keys are made up.
func safetyIsolate(t *testing.T) {
	t.Helper()
	config := t.TempDir()
	t.Setenv("AppData", config)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("HOME", config)
	before := checked
	checked = &checkCache{}
	t.Cleanup(func() { checked = before })
	keys := "header_key = " + safetyHeaderKey + "\nkey_area_key_application_00 = " + safetyAreaKey + "\nkey_area_key_application_02 = " + safetyAreaKey + "\n"
	path := filepath.Join(t.TempDir(), "prod.keys")
	if err := os.WriteFile(path, []byte(keys), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := settings.UseSwitchKeys(path); err != nil {
		t.Fatal(err)
	}
}

func safetyTweak(sector int) [16]byte {
	tweak := [16]byte{}
	for i := 0xF; i >= 0; i-- {
		tweak[i] = uint8(sector & 0xFF)
		sector >>= 8
	}
	return tweak
}

func safetyNextTweak(tweak []byte) {
	var carry byte
	for i := range tweak {
		next := tweak[i] >> 7
		tweak[i] = tweak[i]<<1 | carry
		carry = next
	}
	if carry != 0 {
		tweak[0] ^= 0x87
	}
}

// safetyEncryptHeader encrypts an NCA header as Nintendo does, with AES-XTS and the header key.
func safetyEncryptHeader(plain []byte) []byte {
	key, _ := hex.DecodeString(safetyHeaderKey)
	first, _ := aes.NewCipher(key[:len(key)/2])
	second, _ := aes.NewCipher(key[len(key)/2:])
	result := make([]byte, len(plain))
	for sector := 0; sector < len(plain)/0x200; sector++ {
		start := sector * 0x200
		tweak := safetyTweak(sector)
		encrypted := make([]byte, aes.BlockSize)
		second.Encrypt(encrypted, tweak[:])
		for offset := 0; offset < 0x200; offset += aes.BlockSize {
			block := make([]byte, aes.BlockSize)
			for i := range block {
				block[i] = plain[start+offset+i] ^ encrypted[i]
			}
			first.Encrypt(block, block)
			for i := range block {
				result[start+offset+i] = block[i] ^ encrypted[i]
			}
			safetyNextTweak(encrypted)
		}
	}
	return result
}

func safetyGameData(size int, seed int) []byte {
	data := make([]byte, size)
	text := []byte(fmt.Sprintf("Synthetic game data %d; the same words again and again. ", seed))
	for i := range data {
		data[i] = text[i%len(text)] ^ byte(i/4096)
	}
	return data
}

func safetyProgramNca(plain []byte) []byte {
	return safetyProgramNcaWith(plain, nil)
}

// safetyProgramNcaWith is safetyProgramNca with its plain header changed by change first.
func safetyProgramNcaWith(plain []byte, change func(header []byte)) []byte {
	areaKey, _ := hex.DecodeString(safetyAreaKey)
	contentKey, _ := hex.DecodeString(safetyNcaKey)
	header := make([]byte, 0xC00)
	copy(header[0x200:], "NCA3")
	header[0x205] = switchfs.NcaContentType_Program
	binary.LittleEndian.PutUint32(header[0x240:], 0x20)
	binary.LittleEndian.PutUint32(header[0x244:], uint32(0x20+len(plain)/0x200))
	header[0x402] = 1
	header[0x403] = 2
	header[0x404] = 3
	binary.LittleEndian.PutUint32(header[0x540:], 1)
	binary.LittleEndian.PutUint32(header[0x544:], 0x55667788)
	fsHash := sha256.Sum256(header[0x400:0x600])
	copy(header[0x280:], fsHash[:])
	areaCipher, _ := aes.NewCipher(areaKey)
	areaCipher.Encrypt(header[0x320:0x330], contentKey)
	if change != nil {
		change(header)
	}
	nca := make([]byte, 0x4000)
	copy(nca, safetyEncryptHeader(header))
	section := append([]byte(nil), plain...)
	block, _ := aes.NewCipher(contentKey)
	counter := []byte{0x55, 0x66, 0x77, 0x88, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint64(counter[8:], 0x4000>>4)
	cipher.NewCTR(block, counter).XORKeyStream(section, section)
	return append(nca, section...)
}

// safetyRewrittenNca is a program NCA as a tool leaves it: its header changed after it was
// named, so it no longer matches its content ID. Its data is checked by the hash tree of its
// second section; its first has none, and cannot be checked on its own: the check of a file
// that holds it is in part. It gives the NCA and its name, the content ID of the original.
func safetyRewrittenNca() ([]byte, string) {
	plain := safetyGameData(0x10000, 7)
	tree := make([]byte, 0x400)
	copy(tree[0x200:], safetyGameData(0x200, 8))
	block := sha256.Sum256(tree[0x200:])
	copy(tree, block[:])
	master := sha256.Sum256(tree[:0x20])
	build := func(rewritten bool) []byte {
		nca := safetyProgramNcaWith(plain, func(header []byte) {
			start := uint32(0x20 + len(plain)/0x200)
			binary.LittleEndian.PutUint32(header[0x250:], start)
			binary.LittleEndian.PutUint32(header[0x254:], start+2)
			header[0x602] = 1
			header[0x603] = 2
			header[0x604] = 1
			copy(header[0x608:], master[:])
			binary.LittleEndian.PutUint32(header[0x628:], 0x200)
			binary.LittleEndian.PutUint32(header[0x62C:], 2)
			binary.LittleEndian.PutUint64(header[0x638:], 0x20)
			binary.LittleEndian.PutUint64(header[0x640:], 0x200)
			binary.LittleEndian.PutUint64(header[0x648:], 0x200)
			fsHash := sha256.Sum256(header[0x600:0x800])
			copy(header[0x2A0:], fsHash[:])
			if rewritten {
				header[0x21C] ^= 0x5A
			}
		})
		return append(nca, tree...)
	}
	return build(true), safetyContentName(build(false), ".nca")
}

func safetyMetaNca(section []byte) []byte {
	contentKey, _ := hex.DecodeString(safetyNcaKey)
	areaKey, _ := hex.DecodeString(safetyAreaKey)
	areaCipher, _ := aes.NewCipher(areaKey)
	header := make([]byte, 0xc00)
	copy(header[0x200:], []byte("NCA3"))
	binary.LittleEndian.PutUint32(header[0x240:], 6)
	binary.LittleEndian.PutUint32(header[0x244:], uint32(6+len(section)/0x200))
	header[0x402] = 1
	header[0x403] = 2
	header[0x404] = 3
	binary.LittleEndian.PutUint32(header[0x540:], 1)
	binary.LittleEndian.PutUint64(header[0x440:], 0)
	binary.LittleEndian.PutUint64(header[0x448:], uint64(len(section)))
	fsHash := sha256.Sum256(header[0x400:0x600])
	copy(header[0x280:], fsHash[:])
	areaCipher.Encrypt(header[0x320:0x330], contentKey)
	block, _ := aes.NewCipher(contentKey)
	counter := make([]byte, 0x10)
	binary.BigEndian.PutUint64(counter, 1)
	binary.BigEndian.PutUint64(counter[8:], uint64(0xc00/0x10))
	encoded := make([]byte, len(section))
	cipher.NewCTR(block, counter).XORKeyStream(encoded, section)
	return append(safetyEncryptHeader(header), encoded...)
}

func safetyPFS0(names []string, payloads [][]byte) []byte {
	var table []byte
	offsets := make([]uint32, len(names))
	for i, name := range names {
		offsets[i] = uint32(len(table))
		table = append(table, []byte(name)...)
		table = append(table, 0)
	}
	headerLen := 0x10 + switchfs.PfsfileEntryTableSize*len(names) + len(table)
	result := make([]byte, headerLen)
	copy(result, "PFS0")
	binary.LittleEndian.PutUint32(result[4:8], uint32(len(names)))
	binary.LittleEndian.PutUint32(result[8:12], uint32(len(table)))
	offset := 0
	for i, payload := range payloads {
		entry := result[0x10+i*switchfs.PfsfileEntryTableSize:]
		binary.LittleEndian.PutUint64(entry[0:8], uint64(offset))
		binary.LittleEndian.PutUint64(entry[8:16], uint64(len(payload)))
		binary.LittleEndian.PutUint32(entry[16:20], offsets[i])
		offset += len(payload)
	}
	copy(result[0x10+switchfs.PfsfileEntryTableSize*len(names):], table)
	for _, payload := range payloads {
		result = append(result, payload...)
	}
	return result
}

func safetyPad(data []byte) []byte {
	return append(data, make([]byte, (0x200-len(data)%0x200)%0x200)...)
}

func safetyContentName(nca []byte, suffix string) string {
	sum := sha256.Sum256(nca)
	return hex.EncodeToString(sum[:16]) + suffix
}

// safetyTitleNsp writes an NSP of one title, its Meta NCA and a program NCA; extra files (by
// name) are added, a ticket for example.
func safetyTitleNsp(t *testing.T, path string, titleID uint64, version uint32, metaType byte, seed int, extra map[string][]byte) string {
	t.Helper()
	program := safetyProgramNca(safetyGameData(0x10000+seed*0x200, seed))
	return safetyNspWith(t, path, titleID, version, metaType, program, safetyContentName(program, ".nca"), extra)
}

// safetyNspWith writes an NSP of one title with the program NCA given, under its name.
func safetyNspWith(t *testing.T, path string, titleID uint64, version uint32, metaType byte, program []byte, programName string, extra map[string][]byte) string {
	t.Helper()
	var id [16]byte
	hex.Decode(id[:], []byte(strings.TrimSuffix(programName, ".nca")))
	cnmt := make([]byte, 0x20+0x38)
	binary.LittleEndian.PutUint64(cnmt[0:8], titleID)
	binary.LittleEndian.PutUint32(cnmt[8:12], version)
	cnmt[0xC] = metaType
	binary.LittleEndian.PutUint16(cnmt[0x10:0x12], 1)
	copy(cnmt[0x20+0x20:], id[:])
	cnmt[0x20+0x36] = 1
	meta := safetyMetaNca(safetyPad(safetyPFS0([]string{"Title.cnmt"}, [][]byte{cnmt})))
	names := []string{safetyContentName(meta, ".cnmt.nca"), programName}
	payloads := [][]byte{meta, program}
	extras := []string{}
	for name := range extra {
		extras = append(extras, name)
	}
	sort.Strings(extras)
	for _, name := range extras {
		names = append(names, name)
		payloads = append(payloads, extra[name])
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, safetyPFS0(names, payloads), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// the title IDs of the synthetic game, its update and its DLC
const (
	safetyBaseID   = 0x0100000000010000
	safetyUpdateID = 0x0100000000010800
	safetyDlcID    = 0x0100000000011001
)

// safetyGame writes a game, its update and a DLC into folder.
func safetyGame(t *testing.T, folder string) (string, string, string) {
	t.Helper()
	base := safetyTitleNsp(t, filepath.Join(folder, "base.nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	update := safetyTitleNsp(t, filepath.Join(folder, "update.nsp"), safetyUpdateID, 65536, switchfs.ContentMetaType_Patch, 2, nil)
	dlc := safetyTitleNsp(t, filepath.Join(folder, "dlc.nsp"), safetyDlcID, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
	return base, update, dlc
}

func safetyExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// safetyList is what a folder holds, by name.
func safetyList(folder string) []string {
	entries, _ := os.ReadDir(folder)
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// safetyResults keeps how each file of a work ended.
type safetyResults struct {
	quiet
	mutex   sync.Mutex
	results map[string]safetyResult
}

type safetyResult struct {
	output string
	note   string
	err    error
}

func (r *safetyResults) done(path string, output string, note string, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.results == nil {
		r.results = map[string]safetyResult{}
	}
	r.results[path] = safetyResult{output: output, note: note, err: err}
}

// failed are the files of a work that failed, by path.
func (r *safetyResults) failed() []string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	paths := []string{}
	for path, result := range r.results {
		if result.err != nil && !isSkipped(result.err) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

// safetyTitles are the titles a game file holds, as "<title ID> v<version>", in order.
func safetyTitles(t *testing.T, path string) []string {
	t.Helper()
	titles, err := readTitles(path)
	if err != nil {
		t.Fatalf("%s: %v", filepath.Base(path), err)
	}
	found := []string{}
	for _, title := range titles {
		found = append(found, fmt.Sprintf("%s v%d", strings.ToUpper(title.TitleId), title.Version))
	}
	sort.Strings(found)
	return found
}

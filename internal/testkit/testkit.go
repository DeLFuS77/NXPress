// Package testkit makes games for the tests, with keys of their own, and an output that keeps
// how each file ended.
package testkit

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
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/cache"

	"github.com/dtrunk90/switch-library-manager-web/settings"
	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

const (
	HeaderKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	AreaKey   = "1f1e1d1c1b1a19181716151413121110"
	ncaKey    = "00112233445566778899aabbccddeeff"
)

// Isolate keeps a test away from the cache of the files checked and from the keys of the
// user: the cache starts empty in a folder of the test, and the keys are made up.
func Isolate(t *testing.T) {
	t.Helper()
	config := t.TempDir()
	t.Setenv("AppData", config)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("HOME", config)
	before := cache.Checked
	cache.Checked = &cache.CheckCache{}
	t.Cleanup(func() { cache.Checked = before })
	keys := "header_key = " + HeaderKey + "\nkey_area_key_application_00 = " + AreaKey + "\nkey_area_key_application_02 = " + AreaKey + "\n"
	path := filepath.Join(t.TempDir(), "prod.keys")
	if err := os.WriteFile(path, []byte(keys), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := settings.UseSwitchKeys(path); err != nil {
		t.Fatal(err)
	}
}

func sectorTweak(sector int) [16]byte {
	tweak := [16]byte{}
	for i := 0xF; i >= 0; i-- {
		tweak[i] = uint8(sector & 0xFF)
		sector >>= 8
	}
	return tweak
}

func nextTweak(tweak []byte) {
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

// encryptHeader encrypts an NCA header as Nintendo does, with AES-XTS and the header key.
func encryptHeader(plain []byte) []byte {
	key, _ := hex.DecodeString(HeaderKey)
	first, _ := aes.NewCipher(key[:len(key)/2])
	second, _ := aes.NewCipher(key[len(key)/2:])
	result := make([]byte, len(plain))
	for sector := 0; sector < len(plain)/0x200; sector++ {
		start := sector * 0x200
		tweak := sectorTweak(sector)
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
			nextTweak(encrypted)
		}
	}
	return result
}

func GameData(size int, seed int) []byte {
	data := make([]byte, size)
	text := []byte(fmt.Sprintf("Synthetic game data %d; the same words again and again. ", seed))
	for i := range data {
		data[i] = text[i%len(text)] ^ byte(i/4096)
	}
	return data
}

func programNca(plain []byte) []byte {
	return ProgramNcaWith(plain, nil)
}

// ProgramNcaWith is safetyProgramNca with its plain header changed by change first.
func ProgramNcaWith(plain []byte, change func(header []byte)) []byte {
	areaKey, _ := hex.DecodeString(AreaKey)
	contentKey, _ := hex.DecodeString(ncaKey)
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
	copy(nca, encryptHeader(header))
	section := append([]byte(nil), plain...)
	block, _ := aes.NewCipher(contentKey)
	counter := []byte{0x55, 0x66, 0x77, 0x88, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint64(counter[8:], 0x4000>>4)
	cipher.NewCTR(block, counter).XORKeyStream(section, section)
	return append(nca, section...)
}

// RewrittenNca is a program NCA as a tool leaves it: its header changed after it was
// named, so it no longer matches its content ID. Its data is checked by the hash tree of its
// second section; its first has none, and cannot be checked on its own: the check of a file
// that holds it is in part. It gives the NCA and its name, the content ID of the original.
func RewrittenNca() ([]byte, string) {
	plain := GameData(0x10000, 7)
	tree := make([]byte, 0x400)
	copy(tree[0x200:], GameData(0x200, 8))
	block := sha256.Sum256(tree[0x200:])
	copy(tree, block[:])
	master := sha256.Sum256(tree[:0x20])
	build := func(rewritten bool) []byte {
		nca := ProgramNcaWith(plain, func(header []byte) {
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
	return build(true), ContentName(build(false), ".nca")
}

func metaNca(section []byte) []byte {
	contentKey, _ := hex.DecodeString(ncaKey)
	areaKey, _ := hex.DecodeString(AreaKey)
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
	return append(encryptHeader(header), encoded...)
}

func pfs0(names []string, payloads [][]byte) []byte {
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

func pad(data []byte) []byte {
	return append(data, make([]byte, (0x200-len(data)%0x200)%0x200)...)
}

func ContentName(nca []byte, suffix string) string {
	sum := sha256.Sum256(nca)
	return hex.EncodeToString(sum[:16]) + suffix
}

// TitleNsp writes an NSP of one title, its Meta NCA and a program NCA; extra files (by
// name) are added, a ticket for example.
func TitleNsp(t *testing.T, path string, titleID uint64, version uint32, metaType byte, seed int, extra map[string][]byte) string {
	t.Helper()
	program := programNca(GameData(0x10000+seed*0x200, seed))
	return NspWith(t, path, titleID, version, metaType, program, ContentName(program, ".nca"), extra)
}

// NspWith writes an NSP of one title with the program NCA given, under its name.
func NspWith(t *testing.T, path string, titleID uint64, version uint32, metaType byte, program []byte, programName string, extra map[string][]byte) string {
	t.Helper()
	return NspListing(t, path, titleID, version, metaType, program, programName, strings.TrimSuffix(programName, ".nca"), extra)
}

// NspListing is NspWith with its CNMT listing the program under the content ID listed, which
// need not be its name: a CNMT that does not match the package, as a tool can leave it.
func NspListing(t *testing.T, path string, titleID uint64, version uint32, metaType byte, program []byte, programName string, listed string, extra map[string][]byte) string {
	t.Helper()
	var id [16]byte
	hex.Decode(id[:], []byte(listed))
	// the header, one content and the digest at the end, as the CNMT of Nintendo
	cnmt := make([]byte, 0x20+0x38+0x20)
	binary.LittleEndian.PutUint64(cnmt[0:8], titleID)
	binary.LittleEndian.PutUint32(cnmt[8:12], version)
	cnmt[0xC] = metaType
	binary.LittleEndian.PutUint16(cnmt[0x10:0x12], 1)
	copy(cnmt[0x20+0x20:], id[:])
	cnmt[0x20+0x36] = 1
	meta := metaNca(pad(pfs0([]string{"Title.cnmt"}, [][]byte{cnmt})))
	names := []string{ContentName(meta, ".cnmt.nca"), programName}
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
	if err := os.WriteFile(path, pfs0(names, payloads), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// the title IDs of the synthetic game, its update and its DLC
const (
	BaseID   = 0x0100000000010000
	UpdateID = 0x0100000000010800
	DlcID    = 0x0100000000011001
)

// Game writes a game, its update and a DLC into folder.
func Game(t *testing.T, folder string) (string, string, string) {
	t.Helper()
	base := TitleNsp(t, filepath.Join(folder, "base.nsp"), BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	update := TitleNsp(t, filepath.Join(folder, "update.nsp"), UpdateID, 65536, switchfs.ContentMetaType_Patch, 2, nil)
	dlc := TitleNsp(t, filepath.Join(folder, "dlc.nsp"), DlcID, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
	return base, update, dlc
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// List is what a folder holds, by name.
func List(folder string) []string {
	entries, _ := os.ReadDir(folder)
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// Results keeps how each file of a work ended.
type Results struct {
	Quiet
	mutex   sync.Mutex
	Results map[string]outcome
}

type outcome struct {
	output string
	note   string
	Err    error
}

func (r *Results) Done(path string, output string, note string, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.Results == nil {
		r.Results = map[string]outcome{}
	}
	r.Results[path] = outcome{output: output, note: note, Err: err}
}

// Failed are the files of a work that failed, by path; skipped tells the errors that are no failure.
func (r *Results) Failed(skipped func(error) bool) []string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	paths := []string{}
	for path, result := range r.Results {
		if result.Err != nil && !skipped(result.Err) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

// Quiet takes the progress and the results of a test without printing them.
type Quiet struct{}

func (Quiet) Progress(string, string) func(int64, int64) { return func(int64, int64) {} }
func (Quiet) Done(string, string, string, error)         {}
func (Quiet) Say(string, ...any)                         {}

// the titles of the synthetic game, its update and its DLC together
var WholeGame = []string{"0100000000010000 v0", "0100000000010800 v65536", "0100000000011001 v0"}

// UnreadableFolder makes a folder in folder that cannot be read: one whose name ends in a dot on
// Windows, which only the long form of its path reaches, and one without permissions elsewhere.
func UnreadableFolder(t *testing.T, folder string) string {
	if runtime.GOOS == "windows" {
		path := filepath.Join(folder, "locked.")
		if err := os.Mkdir(`\\?\`+path, 0o755); err != nil {
			t.Skip(err)
		}
		os.WriteFile(`\\?\`+path+`\game.nsp`, []byte("x"), 0o644)
		return path
	}
	path := filepath.Join(folder, "locked")
	os.Mkdir(path, 0o755)
	os.WriteFile(filepath.Join(path, "game.nsp"), []byte("x"), 0o644)
	os.Chmod(path, 0)
	t.Cleanup(func() { os.Chmod(path, 0o755) })
	if _, err := os.ReadDir(path); err == nil {
		t.Skip("the folder can be read without permissions (root)")
	}
	return path
}

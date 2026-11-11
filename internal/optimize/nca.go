package optimize

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/settings"
)

// keyring is what the merge needs of prod.keys and title.keys, read once; the keys stay in
// memory only and are never told.
type keyring struct {
	header  []byte // header_key, 32 bytes
	lookup  func(name string) []byte
	titleDB func(rights string) []byte
}

func loadKeys() (*keyring, error) {
	keys, _ := settings.SwitchKeys()
	if keys == nil {
		return nil, errors.New("no prod.keys loaded")
	}
	get := func(name string) []byte {
		b, err := hex.DecodeString(strings.TrimSpace(keys.GetKey(name)))
		if err != nil {
			return nil
		}
		return b
	}
	k := &keyring{header: get("header_key"), lookup: get}
	k.titleDB = func(rights string) []byte {
		v, ok := keys.TitleKey(rights)
		if !ok {
			return nil
		}
		b, err := hex.DecodeString(strings.TrimSpace(v))
		if err != nil || len(b) != 16 {
			return nil
		}
		return b
	}
	if len(k.header) != 32 {
		return nil, errors.New("prod.keys has no header_key")
	}
	return k, nil
}

// xts is the AES-XTS of NCA headers, as Nintendo makes it: sectors of 0x200 from 0, the number of
// each sector big endian as its tweak (golang.org/x/crypto/xts takes it little endian).
func xts(key []byte, data []byte, encrypt bool) error {
	if len(key) != 32 || len(data)%0x200 != 0 {
		return errors.New("not an NCA header")
	}
	k1, err := aes.NewCipher(key[:16])
	if err != nil {
		return err
	}
	k2, err := aes.NewCipher(key[16:])
	if err != nil {
		return err
	}
	for sector := 0; sector*0x200 < len(data); sector++ {
		var t [16]byte
		binary.BigEndian.PutUint64(t[8:], uint64(sector))
		k2.Encrypt(t[:], t[:])
		for b := sector * 0x200; b < (sector+1)*0x200; b += 16 {
			p := data[b : b+16]
			for i := range 16 {
				p[i] ^= t[i]
			}
			if encrypt {
				k1.Encrypt(p, p)
			} else {
				k1.Decrypt(p, p)
			}
			for i := range 16 {
				p[i] ^= t[i]
			}
			// the tweak times x in GF(2^128), little endian
			carry := byte(0)
			for i := range 16 {
				next := t[i] >> 7
				t[i] = t[i]<<1 | carry
				carry = next
			}
			if carry != 0 {
				t[0] ^= 0x87
			}
		}
	}
	return nil
}

func ecbDecrypt(key []byte, data []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	for i := 0; i+16 <= len(data); i += 16 {
		block.Decrypt(data[i:i+16], data[i:i+16])
	}
	return nil
}

// counterAt is the AES-CTR counter of a section at a place of the NCA.
func counterAt(fsh []byte, at uint64) []byte {
	ctr := make([]byte, 16)
	for i := range 8 {
		ctr[i] = fsh[0x147-i]
	}
	binary.BigEndian.PutUint64(ctr[8:], at>>4)
	return ctr
}

func ctrCrypt(key []byte, fsh []byte, at uint64, data []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	cipher.NewCTR(block, counterAt(fsh, at)).XORKeyStream(data, data)
	return nil
}

// nca is one NCA of a package: where it is, its header decrypted and the key of its sections.
type nca struct {
	name   string
	file   *pkgFile
	offset int64
	size   int64
	header []byte // 0xC00, decrypted
	key    []byte // the key of its sections
}

func (n *nca) contentType() byte { return n.header[0x205] }

func (n *nca) rightsID() []byte { return n.header[0x230:0x240] }

func (n *nca) hasRights() bool { return !bytes.Equal(n.rightsID(), make([]byte, 16)) }

// section is where section i is in the NCA, in bytes; false when there is none.
func (n *nca) section(i int) (start uint64, end uint64, ok bool) {
	st := binary.LittleEndian.Uint32(n.header[0x240+i*0x10:])
	en := binary.LittleEndian.Uint32(n.header[0x244+i*0x10:])
	return uint64(st) * 0x200, uint64(en) * 0x200, en > st
}

func (n *nca) fsHeader(i int) []byte { return n.header[0x400+i*0x200 : 0x600+i*0x200] }

// romfsIndex is the section holding the RomFS.
func (n *nca) romfsIndex() int {
	for i := range 4 {
		if _, _, ok := n.section(i); ok && n.fsHeader(i)[2] == 0 {
			return i
		}
	}
	return -1
}

// ReadAt reads the NCA as it is stored.
func (n *nca) ReadAt(p []byte, off int64) (int, error) {
	if off >= n.size {
		return 0, io.EOF
	}
	if int64(len(p)) > n.size-off {
		k, err := n.file.ReadAt(p[:n.size-off], n.offset+off)
		if err == nil {
			err = io.EOF
		}
		return k, err
	}
	return n.file.ReadAt(p, n.offset+off)
}

func (n *nca) generation() int {
	gen := max(int(n.header[0x206]), int(n.header[0x220]))
	if gen > 0 {
		gen--
	}
	return gen
}

// unlock decrypts the key of the sections of the NCA: the title key of its ticket (or of
// title.keys) for an NCA with a rights ID, else the key of its key area.
func (n *nca) unlock(k *keyring, tickets map[string][]byte) error {
	gen := n.generation()
	if n.hasRights() {
		rights := hex.EncodeToString(n.rightsID())
		titleKey := tickets[rights]
		if titleKey == nil {
			titleKey = k.titleDB(rights)
		}
		if titleKey == nil {
			return fmt.Errorf("%s: no ticket and no title key for its rights ID", n.name)
		}
		kek := k.lookup(fmt.Sprintf("titlekek_%02x", gen))
		if len(kek) != 16 {
			return fmt.Errorf("%s: prod.keys has no titlekek of generation %d", n.name, gen)
		}
		n.key = append([]byte(nil), titleKey...)
		return ecbDecrypt(kek, n.key)
	}
	if n.header[0x207] != 0 {
		return fmt.Errorf("%s: not the key area of an application", n.name)
	}
	kak := k.lookup(fmt.Sprintf("key_area_key_application_%02x", gen))
	if len(kak) != 16 {
		return fmt.Errorf("%s: prod.keys has no key_area_key_application of generation %d", n.name, gen)
	}
	n.key = append([]byte(nil), n.header[0x320:0x330]...)
	return ecbDecrypt(kak, n.key)
}

// readSection reads section i decrypted (for the small ones: the meta NCAs).
func (n *nca) readSection(i int) ([]byte, error) {
	start, end, ok := n.section(i)
	if !ok || end > uint64(n.size) || end-start > 64<<20 {
		return nil, fmt.Errorf("%s: no section %d", n.name, i)
	}
	data := make([]byte, end-start)
	if _, err := n.ReadAt(data, int64(start)); err != nil {
		return nil, err
	}
	if n.fsHeader(i)[4] == 3 { // AES-CTR
		if err := ctrCrypt(n.key, n.fsHeader(i), start, data); err != nil {
			return nil, err
		}
	}
	return data, nil
}

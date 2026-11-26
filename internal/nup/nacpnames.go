package nup

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// where a NACP tells it keeps its names compressed (1), and the room the names have
const (
	nacpCompressedFlag = 0x3215
	nacpTitleData      = nacpLanguages * nacpEntry
)

// errNamesUnreadable is a NACP that keeps its names compressed, whose names do not decompress.
var errNamesUnreadable = errors.New("its NACP keeps its names compressed, and they cannot be read")

// nacpCompressed tells whether a NACP keeps its names compressed.
func nacpCompressed(nacp []byte) bool {
	return len(nacp) > nacpCompressedFlag && nacp[nacpCompressedFlag] == 1
}

// nacpTitles are the names of a NACP as entries of 0x300 bytes (a name of 0x200, a publisher of
// 0x100) and how many there are: 16 as they are, 32 decompressed from a NACP that keeps them
// compressed. Nil for a NACP too small to have them; errNamesUnreadable when they are compressed
// and do not decompress. The entries are a copy.
func nacpTitles(nacp []byte) ([]byte, int, error) {
	if len(nacp) < nacpTitleData {
		return nil, 0, nil
	}
	entries, count, err := switchfs.NacpTitleEntries(nacp)
	if err != nil {
		if nacpCompressed(nacp) {
			return nil, 0, errNamesUnreadable
		}
		return nil, 0, err
	}
	return bytes.Clone(entries), count, nil
}

// NamesUnreadable tells whether the NACP of a package keeps its names compressed and they cannot
// be read: the console and the programs that read it show no name, or a broken one.
func (pkg Package) NamesUnreadable() bool {
	_, _, err := nacpTitles(pkg.Meta[tlvNacp])
	return errors.Is(err, errNamesUnreadable)
}

// packNacpTitles writes entries of names into a NACP that keeps them compressed, as the console
// keeps them: the size as a u16, then deflate without a header, in the first 0x3000 bytes, the
// rest of them zero.
func packNacpTitles(nacp []byte, entries []byte) error {
	var packed bytes.Buffer
	writer, err := flate.NewWriter(&packed, flate.BestCompression)
	if err != nil {
		return err
	}
	if _, err := writer.Write(entries); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if packed.Len() > nacpTitleData-2 || packed.Len() > 0xFFFF {
		return errors.New("its names compressed do not fit in its NACP")
	}
	clear(nacp[:nacpTitleData])
	binary.LittleEndian.PutUint16(nacp, uint16(packed.Len()))
	copy(nacp[2:], packed.Bytes())
	return nil
}

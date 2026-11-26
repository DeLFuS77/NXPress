// Package testnx builds small .nx files (containers of decrypted games) for tests: the two
// header pages, a media region with a NACP and an icon, the catalog of one game with its versions
// and an empty manifest, as the format (github.com/fab918/nxformat, header version 0.2) describes
// them. No file of the game is stored: the library reads only the catalog. Switch Library Manager Web has the same
// builder.
package testnx

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"hash/crc32"
)

// kinds of versions
const (
	Base   = 0
	Update = 1
	Dlc    = 2
)

const (
	slot            = 4096
	headerSize      = 256
	catalogHeader   = 128
	applicationSize = 80
	programSize     = 64
	snapshotSize    = 120
	mediaHeader     = 32
	mediaRecord     = 64
	absent          = 0xffffffff
	mediaJpeg       = 2
	mediaNacp       = 4
)

// Snapshot is a version of the game: the game, an update or a DLC.
type Snapshot struct {
	Kind    byte
	Version uint32
	// for a DLC: its title ID, as a DLC converted from a NUP keeps it; 0 for one from an NSP
	Addon   uint64
	Display string
}

// Game is the game a file holds. Its NACP (with NacpName as the name in American English) goes
// to the newest of its game and updates, Title and Publisher to the catalog.
type Game struct {
	Id        uint64
	Platform  byte // 1 Switch, 2 Switch 2
	Title     string
	Publisher string
	NacpName  string
	Snapshots []Snapshot
}

func align(b []byte, n int) []byte {
	for len(b)%n != 0 {
		b = append(b, 0)
	}
	return b
}

// Icon is the icon each file gets: the start of a JPEG.
var Icon = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{7}, 60)...)

// Build makes the bytes of a .nx file of the game.
func Build(game Game) []byte {
	le := binary.LittleEndian
	if game.Platform == 0 {
		game.Platform = 1
	}

	newest := -1
	for i, s := range game.Snapshots {
		if s.Kind != Dlc && (newest < 0 || s.Version >= game.Snapshots[newest].Version) {
			newest = i
		}
	}
	nacp := make([]byte, 0x4000)
	copy(nacp[0:], game.NacpName)
	copy(nacp[0x200:], game.Publisher)
	if newest >= 0 {
		copy(nacp[0x3060:], game.Snapshots[newest].Display)
	}
	payloads := [][]byte{nacp, Icon}
	types := []uint16{mediaNacp, mediaJpeg}
	media := align(make([]byte, mediaHeader+len(payloads)*mediaRecord), 16)
	payloadArea := len(media)
	offsets := []int{}
	for _, p := range payloads {
		media = align(media, 16)
		offsets = append(offsets, len(media))
		media = append(media, p...)
	}
	copy(media[0:], "NXMD")
	le.PutUint16(media[4:], 1)
	le.PutUint16(media[6:], mediaHeader)
	le.PutUint64(media[8:], uint64(len(media)))
	le.PutUint32(media[16:], uint32(len(payloads)))
	le.PutUint16(media[20:], mediaRecord)
	le.PutUint32(media[24:], mediaHeader)
	le.PutUint32(media[28:], uint32(payloadArea))
	for i, p := range payloads {
		row := media[mediaHeader+i*mediaRecord:]
		le.PutUint16(row, types[i])
		le.PutUint64(row[4:], uint64(len(p)))
		le.PutUint64(row[12:], uint64(len(p)))
		le.PutUint64(row[20:], uint64(offsets[i]))
		sum := sha256.Sum256(p)
		copy(row[28:], sum[:])
	}

	pool := []byte{}
	str := func(s string) (uint32, uint32) {
		if s == "" {
			return absent, 0
		}
		offset := uint32(len(pool))
		pool = append(pool, s...)
		return offset, uint32(len(s))
	}
	appOffset := uint64(catalogHeader)
	programOffset := appOffset + applicationSize
	snapshotOffset := programOffset + programSize
	poolOffset := snapshotOffset + uint64(len(game.Snapshots))*snapshotSize
	catalog := make([]byte, poolOffset)
	app := catalog[appOffset:]
	le.PutUint64(app, game.Id)
	app[8] = game.Platform
	executables, metadata := uint32(0), uint32(absent)
	flags := byte(0)
	for i, s := range game.Snapshots {
		switch s.Kind {
		case Base:
			flags |= 1
		case Dlc:
			flags |= 2
		}
		if s.Kind != Dlc {
			executables++
			if metadata == absent || s.Version >= game.Snapshots[metadata].Version {
				metadata = uint32(i)
			}
		}
	}
	app[9] = flags
	if executables > 0 {
		le.PutUint32(app[16:], 1)
	} else {
		le.PutUint32(app[12:], absent)
	}
	le.PutUint32(app[24:], uint32(len(game.Snapshots)))
	o, l := str(game.Title)
	le.PutUint32(app[28:], o)
	le.PutUint32(app[32:], l)
	o, l = str(game.Publisher)
	le.PutUint32(app[36:], o)
	le.PutUint32(app[40:], l)
	le.PutUint32(app[60:], 0)
	if metadata == absent {
		le.PutUint32(app[44:], absent)
		le.PutUint32(app[56:], absent)
	} else {
		le.PutUint32(app[44:], 1)
		le.PutUint32(app[56:], 0)
	}
	le.PutUint32(app[68:], metadata)
	programs := uint32(0)
	if executables > 0 {
		programs = 1
		program := catalog[programOffset:]
		le.PutUint64(program, game.Id)
		le.PutUint32(program[36:], executables)
		le.PutUint32(program[40:], absent)
	} else {
		// no program: the versions follow the game
		snapshotOffset = programOffset
		poolOffset = snapshotOffset + uint64(len(game.Snapshots))*snapshotSize
		catalog = catalog[:poolOffset]
	}
	for i, s := range game.Snapshots {
		row := catalog[snapshotOffset+uint64(i)*snapshotSize:]
		row[4] = s.Kind
		le.PutUint32(row[8:], s.Version)
		if s.Kind == Dlc {
			le.PutUint32(row, absent)
			if s.Addon != 0 {
				row[5] = 2
				le.PutUint64(row[32:], s.Addon)
			}
		}
		o, l := str(s.Display)
		le.PutUint32(row[24:], o)
		le.PutUint32(row[28:], l)
		row[56], row[57] = byte(i+1), byte(game.Id) // a distinct identity
	}
	catalog = align(append(catalog, pool...), 8)
	copy(catalog[0:], "NXCT")
	le.PutUint16(catalog[4:], 1)
	le.PutUint16(catalog[6:], catalogHeader)
	le.PutUint64(catalog[8:], uint64(len(catalog)))
	le.PutUint32(catalog[20:], 1)
	le.PutUint32(catalog[24:], programs)
	le.PutUint32(catalog[28:], uint32(len(game.Snapshots)))
	le.PutUint64(catalog[48:], appOffset)
	if programs > 0 {
		le.PutUint64(catalog[56:], programOffset)
	}
	le.PutUint64(catalog[64:], snapshotOffset)
	le.PutUint64(catalog[96:], poolOffset)
	le.PutUint64(catalog[104:], uint64(len(pool)))

	manifest := make([]byte, 96)
	copy(manifest, "NXMF")

	file := make([]byte, 2*slot)
	file = append(file, media...)
	file = align(file, 16)
	catalogAt := len(file)
	file = append(file, catalog...)
	file = align(file, 16)
	manifestAt := len(file)
	file = append(file, manifest...)
	header := make([]byte, headerSize)
	copy(header, "NXAR")
	le.PutUint16(header[6:], 2)
	le.PutUint32(header[8:], headerSize)
	le.PutUint16(header[12:], 1)
	le.PutUint64(header[16:], uint64(len(file)))
	le.PutUint64(header[24:], uint64(catalogAt))
	le.PutUint64(header[32:], uint64(len(catalog)))
	le.PutUint64(header[40:], 2*slot)
	le.PutUint64(header[48:], uint64(len(media)))
	le.PutUint64(header[72:], uint64(manifestAt))
	le.PutUint64(header[80:], uint64(len(manifest)))
	sum := sha256.Sum256(catalog)
	copy(header[120:], sum[:])
	sum = sha256.Sum256(manifest)
	copy(header[152:], sum[:])
	sum = sha256.Sum256(nil)
	copy(header[184:], sum[:])
	le.PutUint32(header[224:], 16384)
	Seal(header)
	copy(file[0:], header)
	copy(file[slot:], header)
	return file
}

// Seal writes again the CRC-32C of a header changed by a test.
func Seal(header []byte) {
	binary.LittleEndian.PutUint32(header[252:], crc32.Checksum(header[:252], crc32.MakeTable(crc32.Castagnoli)))
}

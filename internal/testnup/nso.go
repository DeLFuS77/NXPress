package testnup

import (
	"crypto/sha256"
	"encoding/binary"
)

// Nso is a code module of an ExeFS (main, sdk, subsdk0...) made of three segments, each
// compressed with LZ4 when compressed is set, with its SHA-256 in the header to be checked.
func Nso(compressed bool, text, ro, data []byte) []byte {
	const headerSize = 0x100
	header := make([]byte, headerSize)
	copy(header, "NSO0")
	le := binary.LittleEndian
	// the SHA-256 of the three segments is to be checked, and they are compressed if asked
	flags := uint32(0b111000)
	if compressed {
		flags |= 0b111
	}
	le.PutUint32(header[0x0C:], flags)
	body := []byte{}
	memory := uint32(0)
	for i, segment := range [][]byte{text, ro, data} {
		stored := segment
		if compressed {
			stored = Lz4(segment)
		}
		le.PutUint32(header[0x10+0x10*i:], uint32(headerSize+len(body)))
		le.PutUint32(header[0x14+0x10*i:], memory)
		le.PutUint32(header[0x18+0x10*i:], uint32(len(segment)))
		le.PutUint32(header[0x60+4*i:], uint32(len(stored)))
		sum := sha256.Sum256(segment)
		copy(header[0xA0+0x20*i:], sum[:])
		body = append(body, stored...)
		memory += uint32(len(segment)+0xFFF) &^ 0xFFF
	}
	return append(header, body...)
}

// Lz4 compresses data into one LZ4 block, simply: each match is the last place its first four
// bytes were seen, the last bytes are literals, as the format asks.
func Lz4(data []byte) []byte {
	const (
		minMatch = 4
		// a match starts at least 12 bytes before the end, and ends at least 5 before it
		matchLimit   = 12
		lastLiterals = 5
	)
	out := []byte{}
	seen := map[uint32]int{}
	anchor := 0
	for i := 0; i+matchLimit <= len(data); {
		key := binary.LittleEndian.Uint32(data[i:])
		candidate, ok := seen[key]
		seen[key] = i
		if !ok || i-candidate > 0xFFFF {
			i++
			continue
		}
		length := minMatch
		for i+length < len(data)-lastLiterals && data[candidate+length] == data[i+length] {
			length++
		}
		out = lz4Sequence(out, data[anchor:i], i-candidate, length-minMatch)
		i += length
		anchor = i
	}
	literals := data[anchor:]
	out = append(out, lz4Token(len(literals), 0))
	out = lz4Length(out, len(literals))
	return append(out, literals...)
}

func lz4Sequence(out []byte, literals []byte, offset int, match int) []byte {
	out = append(out, lz4Token(len(literals), match))
	out = lz4Length(out, len(literals))
	out = append(out, literals...)
	out = append(out, byte(offset), byte(offset>>8))
	return lz4Length(out, match)
}

func lz4Token(literals int, match int) byte {
	return byte(min(literals, 15)<<4 | min(match, 15))
}

// lz4Length writes the bytes that follow a length of 15 or more in a token.
func lz4Length(out []byte, n int) []byte {
	if n < 15 {
		return out
	}
	for n -= 15; n >= 255; n -= 255 {
		out = append(out, 255)
	}
	return append(out, byte(n))
}

package nup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The code of a game is in the NSO modules of its ExeFS: main, rtld, sdk and subsdk0 to
// subsdk9. The header of a module tells where each of its three segments (text, ro, data) is in
// the file and how many bytes it takes there, and its size and SHA-256 once decompressed (LZ4).
// A dump that lost bytes of a module still has hashes that match, in a NUP made of it, as the
// file is: but the module no longer loads, and an emulator stops while it launches the game.
// The check reads each module as its loader does. The library app does the same check
// (switchfs/nso.go).

const (
	nsoMagic      = "NSO0"
	nsoHeaderSize = 0x100
	// the flags of the header: bits 0 to 2, the segment is compressed with LZ4; bits 3 to 5,
	// its SHA-256 is to be checked; bit 7, its segments are compressed in another way (ZBIC),
	// which the check does not decompress
	nsoFlagOther = 1 << 7
	// a segment bigger than this once decompressed is not decompressed to check it: no module
	// has one so big. What a compressed segment takes is kept in memory while it is read, so a
	// crafted NUP, whose data of zeros zstd makes tiny, cannot make the check take gigabytes
	nsoMaxSegment = 256 << 20
	// LZ4 makes at most 255 bytes of each byte of its data
	lz4MaxRatio = 255
)

// lz4MaxStored is the most an LZ4 block of size bytes can take: a segment that says it takes more
// is not one LZ4 made.
func lz4MaxStored(size int64) int64 {
	return size + size/255 + 16
}

var nsoSegmentNames = [3]string{"text", "ro", "data"}

// the names of the modules of an ExeFS
var nsoModuleName = regexp.MustCompile(`(?i)^(main|rtld|sdk|subsdk[0-9])$`)

// IsCodeModule tells, by its path in a package or a dump ("exefs/main"), an NSO module of an
// ExeFS: a file named as a module, in a folder named exefs.
func IsCodeModule(filePath string) bool {
	// a backslash parts folders too, on every system: a path written on Windows reads the same
	// everywhere
	dir, name := path.Split(strings.ReplaceAll(filePath, `\`, "/"))
	return nsoModuleName.MatchString(name) && strings.EqualFold(path.Base(path.Clean("/"+dir)), "exefs")
}

// nsoSegment is a segment of a module: where its data is in the file and how many bytes it
// takes there, its size and SHA-256 decompressed, and its data as it goes by: gathered when it
// is compressed, hashed at once when it is not.
type nsoSegment struct {
	name                 string
	offset, stored, size int64
	compressed, hashed   bool
	hash                 [32]byte
	hasher               hash.Hash
	read                 int64
	data                 []byte
}

// nsoCheck reads a module of size bytes as it is written into it, and tells once it is
// written whole what is wrong with it (result). A file that is not a module (it does not start
// as one) is left alone.
type nsoCheck struct {
	size   int64
	at     int64
	header []byte
	// the segments still to read, in the order of the file; none when the module is not read
	// past its header
	segments []*nsoSegment
	problem  string
}

func newNsoCheck(size int64) *nsoCheck {
	return &nsoCheck{size: size, header: make([]byte, 0, nsoHeaderSize)}
}

func (c *nsoCheck) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		if len(c.header) < nsoHeaderSize {
			taken := min(nsoHeaderSize-len(c.header), len(p))
			c.header = append(c.header, p[:taken]...)
			c.at += int64(taken)
			p = p[taken:]
			if len(c.header) == nsoHeaderSize {
				c.readHeader()
			}
			continue
		}
		if len(c.segments) == 0 || c.problem != "" {
			c.at += int64(len(p))
			break
		}
		segment := c.segments[0]
		if c.at < segment.offset {
			skipped := min(segment.offset-c.at, int64(len(p)))
			c.at += skipped
			p = p[skipped:]
			continue
		}
		taken := min(segment.offset+segment.stored-c.at, int64(len(p)))
		if segment.compressed {
			if segment.data == nil {
				segment.data = make([]byte, 0, segment.stored)
			}
			segment.data = append(segment.data, p[:taken]...)
		} else {
			segment.hasher.Write(p[:taken])
		}
		segment.read += taken
		c.at += taken
		p = p[taken:]
		if segment.read == segment.stored {
			c.problem = segment.check()
			c.segments = c.segments[1:]
		}
	}
	return n, nil
}

// module tells a file that starts as a module.
func (c *nsoCheck) module() bool {
	return len(c.header) >= 4 && string(c.header[:4]) == nsoMagic
}

// readHeader reads the segments the header tells, and tells a module that lacks bytes of them.
func (c *nsoCheck) readHeader() {
	if !c.module() {
		return
	}
	header := c.header
	le := binary.LittleEndian
	flags := le.Uint32(header[0x0C:])
	end := int64(nsoHeaderSize)
	segments := make([]*nsoSegment, 0, 3)
	for i := range 3 {
		segment := &nsoSegment{
			name:       nsoSegmentNames[i],
			offset:     int64(le.Uint32(header[0x10+0x10*i:])),
			size:       int64(le.Uint32(header[0x18+0x10*i:])),
			compressed: flags&(1<<i) != 0,
			hashed:     flags&(1<<(i+3)) != 0,
		}
		segment.stored = segment.size
		if segment.compressed {
			segment.stored = int64(le.Uint32(header[0x60+4*i:]))
		}
		copy(segment.hash[:], header[0xA0+0x20*i:])
		end = max(end, segment.offset+segment.stored)
		// the segments of another compression are not read; a segment that cannot hold what the
		// header says, or that takes more bytes than LZ4 makes of its size, is told before it is
		// read; the ones too big to decompress are not read
		if segment.compressed && flags&nsoFlagOther != 0 {
			continue
		}
		if segment.compressed && (segment.size > segment.stored*lz4MaxRatio || segment.stored > lz4MaxStored(segment.size)) {
			c.problem = fmt.Sprintf("its %s segment cannot decompress to the %d bytes its header says", segment.name, segment.size)
			return
		}
		if segment.size > nsoMaxSegment {
			continue
		}
		if !segment.compressed {
			segment.hasher = sha256.New()
		}
		if segment.compressed || segment.hashed {
			segments = append(segments, segment)
		}
	}
	if c.size < end {
		c.problem = fmt.Sprintf("it is %d bytes shorter than its header says", end-c.size)
		return
	}
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].offset < segments[j].offset })
	for i, segment := range segments {
		// segments over the header or over each other are not read in one pass: the size is
		// checked, not their data
		if segment.offset < nsoHeaderSize || i > 0 && segment.offset < segments[i-1].offset+segments[i-1].stored {
			return
		}
	}
	// a segment of no bytes is checked at once
	for len(segments) > 0 && segments[0].stored == 0 {
		if c.problem = segments[0].check(); c.problem != "" {
			return
		}
		segments = segments[1:]
	}
	c.segments = segments
}

// check decompresses a segment gathered whole, and compares it with its header. It is
// decompressed into its hash, not into memory.
func (s *nsoSegment) check() string {
	if s.compressed {
		data := s.data
		s.data = nil
		s.hasher = sha256.New()
		if err := lz4DecompressTo(data, int(s.size), s.hasher); err != nil {
			return fmt.Sprintf("its %s segment does not decompress to the %d bytes its header says (LZ4)", s.name, s.size)
		}
	}
	if s.hashed && !bytes.Equal(s.hasher.Sum(nil), s.hash[:]) {
		return fmt.Sprintf("the SHA-256 of its %s segment does not match its header", s.name)
	}
	return ""
}

// result is what is wrong with the module once it was written whole: "" when nothing, or when
// it is no module. A module that ends in its header lacks bytes of it.
func (c *nsoCheck) result() string {
	if c.problem != "" {
		return c.problem
	}
	if len(c.header) < nsoHeaderSize && c.module() {
		return fmt.Sprintf("it ends in its header (%d of %d bytes)", len(c.header), nsoHeaderSize)
	}
	return ""
}

// lz4Decompress decompresses an LZ4 block into exactly size bytes.
func lz4Decompress(block []byte, size int) ([]byte, error) {
	out := bytes.NewBuffer(make([]byte, 0, size))
	if err := lz4DecompressTo(block, size, out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// lz4DecompressTo decompresses an LZ4 block of exactly size bytes into w. Only the last 64 KiB
// are kept, as far back as a match of LZ4 reaches, so a segment of hundreds of megabytes takes
// no more memory than its data. The library app decompresses them the same way
// (switchfs/nso.go).
func lz4DecompressTo(block []byte, size int, w io.Writer) error {
	const (
		minMatch = 4
		window   = 1 << 16
	)
	bad := errors.New("damaged LZ4 block")
	out := make([]byte, 0, 4*window)
	// the bytes already handed to w and dropped from out
	handed := 0
	flush := func(keep int) error {
		if len(out) <= keep {
			return nil
		}
		n := len(out) - keep
		if _, err := w.Write(out[:n]); err != nil {
			return err
		}
		handed += n
		out = out[:copy(out, out[n:])]
		return nil
	}
	i := 0
	for i < len(block) {
		token := block[i]
		i++
		literals := int(token >> 4)
		if literals == 15 {
			for {
				if i >= len(block) {
					return bad
				}
				b := block[i]
				i++
				literals += int(b)
				if b != 255 {
					break
				}
			}
		}
		if i+literals > len(block) || handed+len(out)+literals > size {
			return bad
		}
		out = append(out, block[i:i+literals]...)
		i += literals
		if len(out) > 3*window {
			if err := flush(window); err != nil {
				return err
			}
		}
		if i == len(block) {
			break
		}
		if i+2 > len(block) {
			return bad
		}
		offset := int(block[i]) | int(block[i+1])<<8
		i += 2
		length := int(token&15) + minMatch
		if token&15 == 15 {
			for {
				if i >= len(block) {
					return bad
				}
				b := block[i]
				i++
				length += int(b)
				if b != 255 {
					break
				}
			}
		}
		if offset == 0 || offset > len(out) || handed+len(out)+length > size {
			return bad
		}
		for length > 0 {
			n := min(length, window)
			for k := 0; k < n; k++ {
				out = append(out, out[len(out)-offset])
			}
			length -= n
			if len(out) > 3*window {
				if err := flush(window); err != nil {
					return err
				}
			}
		}
	}
	if err := flush(0); err != nil {
		return err
	}
	if handed != size {
		return bad
	}
	return nil
}

// CodeCheck is what the check of the code of a dump folder found: the modules read, and the
// ones damaged.
type CodeCheck struct {
	Modules int
	Damaged []Damaged
}

// CheckDumpCode reads every code module of the ExeFS of each package of a dump folder, as its
// loader does. A module that cannot be read stops the check with the error of the read.
func CheckDumpCode(ctx context.Context, root string) (CodeCheck, error) {
	check := CodeCheck{}
	sources, _, err := Discover(root)
	if err != nil {
		return check, err
	}
	for _, source := range sources {
		exefs := findDir(source.Dir, "exefs")
		if exefs == "" {
			continue
		}
		entries, err := os.ReadDir(exefs)
		if err != nil {
			return check, err
		}
		label := Label(Package{TitleID: source.TitleID, Kind: source.Kind, Version: source.Version})
		prefix, err := filepath.Rel(source.Dir, exefs)
		if err != nil {
			prefix = "exefs"
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() || !nsoModuleName.MatchString(entry.Name()) {
				continue
			}
			if err := ctx.Err(); err != nil {
				return check, err
			}
			module, problem, err := checkModuleFile(ctx, filepath.Join(exefs, entry.Name()))
			if err != nil {
				return check, err
			}
			if !module {
				continue
			}
			check.Modules++
			if problem != "" {
				check.Damaged = append(check.Damaged, Damaged{Package: label, Path: filepath.ToSlash(filepath.Join(prefix, entry.Name())),
					Reason: problem, Code: true})
			}
		}
	}
	return check, nil
}

// checkModuleFile reads a file of an ExeFS whole: whether it is a code module, and what is
// wrong with it.
func checkModuleFile(ctx context.Context, path string) (bool, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, "", err
	}
	check := newNsoCheck(info.Size())
	if _, err := io.Copy(check, &ctxReader{ctx: ctx, r: file}); err != nil {
		return false, "", err
	}
	return check.module(), check.result(), nil
}

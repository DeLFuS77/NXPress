package nup

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Eglantine and Lichii, the emulators that read NUP files (Lichii is a fork of Eglantine, with the
// same reader), are stricter than the format in a few things, and refuse a whole NUP that does
// not meet them: packages of kind 3 or 4 and flags other than the first, files with codec flags,
// files of 0 bytes that point at data, children of folders marked with the high bit, folders
// inside themselves or more than 128 deep, records of languages that run past their record. To
// start its game they take the ExeFS of the newest update, else the one of the game, and need
// main and main.npdm in it.

// refusedError is a NUP this reader does not read, which Eglantine and Lichii refuse too.
type refusedError struct {
	error
}

func (e refusedError) Unwrap() error {
	return e.error
}

func refused(err error) error {
	return refusedError{err}
}

// EmulatorsRefuse tells whether Eglantine and Lichii refuse too a NUP Open or Read did not read.
func EmulatorsRefuse(err error) bool {
	var r refusedError
	return errors.As(err, &r)
}

// Emulators is what Eglantine and Lichii make of a NUP.
type Emulators struct {
	// why they do not open it at all, none when they open it
	Refused []string
	// why they cannot start its game, "" when they can
	NoStart string
}

// Emulators tells whether Eglantine and Lichii open a NUP and can start its game.
func (c *Container) Emulators() Emulators {
	result := Emulators{Refused: append([]string{}, c.strict...)}
	if reason := c.treeFault(); reason != "" {
		result.Refused = append(result.Refused, reason)
	}
	result.NoStart = c.startFault()
	return result
}

// treeFault tells a folder of the table inside itself, or one more than 128 deep, among all
// the folders of the table, not only the ones of the packages, as the emulators look at them.
func (c *Container) treeFault() string {
	const (
		unseen = iota
		open
		done
	)
	state := make([]byte, len(c.Dirs))
	height := make([]int, len(c.Dirs))
	dirCount := uint32(len(c.Dirs))
	type frame struct {
		dir, next uint32
	}
	for start := range c.Dirs {
		if state[start] != unseen {
			continue
		}
		state[start] = open
		stack := []frame{{dir: uint32(start)}}
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			dir := c.Dirs[top.dir]
			if top.next == dir.ChildCount {
				state[top.dir] = done
				finished := top.dir
				stack = stack[:len(stack)-1]
				if len(stack) > 0 {
					parent := stack[len(stack)-1].dir
					height[parent] = max(height[parent], height[finished]+1)
				}
				continue
			}
			child := c.Children[dir.FirstChild+top.next]
			top.next++
			if child >= dirCount {
				continue
			}
			switch state[child] {
			case open:
				return fmt.Sprintf("the folder %q is inside itself", c.Dirs[child].Name)
			case done:
				height[top.dir] = max(height[top.dir], height[child]+1)
			default:
				state[child] = open
				stack = append(stack, frame{dir: child})
			}
			if len(stack)+height[child] > maxDepth {
				return "its folders go more than 128 deep"
			}
		}
	}
	return ""
}

// startFault tells why the emulators cannot start the game of a NUP, "" when they can: they take
// the ExeFS of its newest update when it has one, else the one of its game (an update stored
// without its game starts on its own), and need main and main.npdm in it, with those names.
func (c *Container) startFault() string {
	base, update := -1, -1
	for i, pkg := range c.Packages {
		switch {
		case pkg.Kind == KindBase && base < 0:
			base = i
		case pkg.Kind == KindUpdate && (update < 0 || pkg.Version > c.Packages[update].Version):
			update = i
		}
	}
	hasExefs := func(pkg int) bool {
		return pkg >= 0 && c.Packages[pkg].ExefsDir != NoDir && c.Dirs[c.Packages[pkg].ExefsDir].ChildCount > 0
	}
	exefs := -1
	switch {
	case hasExefs(base) && hasExefs(update):
		exefs = update
	case hasExefs(base):
		exefs = base
	case hasExefs(update):
		exefs = update
	case base < 0 && update < 0:
		return "it holds no game or update, and they do not load DLC from NUP files yet"
	default:
		return "no game or update in it has an ExeFS"
	}
	names := map[string]bool{}
	dir := c.Dirs[c.Packages[exefs].ExefsDir]
	for _, node := range c.Children[dir.FirstChild : dir.FirstChild+dir.ChildCount] {
		if node >= uint32(len(c.Dirs)) {
			names[c.Files[node-uint32(len(c.Dirs))].Name] = true
		}
	}
	missing := []string{}
	for _, name := range []string{"main", "main.npdm"} {
		if !names[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("the ExeFS of the %s lacks: %s", Label(c.Packages[exefs]), strings.Join(missing, ", "))
}

// strictLocalized tells whether the records of names and icons by language of a block of
// metadata, as its types are written (9 and 10), stay in their record as the emulators read them:
// a length, a tag, a length and the data.
func strictLocalized(block []byte) bool {
	for at := 0; at+tlvHeadSize <= len(block); {
		kind := binary.LittleEndian.Uint16(block[at:])
		length := int(binary.LittleEndian.Uint32(block[at+4:]))
		at += tlvHeadSize
		if length < 0 || length > len(block)-at {
			return true
		}
		payload := block[at : at+length]
		at += length
		if kind != tlvLocalizedTitles && kind != tlvLocalizedIcons {
			continue
		}
		for pos := 0; pos < len(payload); {
			tag := int(payload[pos])
			pos++
			if pos+tag+4 > len(payload) {
				return false
			}
			pos += tag
			size := int(binary.LittleEndian.Uint32(payload[pos:]))
			pos += 4
			if size < 0 || size > len(payload)-pos {
				return false
			}
			pos += size
		}
	}
	return true
}

// countOf is a number of things, as "1 file" or "12 files".
func countOf(n int, one string, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

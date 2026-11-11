// Package optimize is "Optimize for install" (experimental): a game and its update made into one
// NSP that takes the least room once installed. It holds one Application of the version of the
// update: its program NCA merged with the update, its RomFS with the LZ4 layer of the NCA (the LZ4
// HC of Nintendo at level 9, what is held again stored once, of each block the smaller of
// Nintendo's and the new one), a CNMT made again, the control and legal information of the update,
// and the tickets. The merge is the code of NXDock (internal/optimize/native), run on the
// computer; see docs/optimize-install.md.
package optimize

import (
	"errors"
	"fmt"
)

// Options are how a game is optimized.
type Options struct {
	// the LZ4 HC level (9: the one of NXDock; 1 to 12)
	Level int
	// threads packing the blocks (1 to 8: the code of NXDock takes no more)
	Threads int
	// the folder of the NSP made of an NSZ, XCI or XCZ given, while the game is optimized ("":
	// next to the new file)
	Work string
	// the progress: a step ("Reading", "Measuring", "Writing", "Checking") and its bytes
	Progress func(step string, done int64, total int64)
}

// Result is what was made.
type Result struct {
	TitleID       uint64
	Version       uint32 // of the update, now of the one Application
	BaseVersion   uint32
	InputSize     int64 // the base and the update as given (NSP)
	OutputSize    int64 // the new NSP
	ProgramBefore int64 // the program NCAs of the base and the update
	ProgramAfter  int64 // the merged program NCA
	RomfsSize     uint64
	RunsAgain     uint64
	BlocksAgain   uint64
	OldKept       uint64
	ProgramID     string // the content ID of the merged program
	ProgramHash   string // its SHA-256
}

// ErrNotBuilt is told when NXPress is built without cgo, which the code of NXDock needs.
var ErrNotBuilt = errors.New("this build of NXPress has no Optimize for install: it needs the C++ code built in (cgo)")

// ErrNotPair is told when the files are not one game and its update.
type ErrNotPair struct{ Why string }

func (e ErrNotPair) Error() string { return e.Why }

func (o Options) checked() (Options, error) {
	if o.Level == 0 {
		o.Level = 9
	}
	if o.Level < 1 || o.Level > 12 {
		return o, fmt.Errorf("the LZ4 level is 1 to 12, not %d", o.Level)
	}
	if o.Threads < 1 {
		o.Threads = 3
	}
	o.Threads = min(o.Threads, 8)
	if o.Progress == nil {
		o.Progress = func(string, int64, int64) {}
	}
	return o, nil
}

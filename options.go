package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// options are what the command line asks for. The options of nsz work the same, so NXPress
// can take its place in scripts and in the tools that call it; a few of them have no use
// here and are accepted without effect.
type options struct {
	compress   bool
	decompress bool
	verify     bool
	info       bool
	convert    bool
	recompress bool
	trim       bool
	estimate   bool
	measure    bool
	fat32      bool
	template   string
	watch      bool
	// the files in the folders inside the folders given too (the command line takes only the
	// ones of the folders given, as nsz)
	recursive bool
	noCache   bool
	// --rename leaves the files whose name has their title ID
	keepNamed bool
	// --pack-games leaves out the games with these title IDs, in capitals (from the window)
	skipGames map[string]bool
	// what -V checks: "full" (every NCA by its hashes, the default), "signature" (the header
	// of every NCA signed by Nintendo) or "both"
	verifyLevel string
	levels      bool
	split       bool
	pack        string
	packGames   bool
	packName    string
	level       int
	block       bool
	lowPriority bool
	threads     int
	multi       int
	output      string
	overwrite   bool
	rmSource    bool
	keys        string
	dat         string
	datFile     *dat
	minimal     bool
	machine     bool
	version     bool
	help        bool
	files       []string
	// the options of nsz accepted without effect, told once
	ignored []string
	// what the options given do not do together, told once
	notes []string
}

const (
	defaultLevel = 18
	maxLevel     = 19
)

// optionName is an option of the command line: its name (--verify, or -C for the two that
// have only a letter), its letter (0 for none), whether it takes a value, and whether nsz has
// it too.
type optionName struct {
	name   string
	letter byte
	value  bool
	nsz    bool
}

// every option: what each does is in flag and set
var optionNames = []optionName{
	{"-C", 'C', false, true}, {"-D", 'D', false, true},
	{"--verify", 'V', false, true}, {"--quick-verify", 'Q', false, true}, {"--verify-level", 0, true, false},
	{"--no-cache", 0, false, false}, {"--dat", 0, true, false},
	{"--level", 'l', true, true}, {"--block", 'B', false, true}, {"--solid", 'S', false, true},
	{"--threads", 't', true, true}, {"--low-priority", 0, false, false}, {"--multi", 'm', true, true},
	{"--output", 'o', true, true}, {"--watch", 0, false, false}, {"--recursive", 0, false, false},
	{"--overwrite", 'w', false, true},
	{"--rm-source", 0, false, true}, {"--info", 'i', false, true}, {"--estimate", 0, false, false},
	{"--levels", 0, false, false}, {"--measure", 0, false, false}, {"--keys", 0, true, true},
	{"--minimal-output", 0, false, true}, {"--machine-readable", 0, false, true},
	{"--version", 0, false, false}, {"--help", 'h', false, true},
	{"--convert", 0, false, false}, {"--recompress", 0, false, false}, {"--trim", 0, false, false},
	{"--fat32", 0, false, false}, {"--rename", 0, true, false}, {"--keep-named", 0, false, false},
	{"--split", 0, false, false}, {"--pack", 0, true, false}, {"--pack-games", 0, false, false},
	{"--pack-name", 0, true, false},
	// the ones of nsz that NXPress accepts and has no use for
	{"--long", 'L', false, true}, {"--keep", 'K', false, true}, {"--fix-padding", 'F', false, true},
	{"--alwaysParseCnmt", 'P', false, true}, {"--rm-old-version", 'r', false, true},
	{"--bs", 's', true, true}, {"--depth", 0, true, true},
}

// the options of nsz that NXPress accepts and has no use for
var noEffect = map[string]bool{
	"--long": true, "--keep": true, "--fix-padding": true, "--alwaysParseCnmt": true, "--rm-old-version": true, "--bs": true, "--depth": true,
}

// parseOptions reads the command line as argparse reads the one of nsz: letters together
// (-CV), a value after its option, after "=" or right after its letter (-l18, -oDIR), and a
// long option by the start of its name when only one begins so (--lev).
func parseOptions(args []string) (options, error) {
	o := options{level: defaultLevel, threads: 0, multi: 1}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			o.files = append(o.files, args[i+1:]...)
			break
		}
		if !isOption(arg) {
			o.files = append(o.files, arg)
			continue
		}
		// the value of an option given apart: the next argument, unless it is an option itself
		next := func() (string, bool) {
			if i+1 < len(args) && !isOption(args[i+1]) {
				i++
				return args[i], true
			}
			return "", false
		}
		if strings.HasPrefix(arg, "--") {
			typed, value, hasValue := strings.Cut(arg, "=")
			option, err := longOption(typed)
			if err != nil {
				return o, err
			}
			if !option.value {
				if hasValue {
					return o, fmt.Errorf("%s takes no value", option.name)
				}
				o.flag(option)
				continue
			}
			if !hasValue {
				value, hasValue = next()
			}
			if err := o.set(option, option.name, value, hasValue); err != nil {
				return o, err
			}
			continue
		}
		// letters together; one that takes a value takes the rest of the argument, or the next
		for j := 1; j < len(arg); j++ {
			option, ok := letterOption(arg[j])
			if !ok {
				if j == 1 {
					return o, fmt.Errorf("unknown option %s (see nxpress --help)", arg)
				}
				letter, _ := utf8.DecodeRuneInString(arg[j:])
				return o, fmt.Errorf("unknown option -%c (see nxpress --help)", letter)
			}
			told := "-" + arg[j:j+1]
			if !option.value {
				if j+1 < len(arg) && arg[j+1] == '=' {
					return o, fmt.Errorf("%s takes no value", told)
				}
				o.flag(option)
				continue
			}
			value, hasValue := strings.TrimPrefix(arg[j+1:], "="), j+1 < len(arg)
			if !hasValue {
				value, hasValue = next()
			}
			if err := o.set(option, told, value, hasValue); err != nil {
				return o, err
			}
			break
		}
	}
	return o, nil
}

// longOption is the option a long name stands for: the one of that name, or the only one whose
// name begins so. When several do, the one of nsz is taken if only one of them is, so a script
// written for nsz works the same: --lev is --level there, and here too beside --levels.
func longOption(typed string) (optionName, error) {
	found := []optionName{}
	for _, option := range optionNames {
		if option.name == typed {
			return option, nil
		}
		if strings.HasPrefix(option.name, "--") && strings.HasPrefix(option.name, typed) {
			found = append(found, option)
		}
	}
	if len(found) > 1 {
		names := []string{}
		ofNsz := []optionName{}
		for _, option := range found {
			names = append(names, option.name)
			if option.nsz {
				ofNsz = append(ofNsz, option)
			}
		}
		if len(ofNsz) == 1 {
			return ofNsz[0], nil
		}
		return optionName{}, fmt.Errorf("ambiguous option %s: it could be %s", typed, strings.Join(names, ", "))
	}
	if len(found) == 1 {
		return found[0], nil
	}
	return optionName{}, fmt.Errorf("unknown option %s (see nxpress --help)", typed)
}

// letterOption is the option of a letter: -V is --verify.
func letterOption(letter byte) (optionName, bool) {
	for _, option := range optionNames {
		if option.letter == letter {
			return option, true
		}
	}
	return optionName{}, false
}

// negativeNumber finds what argparse takes for a negative number, a value and not an option:
// -1, -.5
var negativeNumber = regexp.MustCompile(`^-\d+$|^-\d*\.\d+$`)

// isOption tells whether an argument is an option, which cannot be the value of the one before
// it: "-" alone is a file, and a negative number a value (-t -1).
func isOption(arg string) bool {
	return strings.HasPrefix(arg, "-") && arg != "-" && !negativeNumber.MatchString(arg)
}

// flag turns on an option without a value.
func (o *options) flag(option optionName) {
	switch option.name {
	case "-C":
		o.compress = true
	case "-D":
		o.decompress = true
	case "--verify", "--quick-verify":
		o.verify = true
	case "--block":
		o.block = true
	case "--low-priority":
		o.lowPriority = true
	case "--solid":
		o.block = false
	case "--overwrite":
		o.overwrite = true
	case "--rm-source":
		o.rmSource = true
	case "--info":
		o.info = true
	case "--convert":
		o.convert = true
	case "--recompress":
		o.recompress = true
	case "--trim":
		o.trim = true
	case "--estimate":
		o.estimate = true
	case "--measure":
		o.measure = true
	case "--fat32":
		o.fat32 = true
	case "--watch":
		o.watch = true
	case "--recursive":
		o.recursive = true
	case "--no-cache":
		o.noCache = true
	case "--keep-named":
		o.keepNamed = true
	case "--levels":
		o.levels = true
	case "--split":
		o.split = true
	case "--pack-games":
		o.packGames = true
	case "--minimal-output":
		o.minimal = true
	case "--machine-readable":
		o.machine = true
	case "--version":
		o.version = true
	case "--help":
		o.help = true
	default:
		if noEffect[option.name] {
			o.ignored = append(o.ignored, option.name)
		}
	}
}

// set gives an option its value; told is the option as it was given. Without one, -o puts the
// new files next to their originals, as in nsz, and the other options need theirs.
func (o *options) set(option optionName, told string, value string, hasValue bool) error {
	if !hasValue {
		if option.name == "--output" {
			o.output = ""
			return nil
		}
		return fmt.Errorf("%s needs a value", told)
	}
	number := func(min int) (int, error) {
		n, err := strconv.Atoi(value)
		if err != nil || n < min {
			return 0, fmt.Errorf("%s needs a number, not %q", told, value)
		}
		return n, nil
	}
	var err error
	switch option.name {
	case "--level":
		o.level, err = number(1)
	case "--threads":
		// nsz takes a number below 1 as "choose for me"
		if n, convErr := strconv.Atoi(value); convErr == nil && n < 1 {
			o.threads = 0
			return nil
		}
		o.threads, err = number(1)
	case "--multi":
		o.multi, err = number(1)
	case "--output":
		o.output = value
	case "--keys":
		o.keys = value
	case "--dat":
		o.dat = value
	case "--rename":
		o.template = value
	case "--pack":
		o.pack = value
	case "--pack-name":
		o.packName = value
	case "--verify-level":
		if value != "full" && value != "signature" && value != "both" {
			return errors.New("--verify-level is full, signature or both")
		}
		o.verifyLevel = value
	default:
		if noEffect[option.name] {
			o.ignored = append(o.ignored, option.name)
		}
	}
	return err
}

// defaultThreads are the threads of each file when none are asked: the usable processors,
// shared among the files worked on at once. The zstd library in C compresses one file on all
// of them (24 threads compressed 38 % faster than 12 on a processor of 24); the memory sets
// fewer when it is short (switchfs.LimitThreads).
func defaultThreads(files int) int {
	return max(1, switchfs.UsableProcessors()/max(1, files))
}

// errOutputModes is told when both outputs for other programs are asked, as nsz tells it.
var errOutputModes = errors.New("--minimal-output and --machine-readable cannot be used together")

// check tells an impossible request, and settles the values left to choose.
func (o *options) check() error {
	if o.minimal && o.machine {
		return errOutputModes
	}
	actions := 0
	for _, on := range []bool{o.compress, o.decompress, o.convert, o.recompress, o.trim, o.estimate, o.measure, o.fat32, o.template != "", o.split, o.pack != "", o.packGames, o.info} {
		if on {
			actions++
		}
	}
	if actions > 1 {
		return errors.New("choose one of -C, -D, -i, --convert, --recompress, --trim, --estimate, --measure, --fat32, --rename, --split, --pack and --pack-games")
	}
	// a watch would look for new files a pack does not take, without end
	if o.watch && (o.pack != "" || o.packGames) {
		return errors.New("--watch does not go with --pack or --pack-games: a pack is made of the files given at once")
	}
	if o.dat != "" && actions > 0 {
		return errors.New("--dat goes with -V: it tells whether the files checked are dumps a DAT lists")
	}
	if o.dat != "" {
		o.verify = true
	}
	if actions == 0 && !o.verify {
		return errors.New("nothing to do: use -C to compress, -D to decompress or -V to check (see nxpress --help)")
	}
	if len(o.files) == 0 {
		return errors.New("no file or folder given")
	}
	if o.level > maxLevel {
		o.notes = append(o.notes, fmt.Sprintf("-l %d: levels above %d make a zstd window larger than the 8 MiB installers decompress with, so %d is used", o.level, maxLevel, maxLevel))
		o.level = maxLevel
	}
	// an option that does nothing with the others is told, not passed over
	if o.levels && !o.estimate {
		o.notes = append(o.notes, "--levels has no effect without --estimate")
	}
	if o.keepNamed && o.template == "" {
		o.notes = append(o.notes, "--keep-named has no effect without --rename")
	}
	if o.packName != "" && !o.packGames {
		o.notes = append(o.notes, "--pack-name has no effect without --pack-games")
	}
	// these write no file
	for _, action := range []struct {
		on   bool
		name string
	}{{o.template != "", "--rename"}, {o.info, "-i"}, {o.estimate, "--estimate"}, {actions == 0, "-V"}} {
		if o.output != "" && action.on {
			o.notes = append(o.notes, "-o has no effect with "+action.name)
		}
	}
	if o.threads == 0 {
		o.threads = defaultThreads(o.multi)
	}
	return nil
}

const usage = `NXPress %s, compresses and decompresses Nintendo Switch NSP and XCI files (NSZ and XCZ)

Usage: nxpress [options] file or folder...

  -C                  compress NSP and XCI files to NSZ and XCZ
  -D                  decompress NSZ and XCZ files to NSP and XCI
  -V, --verify        check files: with -C or -D it is always done, alone it checks the
                      files given (NSP, NSZ, XCI, XCZ)
  -Q, --quick-verify  the same as -V
  --verify-level L    what -V checks: full (every NCA by its hashes, the default),
                      signature (the header of every NCA signed by Nintendo, in seconds:
                      a tool that rewrote an NCA leaves it unsigned) or both
  --no-cache          with -V, check every file again, also the ones checked before and not
                      changed since (NXPress remembers them, and every file it writes)
  --dat FILE          with -V, tell whether each file is a dump the DAT lists (the XML of
                      Logiqx, as No-Intro publishes them), by its size and hashes
  -l, --level N       compression level, 1 to 19 (default 18)
  -B, --block         block compression (always for XCZ): emulators that read NSZ run it
                      as it is; every installer takes it but Awoo
  -S, --solid         solid compression (the default for NSZ): the smallest, and every
                      installer takes it
  -t, --threads N     threads per file (default: every processor but an eighth, shared among
                      the files of -m; fewer when the memory is short; --measure tells the
                      fastest)
  --low-priority      run below the normal priority of the processor, so the computer stays
                      responsive while NXPress works
  -m, --multi N       files at once (default 1)
  -o, --output [DIR]  folder of the new files (default, and -o alone: next to each file)
  --recursive         take the files in the folders inside the folders given too, and in
                      theirs (without it, only the files of the folders given, as nsz)
  --watch             keep looking at the folders given and work on each new file once it
                      is copied, until Ctrl+C
  -w, --overwrite     replace a file that is already there
  --rm-source         delete each original once its new file is checked
  -i, --info          show the titles a file holds
  --estimate          tell the size and the time a compression would give, with -l, -t
                      and -B, from samples of each NSP and XCI: nothing is written
  --levels            with --estimate, at the levels 3, 9, 12, 15, 18 and 19, to choose one
  --measure           try threads and files at once on a few seconds of the first NSP or
                      XCI, at the level of -l (and -B), and tell the fastest for -t and -m
  --keys PATH         prod.keys or keys.txt, or a folder holding one (default: the folder
                      of NXPress, then ~/.switch)
  --minimal-output    print the progress on one line written over, "<percentage>%% <step>",
                      and "100%% Done" at the end, as nsz does
  --machine-readable  print one JSON object per line, as nsz does: progress, complete, error,
                      info, warning, heartbeat and, at the end, summary
  --version           show the version

  --convert           turn XCI and XCZ files into NSP and NSZ
  --recompress        write NSZ and XCZ files again, solid (-S) or in blocks (-B), at the
                      level of -l: no NSP in between and no keys needed; a file is replaced
                      once its new copy is checked, unless -o gives another folder
  --trim              take away what installers do not use: the padding after the data
                      of XCI files, as game cards are dumped, and the delta fragments of
                      updates in NSP and NSZ files; in place, or into the folder of -o
  --fat32             files bigger than FAT32 holds (4 GiB) to a folder of parts with the
                      archive bit, which the console reads as one file; and files in parts
                      (such a folder, or name.ns0, name.ns1...) back to one file
  --rename TEMPLATE   rename files by what they hold: {name}, {id}, {version} (of the newest
                      update, for a game with its updates), {version_txt} (as the game
                      shows it: 1.0.5), {type}, {titles}, {contents} (1G+1U+2D),
                      {size_gb} and {size_mb}; the fields of Switch Library Manager
                      ({TITLE_NAME}, {VERSION_TXT}, {PACKAGE_CONTENTS}...) work too. For
                      example "{name} [{id}][v{version}]"; {dlc_number} is the number of a
                      DLC (1, 2...), for names like "{name} [DLC {dlc_number}] [{id}]"
  --keep-named        with --rename, leave the files whose name has their title ID in
                      brackets, as you named them
  --split             split multi-title files (SuperNSP) into one file per title
  --pack FILE         merge the files given into one multi-title file (SuperNSP): an
                      NSP when they are all NSP, an NSZ when one of them is an NSZ; of a
                      title given twice, the newest version is kept (a SuperNSP and a
                      newer update of its game give the SuperNSP with the new update);
                      with an XCI or XCZ among them, its updates and DLC go into the game
                      card, and the pack is an XCI (or an XCZ)
  --pack-games        pack the files given by game: each game with its updates and DLC in a
                      SuperNSP of its own, next to its files (or in -o); a game with one
                      file only is left as it is
  --pack-name T       the name of each pack of --pack-games, by the fields of --rename
                      (default "{name} [{id}][v{version}] ({contents})")

Multi-title files (SuperNSP) compress like any other file. The options of nsz --long,
--keep, --fix-padding, --alwaysParseCnmt, --rm-old-version, --bs and --depth are accepted
and have no effect.
`

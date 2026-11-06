package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/jobs"
)

// -o with no value, or with an option after it, puts the new files next to their originals as
// in nsz; the other options do not take the option after them as their value.
func TestOptionValueIsNotTheNextOption(t *testing.T) {
	o, err := jobs.ParseOptions([]string{"-C", "-o", "--keys", "K", "a.nsp"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Output != "" || o.Keys != "K" || !reflect.DeepEqual(o.Files, []string{"a.nsp"}) {
		t.Fatalf("-o before an option: output %q, keys %q, files %v", o.Output, o.Keys, o.Files)
	}
	for _, args := range [][]string{{"-C", "a.nsp", "-o"}, {"-C", "--output", "-w", "a.nsp"}, {"-C", "-o", "out", "-o", "--", "a.nsp"}} {
		if o, err := jobs.ParseOptions(args); err != nil || o.Output != "" || !reflect.DeepEqual(o.Files, []string{"a.nsp"}) {
			t.Errorf("%v: output %q, files %v, %v", args, o.Output, o.Files, err)
		}
	}
	// a negative number is a value, and "-" alone a file
	if o, err := jobs.ParseOptions([]string{"-o", "-", "-t", "-1", "a.nsp"}); err != nil || o.Output != "-" || o.Threads != 0 {
		t.Fatalf("values that start with -: %+v %v", o, err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-C", "--keys", "-o", "out"}, "--keys needs a value"},
		{[]string{"-C", "-l", "-w", "a.nsp"}, "-l needs a value"},
		{[]string{"--dat", "--", "a.nsp"}, "--dat needs a value"},
		{[]string{"--rename", "-V"}, "--rename needs a value"},
	} {
		if _, err := jobs.ParseOptions(c.args); err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
}

// The command line is read as argparse reads the one of nsz: letters together, a value right
// after its letter or after "=", and a long option by the start of its name.
func TestOptionsAsArgparse(t *testing.T) {
	for _, c := range []struct {
		args []string
		want func(o jobs.Options) bool
	}{
		{[]string{"-CV"}, func(o jobs.Options) bool { return o.Compress && o.Verify }},
		{[]string{"-Cw"}, func(o jobs.Options) bool { return o.Compress && o.Overwrite }},
		{[]string{"-CBl3"}, func(o jobs.Options) bool { return o.Compress && o.Block && o.Level == 3 }},
		{[]string{"-l18"}, func(o jobs.Options) bool { return o.Level == 18 }},
		{[]string{"-l=12"}, func(o jobs.Options) bool { return o.Level == 12 }},
		{[]string{"-t4"}, func(o jobs.Options) bool { return o.Threads == 4 }},
		{[]string{"-t-1"}, func(o jobs.Options) bool { return o.Threads == 0 }},
		{[]string{"-oDIR"}, func(o jobs.Options) bool { return o.Output == "DIR" }},
		{[]string{"-Co", "out"}, func(o jobs.Options) bool { return o.Compress && o.Output == "out" }},
		{[]string{"-Vo", "-w"}, func(o jobs.Options) bool { return o.Verify && o.Output == "" && o.Overwrite }},
		{[]string{"-m2", "-t", "3"}, func(o jobs.Options) bool { return o.Multi == 2 && o.Threads == 3 }},
		{[]string{"--level=7"}, func(o jobs.Options) bool { return o.Level == 7 }},
		{[]string{"--lev", "5"}, func(o jobs.Options) bool { return o.Level == 5 && !o.Levels }},
		{[]string{"--lev=5"}, func(o jobs.Options) bool { return o.Level == 5 }},
		{[]string{"--levels"}, func(o jobs.Options) bool { return o.Levels && o.Level == jobs.DefaultLevel }},
		{[]string{"--leve", "4"}, func(o jobs.Options) bool { return o.Level == 4 }},
		{[]string{"--ver"}, func(o jobs.Options) bool { return o.Verify }},
		{[]string{"--verify-l", "both"}, func(o jobs.Options) bool { return o.VerifyLevel == "both" }},
		{[]string{"--pack", "x.nsp"}, func(o jobs.Options) bool { return o.Pack == "x.nsp" && !o.PackGames }},
		{[]string{"--pack-g"}, func(o jobs.Options) bool { return o.PackGames }},
		{[]string{"--pack-n", "{name}"}, func(o jobs.Options) bool { return o.PackName == "{name}" }},
		{[]string{"--rm-s"}, func(o jobs.Options) bool { return o.RmSource }},
		{[]string{"--machine"}, func(o jobs.Options) bool { return o.Machine }},
		{[]string{"--min"}, func(o jobs.Options) bool { return o.Minimal }},
		{[]string{"--keep"}, func(o jobs.Options) bool { return reflect.DeepEqual(o.Ignored, []string{"--keep"}) && !o.KeepNamed }},
		{[]string{"--keep-n"}, func(o jobs.Options) bool { return o.KeepNamed }},
		{[]string{"-Ks4", "-L"}, func(o jobs.Options) bool { return reflect.DeepEqual(o.Ignored, []string{"--keep", "--bs", "--long"}) }},
		{[]string{"-h"}, func(o jobs.Options) bool { return o.Help }},
		{[]string{"--he"}, func(o jobs.Options) bool { return o.Help }},
	} {
		o, err := jobs.ParseOptions(append(c.args, "a.nsp"))
		if err != nil || !c.want(o) || !reflect.DeepEqual(o.Files, []string{"a.nsp"}) {
			t.Errorf("%v: %+v %v", c.args, o, err)
		}
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-Cx"}, "unknown option -x"},
		{[]string{"-xC"}, "unknown option -xC"},
		{[]string{"-C=1"}, "-C takes no value"},
		{[]string{"-CV=1"}, "-V takes no value"},
		{[]string{"--verify=1"}, "--verify takes no value"},
		{[]string{"--ver=1"}, "--verify takes no value"},
		{[]string{"--re"}, "ambiguous option --re: it could be --re"},
		{[]string{"--rm"}, "ambiguous option --rm: it could be --rm-source, --rm-dumps, --rm-old-version"},
		{[]string{"--ke"}, "ambiguous option --ke: it could be --keys, --keep-named, --keep"},
		{[]string{"--m"}, "ambiguous option --m"},
		{[]string{"--nothing"}, "unknown option --nothing"},
		{[]string{"-l"}, "-l needs a value"},
		{[]string{"-Cl"}, "-l needs a value"},
		{[]string{"-lx"}, `-l needs a number, not "x"`},
		{[]string{"--lev"}, "--level needs a value"},
	} {
		if _, err := jobs.ParseOptions(c.args); err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
}

// An option that does nothing with the others is told once, and a watch does not go with a
// pack, which would wait for files it never takes.
func TestCheckTellsWhatHasNoEffect(t *testing.T) {
	for args, want := range map[string][]string{
		"--levels -C":                     {"--levels has no effect without --estimate"},
		"--levels --estimate":             nil,
		"--keep-named -C":                 {"--keep-named has no effect without --rename"},
		"--pack-name {name} -C":           {"--pack-name has no effect without --pack-games or --pack-one"},
		"--pack-name {name} --pack-games": nil,
		"--extra -D":                      {"--extra, --sparse and --archive have no effect without -C"},
		"--sparse -V":                     {"--extra, --sparse and --archive have no effect without -C"},
		"--extra -C":                      nil,
		"--archive --estimate":            nil,
		"--rm-dumps --nup":                {"--rm-dumps has no effect without --rm-source"},
		"--rm-dumps --rm-source -C":       {"--rm-dumps has no effect without --nup"},
		"--rm-dumps --rm-source --nup":    nil,
		"--rm-dumps --rm-source --pack p": nil,
		"-o out --rename {name}":          {"-o has no effect with --rename"},
		"-o out -i":                       {"-o has no effect with -i"},
		"-o out --estimate":               {"-o has no effect with --estimate"},
		"-o out -V":                       {"-o has no effect with -V"},
		"-o out --dat d.dat":              {"-o has no effect with -V"},
		"-o out -C":                       nil,
		"-o out --measure":                nil,
		"-o -i":                           nil,
		"-C -l 25":                        {"-l 25: levels above 19 make a zstd window larger than the 8 MiB installers decompress with, so 19 is used"},
	} {
		o, err := jobs.ParseOptions(append(strings.Fields(args), "x"))
		if err == nil {
			err = o.Check()
		}
		if err != nil || !reflect.DeepEqual(o.Notes, want) || len(o.Ignored) > 0 {
			t.Errorf("%s: notes %q, ignored %q, %v", args, o.Notes, o.Ignored, err)
		}
	}
	for _, args := range []string{"--watch --pack p.nsp", "--watch --pack-games"} {
		o, err := jobs.ParseOptions(append(strings.Fields(args), "x"))
		if err == nil {
			err = o.Check()
		}
		if err == nil || !strings.HasPrefix(err.Error(), "--watch does not go with --pack") {
			t.Errorf("%s: %v", args, err)
		}
	}
}

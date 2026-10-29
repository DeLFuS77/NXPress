package main

import (
	"reflect"
	"strings"
	"testing"
)

// -o with no value, or with an option after it, puts the new files next to their originals as
// in nsz; the other options do not take the option after them as their value.
func TestOptionValueIsNotTheNextOption(t *testing.T) {
	o, err := parseOptions([]string{"-C", "-o", "--keys", "K", "a.nsp"})
	if err != nil {
		t.Fatal(err)
	}
	if o.output != "" || o.keys != "K" || !reflect.DeepEqual(o.files, []string{"a.nsp"}) {
		t.Fatalf("-o before an option: output %q, keys %q, files %v", o.output, o.keys, o.files)
	}
	for _, args := range [][]string{{"-C", "a.nsp", "-o"}, {"-C", "--output", "-w", "a.nsp"}, {"-C", "-o", "out", "-o", "--", "a.nsp"}} {
		if o, err := parseOptions(args); err != nil || o.output != "" || !reflect.DeepEqual(o.files, []string{"a.nsp"}) {
			t.Errorf("%v: output %q, files %v, %v", args, o.output, o.files, err)
		}
	}
	// a negative number is a value, and "-" alone a file
	if o, err := parseOptions([]string{"-o", "-", "-t", "-1", "a.nsp"}); err != nil || o.output != "-" || o.threads != 0 {
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
		if _, err := parseOptions(c.args); err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
}

// The command line is read as argparse reads the one of nsz: letters together, a value right
// after its letter or after "=", and a long option by the start of its name.
func TestOptionsAsArgparse(t *testing.T) {
	for _, c := range []struct {
		args []string
		want func(o options) bool
	}{
		{[]string{"-CV"}, func(o options) bool { return o.compress && o.verify }},
		{[]string{"-Cw"}, func(o options) bool { return o.compress && o.overwrite }},
		{[]string{"-CBl3"}, func(o options) bool { return o.compress && o.block && o.level == 3 }},
		{[]string{"-l18"}, func(o options) bool { return o.level == 18 }},
		{[]string{"-l=12"}, func(o options) bool { return o.level == 12 }},
		{[]string{"-t4"}, func(o options) bool { return o.threads == 4 }},
		{[]string{"-t-1"}, func(o options) bool { return o.threads == 0 }},
		{[]string{"-oDIR"}, func(o options) bool { return o.output == "DIR" }},
		{[]string{"-Co", "out"}, func(o options) bool { return o.compress && o.output == "out" }},
		{[]string{"-Vo", "-w"}, func(o options) bool { return o.verify && o.output == "" && o.overwrite }},
		{[]string{"-m2", "-t", "3"}, func(o options) bool { return o.multi == 2 && o.threads == 3 }},
		{[]string{"--level=7"}, func(o options) bool { return o.level == 7 }},
		{[]string{"--lev", "5"}, func(o options) bool { return o.level == 5 && !o.levels }},
		{[]string{"--lev=5"}, func(o options) bool { return o.level == 5 }},
		{[]string{"--levels"}, func(o options) bool { return o.levels && o.level == defaultLevel }},
		{[]string{"--leve", "4"}, func(o options) bool { return o.level == 4 }},
		{[]string{"--ver"}, func(o options) bool { return o.verify }},
		{[]string{"--verify-l", "both"}, func(o options) bool { return o.verifyLevel == "both" }},
		{[]string{"--pack", "x.nsp"}, func(o options) bool { return o.pack == "x.nsp" && !o.packGames }},
		{[]string{"--pack-g"}, func(o options) bool { return o.packGames }},
		{[]string{"--pack-n", "{name}"}, func(o options) bool { return o.packName == "{name}" }},
		{[]string{"--rm-s"}, func(o options) bool { return o.rmSource }},
		{[]string{"--machine"}, func(o options) bool { return o.machine }},
		{[]string{"--min"}, func(o options) bool { return o.minimal }},
		{[]string{"--keep"}, func(o options) bool { return reflect.DeepEqual(o.ignored, []string{"--keep"}) && !o.keepNamed }},
		{[]string{"--keep-n"}, func(o options) bool { return o.keepNamed }},
		{[]string{"-Ks4", "-L"}, func(o options) bool { return reflect.DeepEqual(o.ignored, []string{"--keep", "--bs", "--long"}) }},
		{[]string{"-h"}, func(o options) bool { return o.help }},
		{[]string{"--he"}, func(o options) bool { return o.help }},
	} {
		o, err := parseOptions(append(c.args, "a.nsp"))
		if err != nil || !c.want(o) || !reflect.DeepEqual(o.files, []string{"a.nsp"}) {
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
		{[]string{"--rm"}, "ambiguous option --rm: it could be --rm-source, --rm-old-version"},
		{[]string{"--ke"}, "ambiguous option --ke: it could be --keys, --keep-named, --keep"},
		{[]string{"--m"}, "ambiguous option --m"},
		{[]string{"--nothing"}, "unknown option --nothing"},
		{[]string{"-l"}, "-l needs a value"},
		{[]string{"-Cl"}, "-l needs a value"},
		{[]string{"-lx"}, `-l needs a number, not "x"`},
		{[]string{"--lev"}, "--level needs a value"},
	} {
		if _, err := parseOptions(c.args); err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
}

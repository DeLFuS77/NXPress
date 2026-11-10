package main

import (
	"bytes"
	"strings"
	"testing"
)

// The menu of the Explorer the installer writes has the names of the page, in the language
// chosen; one NXPress has not is English.
func TestMenuTexts(t *testing.T) {
	spanish := menuTexts("es")
	if spanish.Verify != "Verificar" || spanish.Info != "Detalles" || spanish.Compress != "Comprimir" || spanish.Decompress != "Descomprimir" || spanish.Pack != "Juntar" {
		t.Errorf("es: %+v", spanish)
	}
	english := menuTexts("xx")
	if english.Verify != "Verify" || english.Pack != "Combine" {
		t.Errorf("unknown language: %+v", english)
	}
	for _, language := range installerLanguages {
		texts := menuTexts(language)
		if texts.Verify == "" || texts.Info == "" || texts.Compress == "" || texts.Decompress == "" || texts.Pack == "" {
			t.Errorf("%s: %+v", language, texts)
		}
	}
}

// What the installer cannot ask is refused, and nothing is done.
func TestInstallerRefuses(t *testing.T) {
	for _, args := range [][]string{{}, {"install"}, {"add", "shortcuts"}, {"remove", "all"}, {"replace", "a"}} {
		var out, errs bytes.Buffer
		if code := runInstaller(args, &out, &errs); code != 1 || out.Len() != 0 || errs.Len() == 0 {
			t.Errorf("%v: %d, %q, %q", args, code, out.String(), errs.String())
		}
	}
	var out, errs bytes.Buffer
	if code := runInstaller([]string{"add"}, &out, &errs); code != 0 || !strings.Contains(out.String(), "done") {
		t.Errorf("add of nothing: %d, %q", code, errs.String())
	}
}

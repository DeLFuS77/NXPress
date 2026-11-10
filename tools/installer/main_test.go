package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every language of the window has every text of the installer, with the placeholders of the
// English one, and makes a file NSIS reads.
func TestTexts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "build", "windows", "installer", "texts.json"))
	if err != nil {
		t.Fatal(err)
	}
	texts := map[string]map[string]string{}
	if err := json.Unmarshal(data, &texts); err != nil {
		t.Fatal(err)
	}
	if len(texts) != len(languages) {
		t.Errorf("texts.json has %d languages, the window %d", len(texts), len(languages))
	}
	for _, code := range languages {
		for _, key := range keys {
			for placeholder := range placeholders {
				if strings.Contains(texts["en"][key], placeholder) != strings.Contains(texts[code][key], placeholder) {
					t.Errorf("%s %s: %s", code, key, placeholder)
				}
			}
		}
		for key := range texts[code] {
			if key != "nsis" && key != "name" && !contains(keys, key) && !contains(macKeys, key) {
				t.Errorf("%s has %s, which the installer does not use", code, key)
			}
		}
	}
	file, err := languageFile(texts)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`!insertmacro MUI_LANGUAGE "SimpChinese"`, `!insertmacro MUI_LANGUAGE "PortugueseBR"`,
		`LangString UnKeep ${LANG_SPANISH} "Conservar mis ajustes"`, "Function LangCode", "Function LangByName", "!macro FillLanguages"} {
		if !strings.Contains(file, want) {
			t.Errorf("no %s", want)
		}
	}
	// the first language is the one of a Windows NXPress has not
	if !strings.HasPrefix(strings.SplitN(file, "\r\n", 2)[1], `!insertmacro MUI_LANGUAGE "English"`) {
		t.Error("English is not the first")
	}
	delete(texts["fr"], "UnKeep")
	if _, err := languageFile(texts); err == nil {
		t.Error("a text missing was taken")
	}
}

func contains(list []string, item string) bool {
	for _, each := range list {
		if each == item {
			return true
		}
	}
	return false
}

// A text keeps its $ and its quotes, and its lines and placeholders are as NSIS writes them.
func TestNsisText(t *testing.T) {
	got := nsisText("Costs $5, \"really\"\nNXPress {OLD} in {DIR}, now {VERSION}")
	want := `Costs $$5, $\"really$\"$\r$\nNXPress $OldVersion in $INSTDIR, now ${VERSION}`
	if got != want {
		t.Errorf("%s", got)
	}
	if numbers("1.7.8") != "1.7.8.0" || numbers("dev-abc1234") != "0.0.0.0" || numbers("1.7.8-beta.1") != "0.0.0.0" {
		t.Error("numbers")
	}
	if languageName("SIMPCHINESE") != "SimpChinese" || languageName("SPANISH") != "Spanish" {
		t.Error("languageName")
	}
}

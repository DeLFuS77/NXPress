package main

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// the texts, read from the root of the repository as the tool is
func repoTexts(t *testing.T) map[string]map[string]string {
	t.Helper()
	t.Chdir(filepath.Join("..", ".."))
	texts, err := readTexts()
	if err != nil {
		t.Fatal(err)
	}
	return texts
}

// The pages of the Installer of macOS are written for each language, in the folder macOS names
// it by, with the notice on the first one; the license and the background beside them.
func TestMacResources(t *testing.T) {
	texts := repoTexts(t)
	resources := t.TempDir()
	if err := writeMacResources(resources, "1.7.8", texts); err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{"en", "es", "pt_BR", "zh_CN", "ja"} {
		welcome, err := os.ReadFile(filepath.Join(resources, folder+".lproj", "welcome.html"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(welcome), "1.7.8") || !strings.Contains(string(welcome), `class="notice"`) || strings.Contains(string(welcome), "{VERSION}") {
			t.Errorf("%s: %s", folder, welcome)
		}
		if _, err := os.Stat(filepath.Join(resources, folder+".lproj", "conclusion.html")); err != nil {
			t.Error(err)
		}
	}
	spanish, _ := os.ReadFile(filepath.Join(resources, "es.lproj", "welcome.html"))
	if !strings.Contains(string(spanish), "no apoyamos la piratería") {
		t.Error("the notice in Spanish")
	}
	for _, name := range []string{"LICENSE.txt", "background.png"} {
		if _, err := os.Stat(filepath.Join(resources, name)); err != nil {
			t.Error(err)
		}
	}
	var script struct{ XMLName xml.Name }
	if err := xml.Unmarshal([]byte(distributionXML("arm64", "1.7.8")), &script); err != nil || script.XMLName.Local != "installer-gui-script" {
		t.Errorf("distribution: %v", err)
	}
	if !strings.Contains(distributionXML("x86_64", "1.7.8"), `hostArchitectures="x86_64"`) {
		t.Error("the processor")
	}
}

// The uninstaller speaks every language, and its script is one the shell reads.
func TestUninstallScript(t *testing.T) {
	texts := repoTexts(t)
	script := uninstallScript(texts)
	for _, want := range []string{"#!/bin/bash", "MacUnAsk='¿Desinstalar NXPress", "  zh)", "--forget " + bundleID, "with administrator privileges"} {
		if !strings.Contains(script, want) {
			t.Errorf("no %s", want)
		}
	}
	if shellQuote(`It's "here"`) != `'It'\''s \"here\"'` {
		t.Errorf("shellQuote: %s", shellQuote(`It's "here"`))
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	file := filepath.Join(t.TempDir(), "uninstall")
	os.WriteFile(file, []byte(script), 0o755)
	if out, err := exec.Command(bash, "-n", file).CombinedOutput(); err != nil {
		t.Errorf("bash -n: %v %s", err, out)
	}
}

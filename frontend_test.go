package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// frontendChecks runs in node over the folder of the page: it loads the texts as the page does
// and prints one line for each problem it finds.
const frontendChecks = `
"use strict";
const fs = require("fs"), path = require("path"), vm = require("vm");
const dir = process.argv[1];
const problems = [];
const say = text => problems.push(text);

// the texts, loaded in the order of the page, with stand-ins for the sizes and times it formats
const order = ["base", "en", "es", "fr", "de", "it", "pt", "nl", "ru", "ja", "ko", "zh"];
const context = { size: bytes => bytes + " B", time: seconds => seconds + " s" };
vm.createContext(context);
let source = "";
for (const name of order) source += fs.readFileSync(path.join(dir, "i18n", name + ".js"), "utf8") + "\n;\n";
vm.runInContext(source + "this.texts = texts; this.noteRules = noteRules; this.languages = languages;", context);
const { texts, noteRules, languages } = context;
const en = texts.en;
const codes = order.slice(1);
const others = codes.filter(code => code !== "en");

// every language of the menu, with the same texts as English, of the same kind and arguments
if (languages.map(language => language[0]).join() !== codes.join()) say("the menu of languages is " + languages.map(language => language[0]).join());
const braces = text => (text.match(/\{[A-Za-z_]+\}/g) || []).sort().join(" ");
for (const code of codes) {
  const table = texts[code];
  if (!table) { say(code + ": no texts"); continue; }
  for (const key of Object.keys(en)) {
    if (!(key in table)) { say(code + ": no text " + key); continue; }
    if (typeof table[key] !== typeof en[key]) say(code + "." + key + " is a " + typeof table[key] + ", in English a " + typeof en[key]);
    else if (typeof en[key] === "function" && table[key].length !== en[key].length) say(code + "." + key + " takes " + table[key].length + " arguments, in English " + en[key].length);
    else if (typeof en[key] === "string" && braces(table[key]) !== braces(en[key])) say(code + "." + key + " has the fields " + braces(table[key]) + ", in English " + braces(en[key]));
  }
  for (const key of Object.keys(table)) if (!(key in en)) say(code + ": text " + key + " is not in English");
}

// every function answers with text for the arguments the page gives it
const calls = {
  keys: [[""], ["C:\\keys\\prod.keys"]],
  start: ["compress", "decompress", "recompress", "trim", "verify", "convert", "split", "pack", "info", "fat32", "rename"].map(mode => [mode]),
  gameKind: [["BASE"], ["UPD"], ["DLC"]],
  shutdownFailed: [[true], [false]],
};
for (const code of codes) {
  for (const [key, value] of Object.entries(texts[code])) {
    if (typeof value !== "function") continue;
    for (const args of calls[key] || [[1, 1, 1, 1], [2, 0, 1, 0], [5, 3, 2, 1], [21, 4, 21, 4]]) {
      let text;
      try { text = String(value(...args)); } catch (error) { say(code + "." + key + "(" + args.join(", ") + ") fails: " + error.message); continue; }
      if (/undefined|NaN/.test(text)) say(code + "." + key + "(" + args.join(", ") + ") gives " + text);
    }
  }
}

// plurals: [language, text, arguments, what it must say, what it must not]
const plurals = [
  ["en", "lifetime", ["S", 1], "1 file", "1 files"], ["es", "lifetime", ["S", 1], "1 archivo", "1 archivos"],
  ["fr", "lifetime", ["S", 1], "1 fichier", "1 fichiers"], ["de", "lifetime", ["S", 1], "1 Datei", "1 Dateien"],
  ["pt", "lifetime", ["S", 1], "1 arquivo", "1 arquivos"], ["nl", "lifetime", ["S", 1], "1 bestand", "1 bestanden"],
  ["ru", "lifetime", ["S", 1], "1 файле", ""], ["ru", "lifetime", ["S", 2], "2 файлах", ""], ["ru", "lifetime", ["S", 21], "21 файле", ""],
  ["ru", "popTitles", [21, 0, 0], "21 игра", ""], ["ru", "popTitles", [4, 0, 0], "4 игры", ""], ["ru", "popTitles", [11, 0, 0], "11 игр", ""],
  ["ru", "popTitles", [0, 22, 0], "22 обновления", ""], ["ru", "popTitles", [0, 25, 0], "25 обновлений", ""],
  ["ru", "measureIntro", [4, 18, false], "4 процессора", ""], ["ru", "measureIntro", [21, 18, false], "21 процессор,", ""], ["ru", "measureIntro", [8, 18, false], "8 процессоров", ""],
];
for (const [code, key, args, want, unwanted] of plurals) {
  const text = texts[code][key](...args);
  if (!text.includes(want) || unwanted && text.includes(unwanted)) say(code + "." + key + "(" + args.join(", ") + ") gives " + text + ", not " + want);
}

// the notes: each rule has a text in every language but English (which has one where its own
// differs), with the parts the rule finds and no more
const ruleKeys = noteRules.map(rule => rule[1]);
for (const key of ruleKeys.filter((key, i) => ruleKeys.indexOf(key) !== i)) say("the rule " + key + " is there twice");
for (const code of codes) {
  const notes = texts[code].notes || {};
  for (const key of Object.keys(notes)) if (!ruleKeys.includes(key)) say(code + ": note " + key + " has no rule");
  for (const [pattern, key] of noteRules) {
    if (!(key in notes)) { if (code !== "en") say(code + ": no note " + key); continue; }
    if (typeof notes[key] !== "string") continue;
    const parts = new RegExp(pattern.source + "|").exec("").length - 1;
    const used = (notes[key].match(/\$\d/g) || []).map(found => +found[1]);
    for (let part = 1; part <= parts; part++) if (!used.includes(part)) say(code + ".notes." + key + " leaves out $" + part);
    for (const part of used) if (part > parts) say(code + ".notes." + key + " uses $" + part + ", the rule finds " + parts);
  }
}
// the steps of the work have their names in every language that names them
const steps = new Set(others.flatMap(code => Object.keys(texts[code].steps || {})));
for (const code of others) for (const step of steps) if (!(texts[code].steps || {})[step]) say(code + ": no step " + step);

// notes of NXPress come out in the language, as the page shows them: [note, English left in it]
function noteText(note, code) {
  const own = texts[code].notes || {}, english = en.notes || {};
  for (const [pattern, key] of noteRules) {
    const replacement = own[key] !== undefined ? own[key] : english[key];
    if (replacement !== undefined) note = note.replace(pattern, replacement);
  }
  return note;
}
const samples = [
  ["left out of the games to combine", "left out"],
  ["already combined in Game [0100000000010000][v0] (1G+1U).nsp (-w writes it again)", "-w"],
  ["1.0 GB -> 500.0 MB, 50.0% smaller; the original is kept: the new file could only be checked in part", "the original is kept"],
  ["the priority could not be lowered: access denied", "could not be lowered"],
  ["no file to verify among the ones given", "among the ones given"],
  ["the file is empty", "the file is empty"],
  ["Game.nsp would get the same name as Game (2).nsp", "would get the same name"],
  ["about 8.0 GB -> 5.1 GB, 36.2% smaller, 1 h 5 min at level 18", " at level "],
  ["about 8.0 GB -> 5.1 GB, 36.2% smaller, 40 s at level 3", " at level "],
  ["8.0 MB -> 42.3 KB, 99.5% smaller", " -> "],
];
// in English too: what the window says in its own words
const ownEnglish = ["-w", " -> "];
for (const [note, english] of samples) {
  for (const code of codes) {
    const text = noteText(note, code);
    if (/\$\d/.test(text)) say(code + ": " + note + " gives " + text);
    if ((code !== "en" || ownEnglish.includes(english)) && text.includes(english)) say(code + ": " + note + " keeps " + english + ": " + text);
  }
}
// the decimal mark of the language, as the page writes its own sizes
for (const code of ["es", "fr", "de", "it", "pt", "nl", "ru"]) {
  const text = noteText("8.0 MB -> 42.3 KB, 99.5% smaller", code);
  if (!text.includes("8,0 MB") || !text.includes("42,3 KB") || !text.includes("99,5%")) say(code + ": the figures of a note keep their point: " + text);
}

// the texts the page asks for by name are there
const page = fs.readFileSync(path.join(dir, "app.js"), "utf8") + fs.readFileSync(path.join(dir, "index.html"), "utf8");
for (const found of page.matchAll(/\bt\("([A-Za-z0-9]+)"\)|data-t(?:-title|-label)?="([A-Za-z0-9]+)"/g)) {
  const key = found[1] || found[2];
  if (!(key in en)) say("the page asks for " + key + ", which has no text");
}

console.log(problems.join("\n"));
`

// The page and its texts: every script of it reads, the 11 languages have the same texts with
// the same arguments and fields, the plurals agree, each note rule has its text with the parts it
// finds, the notes of NXPress come out translated, and the texts the page asks for are there.
func TestFrontendTexts(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	scripts, _ := filepath.Glob(filepath.Join("frontend", "*.js"))
	texts, _ := filepath.Glob(filepath.Join("frontend", "i18n", "*.js"))
	for _, script := range append(scripts, texts...) {
		if out, err := exec.Command(node, "--check", script).CombinedOutput(); err != nil {
			t.Errorf("%s does not read: %s", script, out)
		}
	}
	out, err := exec.Command(node, "-e", frontendChecks, "frontend").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, problem := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if problem != "" {
			t.Error(problem)
		}
	}
}

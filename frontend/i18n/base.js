"use strict";

// the texts of the window, by language: each file of this folder adds one
const texts = {};

// the languages, in the order of the menu, by their own name
const languages = [
  ["en", "English"], ["es", "Español"], ["fr", "Français"], ["de", "Deutsch"], ["it", "Italiano"], ["pt", "Português"],
  ["nl", "Nederlands"], ["ru", "Русский"], ["ja", "日本語"], ["ko", "한국어"], ["zh", "中文"],
];

// The notes of the results come from NXPress in English: each rule finds a part of one, and
// each language gives the text for it under the name of the rule ($1, $2 for what it found).
// A part a language has no text for is left in English.
const noteRules = [
  [/ is already there \(use -w to replace it\)/g, "alreadyThere"],
  [/^nothing in it can be compressed, it is left as it is$/, "notCompressible"],
  [/^nothing in it can be compressed$/, "nothingToCompress"],
  [/^it holds one title, nothing to split$/, "oneTitle"],
  [/^nothing to trim, it ends where its data ends$/, "trimNoPadding"],
  [/^nothing to trim, it holds no delta fragments$/, "trimNoDeltas"],
  [/data follows the last partition: it is not padding, the XCI is left as it is/g, "notPadding"],
  [/^it fits in FAT32 as it is$/, "fitsFat32"],
  [/^(\d+) parts in a folder/, "partsFolder"],
  [/; set its archive bit on the microSD \(Hekate: Tools, Archive bit\)/g, "archiveBit"],
  [/^(\d+) parts joined$/, "partsJoined"],
  [/^renamed to /, "renamedTo"],
  [/^the name is already that one$/, "sameName"],
  [/^checked on (\S+), not changed since/, "checkedBefore"],
  [/^(\d+) levels compared$/, "levelsCompared"],
  [/level (\d+): about /g, "levelAbout"],
  [/^watching for new files$/, "watchingLog"],
  [/not in the DAT: no dump of its size/g, "datNoSize"],
  [/not in the DAT: (\d+) dumps of its size, with other hashes/g, "datOther"],
  [/in the DAT: /g, "datIn"],
  [/works with: every installer \(DBI, Tinfoil, Awoo, CyberFoil, Sphaira\); emulators need it decompressed/g, "worksSolid"],
  [/works with: every installer but Awoo; emulators that read NSZ run it as it is/g, "worksBlocks"],
  [/works with: every installer and emulator/g, "worksAll"],
  [/padding: (.+?) that trimming takes away/g, "paddingInfo"],
  [/delta fragments: (.+?) that trimming takes away/g, "deltasInfo"],
  [/ of padding taken away/g, "paddingTaken"],
  [/ of delta fragments taken away/g, "deltasTaken"],
  [/ of delta fragments left out/g, "deltasLeft"],
  [/, newer versions in place of /g, "newerInPlace"],
  [/(\d+) NCA rewritten by a tool, their data checked/g, "ncaRewritten"],
  [/; some of its files are in no part, so it is kept/g, "unclaimed"],
  [/^(\d+) files merged/, "filesMerged"],
  [/^(\d+) files: /, "filesSplit"],
  [/^1 title$/, "oneTitleCount"],
  [/^(\d+) titles$/, "titlesCount"],
  [/^about /, "about"],
  [/, (\S+ \S+) at level (\d+)$/, "atLevel"],
  [/, solid and in blocks: /g, "solidAndBlocks"],
  [/, solid: /g, "solidColon"],
  [/, in blocks: /g, "blocksColon"],
  [/, uncompressed: /g, "uncompressedColon"],
  [/ uncompressed \(/g, "uncompressedParen"],
  [/^solid, /, "solidComma"],
  [/^in blocks, /, "blocksComma"],
  [/ smaller/g, "smaller"],
];

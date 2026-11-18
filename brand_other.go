//go:build !official

package main

import _ "embed"

// A program built from the code of NXPress without the tag "official", which only the workflow of
// its repository uses: it does not carry the name and the logo of NXPress (TRADEMARKS.md), and its
// window says it is not an official version.

const officialBuild = false

// brandName is the name the window has.
const brandName = "NXPress (unofficial build)"

// a plain icon, with nothing of the logo of NXPress
//
//go:embed build/unofficial.png
var appIcon []byte

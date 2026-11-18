//go:build official

package main

import _ "embed"

// The name and the icon of NXPress are for its official versions only (TRADEMARKS.md): the
// workflow of its repository builds them with the tag "official". A program built from the code
// without it has another name and a plain icon (brand_other.go).

const officialBuild = true

// brandName is the name the window has.
const brandName = "NXPress"

// the icon of the window on Linux; Windows takes it from the resources of the program, macOS
// from its app
//
//go:embed build/appicon.png
var appIcon []byte

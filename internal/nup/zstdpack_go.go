//go:build !cgo

package nup

// nativeZstd tells NXPress is built with the zstd library in C; this build has the encoder in
// Go only.
const nativeZstd = false

func newNativeWhole(level int) wholeEncoder {
	panic("NXPress is built without the zstd library in C")
}

func newNativeStream(level int, threads int) streamEncoder {
	panic("NXPress is built without the zstd library in C")
}

//go:build cgo

package native

/*
#include <stddef.h>
#include <stdint.h>
*/
import "C"

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"fmt"
	"runtime/cgo"
	"unsafe"
)

// What the code of NXDock calls (bridge.h), from the thread that called it or from the threads it
// packs on.

func bytesAt(p *C.uint8_t, n C.size_t) []byte {
	if n == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n))
}

//export nxoGoRead
func nxoGoRead(handle C.uintptr_t, which C.int, offset C.uint64_t, buf *C.uint8_t, size C.size_t) C.int {
	job := cgo.Handle(handle).Value().(*Job)
	if int(which) < 0 || int(which) >= len(job.NCAs) {
		job.fail(fmt.Errorf("no NCA %d", which))
		return 0
	}
	p := bytesAt(buf, size)
	n, err := job.NCAs[which].ReadAt(p, int64(offset))
	if n != len(p) {
		if err == nil {
			err = fmt.Errorf("short read")
		}
		job.fail(fmt.Errorf("reading an NCA at %d: %w", uint64(offset), err))
		return 0
	}
	return 1
}

//export nxoGoCtr
func nxoGoCtr(key *C.uint8_t, ctr *C.uint8_t, data *C.uint8_t, size C.size_t) {
	block, err := aes.NewCipher(bytesAt(key, 16))
	if err != nil {
		panic(err)
	}
	iv := make([]byte, 16)
	copy(iv, bytesAt(ctr, 16))
	p := bytesAt(data, size)
	cipher.NewCTR(block, iv).XORKeyStream(p, p)
}

//export nxoGoSha256
func nxoGoSha256(data *C.uint8_t, size C.size_t, out *C.uint8_t) {
	sum := sha256.Sum256(bytesAt(data, size))
	copy(bytesAt(out, 32), sum[:])
}

//export nxoGoHeader
func nxoGoHeader(handle C.uintptr_t, header *C.uint8_t, size C.size_t) {
	cgo.Handle(handle).Value().(*Job).EncryptHeader(bytesAt(header, size))
}

//export nxoGoOut
func nxoGoOut(handle C.uintptr_t, data *C.uint8_t, size C.size_t) C.int {
	job := cgo.Handle(handle).Value().(*Job)
	if err := job.Out(bytesAt(data, size)); err != nil {
		job.fail(err)
		return 0
	}
	return 1
}

//export nxoGoStep
func nxoGoStep(handle C.uintptr_t, phase C.int, done C.uint64_t) C.int {
	job := cgo.Handle(handle).Value().(*Job)
	if job.Step != nil && !job.Step(int(phase), uint64(done)) {
		return 0
	}
	return 1
}

// SPDX-License-Identifier: AGPL-3.0-only

// gen is the hand-rolled wasm emitter for the plugin test fixtures
// (task 33). It encodes the wasm binary format directly — LEB128, sections
// and code bodies — so no assembler toolchain is needed and CI never gains
// one. Run it manually to (re)generate the six fixture modules:
//
//	go run ./internal/plugin/testdata/gen/main.go internal/plugin/testdata
//
// Fixtures produced (each under 2 KB):
//
//	hello.wasm    reads stdin, prints "hello from wasm" (no trailing
//	              newline: the capture flush is part of the contract)
//	rewrite.wasm  finds file_path in the stdin JSON, opens the file
//	              through the preopen, overwrites it in place
//	escape.wasm   tries to open /etc/passwd and reports the errno
//	badfile.wasm  prints {"file": "/etc/passwd"} — the handover protocol
//	              naming a path outside scratch
//	spin.wasm     loops forever (the deadline test; no imports, no memory)
//	oom.wasm      declares a memory max above any runtime ceiling
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// ------------------------------------------------------------- encoding

// uleb encodes v as an unsigned LEB128 integer.
func uleb(v uint64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			return out
		}
	}
}

// u32 is uleb for a wasm u32 immediate.
func u32(v uint32) []byte { return uleb(uint64(v)) }

// sleb encodes v as a signed LEB128 integer.
func sleb(v int64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		sign := b&0x40 != 0
		if (v == 0 && !sign) || (v == -1 && sign) {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

// i32 is sleb for a wasm i32 immediate.
func i32(v int32) []byte { return sleb(int64(v)) }

// i64 is sleb for a wasm i64 immediate.
func i64(v int64) []byte { return sleb(v) }

// name encodes a length-prefixed wasm name.
func name(s string) []byte {
	b := []byte(s)
	return append(u32(uint32(len(b))), b...)
}

// vec wraps items as a wasm vector: count followed by the items.
func vec(items ...[]byte) []byte {
	out := u32(uint32(len(items)))
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}

// memarg emits the align+offset immediates of a load or store.
func memarg(align, offset uint32) []byte {
	return append(u32(align), u32(offset)...)
}

// sec builds one section: id, size, payload.
func sec(id byte, payload []byte) []byte {
	return append([]byte{id}, append(uleb(uint64(len(payload))), payload...)...)
}

// ------------------------------------------------------------- opcodes

const (
	opBlock     = 0x02
	opLoop      = 0x03
	opIf        = 0x04
	opEnd       = 0x0b
	opBr        = 0x0c
	opBrIf      = 0x0d
	opReturn    = 0x0f
	opCall      = 0x10
	opDrop      = 0x1a
	opLocalGet  = 0x20
	opLocalSet  = 0x21
	opLocalTee  = 0x22
	opI32Load   = 0x28
	opI64Load   = 0x29
	opI32Load8U = 0x2d
	opI32Store  = 0x36
	opI32Store8 = 0x3a
	opI32Const  = 0x41
	opI64Const  = 0x42
	opI32Eqz    = 0x45
	opI32Eq     = 0x46
	opI32GtS    = 0x4a
	opI32GeS    = 0x4e
	opI64Eq     = 0x51
	opI32Add    = 0x6a
	opI32Sub    = 0x6b
	opI32DivU   = 0x6e
	opI32RemU   = 0x70
)

// valtype is a wasm value type byte.
const (
	vtI32 byte = 0x7f
	vtI64 byte = 0x7e
)

// fnType encodes a function type: 0x60, params, results.
func fnType(params, results []byte) []byte {
	count := func(ts []byte) []byte { return u32(uint32(len(ts))) }
	out := []byte{0x60}
	out = append(out, count(params)...)
	out = append(out, params...)
	out = append(out, count(results)...)
	out = append(out, results...)
	return out
}

// ------------------------------------------------------------- module

// module assembles a wasm binary. Sections are emitted in the fixed order
// the format requires: type, import, function, memory, export, start,
// code, data.
type module struct {
	types     [][]byte
	imports   [][]byte
	funcs     [][]byte // type indices of the local functions
	exports   [][]byte
	start     []byte // start-section payload (the function index), optional
	bodies    [][]byte
	memMin    uint32
	memMax    uint32
	memCapped bool
	data      [][]byte
	nextFunc  uint32 // next function index (imports take the first ones)
}

// importWasi registers a wasi_snapshot_preview1 import and returns the
// function index the bodies must call.
func (m *module) importWasi(f string, typeIdx uint32) uint32 {
	idx := m.nextFunc
	entry := append(name("wasi_snapshot_preview1"), name(f)...)
	entry = append(entry, 0x00) // import kind: function
	entry = append(entry, u32(typeIdx)...)
	m.imports = append(m.imports, entry)
	m.nextFunc++
	return idx
}

// addFunc registers a local function (with n i32 locals) and returns its
// function index. The code entry carries the locals declaration ahead of
// the body.
func (m *module) addFunc(typeIdx uint32, nLocals uint32, body []byte) uint32 {
	idx := m.nextFunc
	m.funcs = append(m.funcs, u32(typeIdx))
	m.bodies = append(m.bodies, append(localsDecl(nLocals), body...))
	m.nextFunc++
	return idx
}

// localsDecl encodes the locals vector of a code entry: no groups, or one
// group of n i32 locals (enough for these fixtures).
func localsDecl(n uint32) []byte {
	if n == 0 {
		return u32(0)
	}
	return append(u32(1), append(u32(n), vtI32)...)
}

// addStart exports nm as the module's entry point. The export alone is
// what wazero runs: an additional start section would execute the body a
// second time (the double-run bug that doubled every fixture's output),
// so no start section is emitted.
func (m *module) addStart(typeIdx uint32, nLocals uint32, body []byte, nm string) {
	idx := m.addFunc(typeIdx, nLocals, body)
	m.exports = append(m.exports, append(name(nm), append([]byte{0x00}, u32(idx)...)...))
}

// addData appends an active data segment at offset. Flags 0x00 means
// memory 0 is implicit: offset expression, then the byte vector.
func (m *module) addData(offset uint32, b []byte) {
	seg := append([]byte{0x00}, opI32Const)
	seg = append(seg, i32(int32(offset))...)
	seg = append(seg, opEnd)
	seg = append(seg, u32(uint32(len(b)))...)
	seg = append(seg, b...)
	m.data = append(m.data, seg)
}

// build returns the full binary.
func (m *module) build() []byte {
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	if len(m.types) > 0 {
		out = append(out, sec(1, vec(m.types...))...)
	}
	if len(m.imports) > 0 {
		out = append(out, sec(2, vec(m.imports...))...)
	}
	if len(m.funcs) > 0 {
		out = append(out, sec(3, vec(m.funcs...))...)
	}
	if m.memCapped || m.memMin > 0 {
		entry := append([]byte{0x01}, append(u32(m.memMin), u32(m.memMax)...)...)
		out = append(out, sec(5, vec(entry))...)
	}
	if len(m.exports) > 0 {
		out = append(out, sec(7, vec(m.exports...))...)
	}
	if m.start != nil {
		out = append(out, sec(8, m.start)...)
	}
	if len(m.bodies) > 0 {
		wrapped := make([][]byte, len(m.bodies))
		for i, b := range m.bodies {
			// Each code entry: size, locals declaration, body.
			wrapped[i] = append(u32(uint32(len(b))), b...)
		}
		out = append(out, sec(10, vec(wrapped...))...)
	}
	if len(m.data) > 0 {
		out = append(out, sec(11, vec(m.data...))...)
	}
	return out
}

// ------------------------------------------------------------- fixtures

// funcTypes is the shared type section:
//
//	0: () -> ()                                       _start
//	1: (i32 i32 i32 i32) -> (i32)                     fd_read, fd_write
//	2: (i32 i32 i32 i32 i32 i64 i64 i32 i32) -> (i32) path_open
func funcTypes() [][]byte {
	return [][]byte{
		fnType(nil, nil),
		fnType([]byte{vtI32, vtI32, vtI32, vtI32}, []byte{vtI32}),
		fnType([]byte{vtI32, vtI32, vtI32, vtI32, vtI32, vtI64, vtI64, vtI32, vtI32}, []byte{vtI32}),
	}
}

// rightsAll is the WASI rights bitmask (everything).
const rightsAll = int64(0x1ffffffffff)

// emit writes one fixture under dir, refusing fixtures over 2 KB.
func emit(dir, file string, m *module) {
	raw := m.build()
	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write", path, err)
		os.Exit(1)
	}
	if len(raw) > 2048 {
		fmt.Fprintln(os.Stderr, file, "is", len(raw), "bytes; fixtures stay under 2 KB")
		os.Exit(1)
	}
	fmt.Printf("%-14s %5d bytes\n", file, len(raw))
}

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	emit(dir, "hello.wasm", hello())
	emit(dir, "rewrite.wasm", rewrite())
	emit(dir, "escape.wasm", escape())
	emit(dir, "badfile.wasm", badfile())
	emit(dir, "spin.wasm", spin())
	emit(dir, "oom.wasm", oom())
}

// push emits i32.const v.
func push(v int32) []byte { return append([]byte{opI32Const}, i32(v)...) }

// push64 emits i64.const v.
func push64(v int64) []byte { return append([]byte{opI64Const}, i64(v)...) }

// call emits call fn.
func call(fn uint32) []byte { return append([]byte{opCall}, u32(fn)...) }

// hello reads up to 1 KiB of stdin, prints "hello from wasm" (without a
// trailing newline, so the capture's flush is part of the contract), and
// returns: the start function returning is WASI exit 0.
func hello() *module {
	m := &module{types: funcTypes()}
	fdRead := m.importWasi("fd_read", 1)
	fdWrite := m.importWasi("fd_write", 1)

	// Memory layout:
	//   0x0000 nread (fd_read result)
	//   0x0004 stdin iovec {0x1000, 1024}
	//   0x0020 stdout iovec {0x3000, 15}
	//   0x0028 nwritten (fd_write result)
	//   0x1000 stdin buffer (1024 bytes)
	//   0x3000 "hello from wasm" (15 bytes)
	var b []byte
	b = append(b, push(0)...)    // fd 0 (stdin)
	b = append(b, push(0x04)...) // iovs
	b = append(b, push(1)...)    // iovs_len
	b = append(b, push(0x00)...) // nwritten ptr
	b = append(b, call(fdRead)...)
	b = append(b, opDrop)
	b = append(b, push(1)...)    // fd 1 (stdout)
	b = append(b, push(0x20)...) // iovs
	b = append(b, push(1)...)    // iovs_len
	b = append(b, push(0x28)...) // nwritten ptr
	b = append(b, call(fdWrite)...)
	b = append(b, opDrop)
	b = append(b, opEnd)
	m.addStart(0, 0, b, "_start")

	readIov := make([]byte, 8)
	binary.LittleEndian.PutUint32(readIov[0:], 0x1000)
	binary.LittleEndian.PutUint32(readIov[4:], 1024)
	m.addData(0x04, readIov)
	writeIov := make([]byte, 8)
	binary.LittleEndian.PutUint32(writeIov[0:], 0x3000)
	binary.LittleEndian.PutUint32(writeIov[4:], 15)
	m.addData(0x20, writeIov)
	m.addData(0x3000, []byte("hello from wasm"))
	m.memMin, m.memMax, m.memCapped = 1, 16, true
	return m
}

// rewrite locates the "file_path":" value in the stdin JSON, opens that
// file through the preopen (fd 3), overwrites it in place and returns. It
// prints nothing: an in-place rewrite hands the original path down the
// chain (the task 32 protocol).
func rewrite() *module {
	m := &module{types: funcTypes()}
	fdRead := m.importWasi("fd_read", 1)
	fdWrite := m.importWasi("fd_write", 1)
	pathOpen := m.importWasi("path_open", 2)

	// The needle "file_path":" is matched with two overlapping i64 loads:
	// bytes 0..8 and bytes 5..13 of the needle, so 13 bytes in 4 compares.
	needle := []byte(`"file_path":"`)
	hi := int64(binary.LittleEndian.Uint64(needle[0:8]))
	lo := int64(binary.LittleEndian.Uint64(needle[5:13]))

	// Memory layout:
	//   0x0000 nread
	//   0x0004 stdin iovec {0x1000, 4096}
	//   0x002c nwritten (file write)
	//   0x0030 file iovec {0x3000, 14}
	//   0x0040 opened fd (path_open result)
	//   0x1000 stdin buffer (4096 bytes)
	//   0x3000 "WASM WAS HERE\n"
	// Locals: 0 scan index / start ptr, 1 plen, 2 fd.
	var b []byte
	b = append(b, push(0)...)
	b = append(b, push(0x04)...)
	b = append(b, push(1)...)
	b = append(b, push(0x00)...)
	b = append(b, call(fdRead)...)
	b = append(b, opDrop)

	// i = 0
	b = append(b, push(0)...)
	b = append(b, opLocalSet, 0)

	// block            ; label 2 from inside the loop
	//   block          ; label 1
	//     loop         ; label 0
	//       ...
	b = append(b, opBlock, 0x40)
	b = append(b, opBlock, 0x40)
	b = append(b, opLoop, 0x40)

	// if i > nread-13 (signed): the needle is absent → br 2 (skip all)
	b = append(b, opLocalGet, 0)
	b = append(b, push(0)...)
	b = append(b, opI32Load)
	b = append(b, memarg(2, 0)...)
	b = append(b, push(13)...)
	b = append(b, opI32Sub)
	b = append(b, opI32GtS)
	b = append(b, opBrIf, 2)

	// p = 0x1000 + i
	b = append(b, opLocalGet, 0)
	b = append(b, push(0x1000)...)
	b = append(b, opI32Add)
	b = append(b, opLocalSet, 1)

	// i64.load(p) == hi? else advance and retry. The increment must run
	// only on the mismatch path (inside an if), so the next compare sees
	// the next position; without it the loop never moves and the module
	// spins until the deadline kills it.
	b = append(b, opLocalGet, 1)
	b = append(b, opI64Load)
	b = append(b, memarg(3, 0)...)
	b = append(b, push64(hi)...)
	b = append(b, opI64Eq)
	b = append(b, opI32Eqz)
	b = append(b, opIf, 0x40) // if mismatch
	b = append(b, opLocalGet, 0)
	b = append(b, push(1)...)
	b = append(b, opI32Add)
	b = append(b, opLocalSet, 0)
	b = append(b, opBr, 1) // continue the loop (label 1 inside the if)
	b = append(b, opEnd)

	// i64.load(p+5) == lo? else advance and retry.
	b = append(b, opLocalGet, 1)
	b = append(b, push(5)...)
	b = append(b, opI32Add)
	b = append(b, opI64Load)
	b = append(b, memarg(3, 0)...)
	b = append(b, push64(lo)...)
	b = append(b, opI64Eq)
	b = append(b, opI32Eqz)
	b = append(b, opIf, 0x40) // if mismatch
	b = append(b, opLocalGet, 0)
	b = append(b, push(1)...)
	b = append(b, opI32Add)
	b = append(b, opLocalSet, 0)
	b = append(b, opBr, 1) // continue the loop
	b = append(b, opEnd)

	// found: start = p + 13, br 1 (continue after the loop)
	b = append(b, opLocalGet, 1)
	b = append(b, push(13)...)
	b = append(b, opI32Add)
	b = append(b, opLocalSet, 0)
	b = append(b, opBr, 1)

	b = append(b, opEnd) // loop
	b = append(b, opEnd) // inner block
	b = append(b, opEnd) // outer block

	// plen = 0, then scan until '"' (bounded at 128 so a malformed JSON
	// cannot run off the buffer)
	b = append(b, push(0)...)
	b = append(b, opLocalSet, 1)
	b = append(b, opBlock, 0x40)
	b = append(b, opLoop, 0x40)
	// if plen >= 128: bail (br 1) — the path is absurdly long; the open
	// below then uses whatever length was found. The test's JSON is sane.
	b = append(b, opLocalGet, 1)
	b = append(b, push(128)...)
	b = append(b, opI32GeS)
	b = append(b, opBrIf, 1)
	b = append(b, opLocalGet, 0)
	b = append(b, opLocalGet, 1)
	b = append(b, opI32Add)
	b = append(b, opI32Load8U)
	b = append(b, memarg(0, 0)...)
	b = append(b, push(int32('"'))...)
	b = append(b, opI32Eq)
	b = append(b, opBrIf, 1)
	b = append(b, opLocalGet, 1)
	b = append(b, push(1)...)
	b = append(b, opI32Add)
	b = append(b, opLocalSet, 1)
	b = append(b, opBr, 0)
	b = append(b, opEnd) // loop
	b = append(b, opEnd) // block

	// path_open(3, 0, start, plen, 0, rightsAll, rightsAll, 0, 0x40)
	b = append(b, push(3)...)
	b = append(b, push(0)...)
	b = append(b, opLocalGet, 0)
	b = append(b, opLocalGet, 1)
	b = append(b, push(0)...)
	b = append(b, push64(rightsAll)...)
	b = append(b, push64(rightsAll)...)
	b = append(b, push(0)...)
	b = append(b, push(0x40)...)
	b = append(b, call(pathOpen)...)
	b = append(b, opDrop)

	// fd = *0x40
	b = append(b, push(0x40)...)
	b = append(b, opI32Load)
	b = append(b, memarg(2, 0)...)
	b = append(b, opLocalSet, 2)

	// fd_write(fd, 0x30, 1, 0x2c)
	b = append(b, opLocalGet, 2)
	b = append(b, push(0x30)...)
	b = append(b, push(1)...)
	b = append(b, push(0x2c)...)
	b = append(b, call(fdWrite)...)
	b = append(b, opDrop)
	b = append(b, opEnd)
	m.addStart(0, 3, b, "_start")

	readIov := make([]byte, 8)
	binary.LittleEndian.PutUint32(readIov[0:], 0x1000)
	binary.LittleEndian.PutUint32(readIov[4:], 4096)
	m.addData(0x04, readIov)
	fileIov := make([]byte, 8)
	binary.LittleEndian.PutUint32(fileIov[0:], 0x3000)
	binary.LittleEndian.PutUint32(fileIov[4:], 14)
	m.addData(0x30, fileIov)
	m.addData(0x3000, []byte("WASM WAS HERE\n"))
	m.memMin, m.memMax, m.memCapped = 1, 16, true
	return m
}

// escape tries to open /etc/passwd through the preopen and prints the
// errno as decimal. errno 0 would mean it opened — the test asserts the
// number is not 0.
func escape() *module {
	m := &module{types: funcTypes()}
	fdWrite := m.importWasi("fd_write", 1)
	pathOpen := m.importWasi("path_open", 2)

	// Memory layout:
	//   0x0010 stdout iovecs {0x3100,24} {runtime} {0x3300,1}
	//   0x0028 nwritten
	//   0x0040 opened fd (path_open result)
	//   0x3000 "/etc/passwd"
	//   0x3100 "open /etc/passwd: errno="
	//   0x3200..0x3210 decimal digits (written backwards)
	//   0x3300 "\n"
	// Locals: 0 errno, 1 end ptr, 2 digit.
	var b []byte
	// errno = path_open(3, 0, "/etc/passwd", 11, 0, all, all, 0, 0x40)
	b = append(b, push(3)...)
	b = append(b, push(0)...)
	b = append(b, push(0x3000)...)
	b = append(b, push(11)...)
	b = append(b, push(0)...)
	b = append(b, push64(rightsAll)...)
	b = append(b, push64(rightsAll)...)
	b = append(b, push(0)...)
	b = append(b, push(0x40)...)
	b = append(b, call(pathOpen)...)
	b = append(b, opLocalSet, 0)

	// end = 0x3210
	b = append(b, push(0x3210)...)
	b = append(b, opLocalSet, 1)
	b = append(b, opBlock, 0x40)
	b = append(b, opLoop, 0x40)
	// digit = errno % 10
	b = append(b, opLocalGet, 0)
	b = append(b, push(10)...)
	b = append(b, opI32RemU)
	b = append(b, opLocalSet, 2)
	// errno = errno / 10
	b = append(b, opLocalGet, 0)
	b = append(b, push(10)...)
	b = append(b, opI32DivU)
	b = append(b, opLocalSet, 0)
	// store8(--end, '0' + digit)
	b = append(b, opLocalGet, 1)
	b = append(b, push(1)...)
	b = append(b, opI32Sub)
	b = append(b, opLocalTee, 1)
	b = append(b, opLocalGet, 2)
	b = append(b, push('0')...)
	b = append(b, opI32Add)
	b = append(b, opI32Store8)
	b = append(b, memarg(0, 0)...)
	// while errno != 0
	b = append(b, opLocalGet, 0)
	b = append(b, opI32Eqz)
	b = append(b, opBrIf, 1)
	b = append(b, opBr, 0)
	b = append(b, opEnd) // loop
	b = append(b, opEnd) // block

	// iovec[1] = {end, 0x3210 - end}: the address first, then the value
	// (i32.store pops value, then address).
	b = append(b, push(0x18)...)
	b = append(b, opLocalGet, 1)
	b = append(b, opI32Store)
	b = append(b, memarg(2, 0)...)
	b = append(b, push(0x1c)...)
	b = append(b, push(0x3210)...)
	b = append(b, opLocalGet, 1)
	b = append(b, opI32Sub)
	b = append(b, opI32Store)
	b = append(b, memarg(2, 0)...)

	// fd_write(1, 0x10, 3, 0x28)
	b = append(b, push(1)...)
	b = append(b, push(0x10)...)
	b = append(b, push(3)...)
	b = append(b, push(0x28)...)
	b = append(b, call(fdWrite)...)
	b = append(b, opDrop)
	b = append(b, opEnd)
	m.addStart(0, 3, b, "_start")

	stdIov := make([]byte, 24)
	binary.LittleEndian.PutUint32(stdIov[0:], 0x3100)
	binary.LittleEndian.PutUint32(stdIov[4:], 24)
	// [8:] and [12:] are the digits' ptr/len, written at run time
	binary.LittleEndian.PutUint32(stdIov[16:], 0x3300)
	binary.LittleEndian.PutUint32(stdIov[20:], 1)
	m.addData(0x10, stdIov)
	m.addData(0x3000, []byte("/etc/passwd"))
	m.addData(0x3100, []byte("open /etc/passwd: errno="))
	m.addData(0x3300, []byte("\n"))
	m.memMin, m.memMax, m.memCapped = 1, 16, true
	return m
}

// badfile prints the handover result line naming a path outside scratch:
// the runner must reject it with the path in the message.
func badfile() *module {
	m := &module{types: funcTypes()}
	fdWrite := m.importWasi("fd_write", 1)

	// Memory layout: 0x0010 iovec {0x3000, 24}, 0x0018 nwritten,
	// 0x3000 {"file": "/etc/passwd"}\n (24 bytes).
	var b []byte
	b = append(b, push(1)...)
	b = append(b, push(0x10)...)
	b = append(b, push(1)...)
	b = append(b, push(0x18)...)
	b = append(b, call(fdWrite)...)
	b = append(b, opDrop)
	b = append(b, opEnd)
	m.addStart(0, 0, b, "_start")

	iov := make([]byte, 8)
	binary.LittleEndian.PutUint32(iov[0:], 0x3000)
	binary.LittleEndian.PutUint32(iov[4:], 24)
	m.addData(0x10, iov)
	m.addData(0x3000, []byte(`{"file": "/etc/passwd"}`+"\n"))
	m.memMin, m.memMax, m.memCapped = 1, 16, true
	return m
}

// spin loops forever. No imports and no memory: only the runtime's
// context-aware interruption (WithCloseOnContextDone) can stop it.
func spin() *module {
	m := &module{types: funcTypes()}
	// loop br 0 end / end
	b := []byte{opLoop, 0x40, opBr, 0, opEnd, opEnd}
	m.addStart(0, 0, b, "_start")
	return m
}

// oom declares a memory max above every runtime ceiling wazero accepts
// (65536 pages = 4 GiB): compiling the module is then rejected with the
// "over limit" error text, which is the memory-limit failure the task
// expects. A declared max below the ceiling is clamped to the runtime
// limit by wazero's sizer instead of being rejected.
func oom() *module {
	m := &module{types: funcTypes()}
	b := []byte{opEnd}
	m.addStart(0, 0, b, "_start")
	m.memMin, m.memMax, m.memCapped = 1, 65537, true
	return m
}

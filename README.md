<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-msgpack/brand/main/social/go-ruby-msgpack-msgpack.png" alt="go-ruby-msgpack/msgpack" width="720"></p>

# msgpack — go-ruby-msgpack

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-msgpack.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of Ruby's [`msgpack`](https://github.com/msgpack/msgpack-ruby)
gem** — the deterministic, interpreter-independent MessagePack codec for the Ruby
value model. `Pack` renders a tree of Ruby values to MessagePack bytes and `Unpack`
parses them back, **byte-for-byte to the [MessagePack spec](https://github.com/msgpack/msgpack/blob/master/spec.md)
and the gem** — minimal integer widths, the str/bin/array/map/ext families, and
the reserved `Time` extension (type `-1`) — **without any Ruby runtime**.

It is the MessagePack backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime — a sibling
of [go-ruby-yaml](https://github.com/go-ruby-yaml/yaml) (Psych),
[go-ruby-marshal](https://github.com/go-ruby-marshal/marshal) (Marshal),
[go-ruby-regexp](https://github.com/go-ruby-regexp/regexp) (Onigmo) and
[go-ruby-erb](https://github.com/go-ruby-erb/erb) (ERB).

> **What it is — and isn't.** Encoding and decoding MessagePack for the Ruby value
> model is fully deterministic and needs **no interpreter**, so it lives here as
> pure Go. Binding the bytes to live Ruby objects is the host's job; this library
> hands back a small, explicit value model (`*Map`, `Bin`, `Symbol`, `*Ext`, …) the
> host maps to and from its own objects.

## Features

Faithful port of the gem's pack + unpack, validated against the `msgpack` gem on
every supported platform:

- **Minimal integer widths** — positive/negative fixint, `uint8/16/32/64`,
  `int8/16/32/64`, each narrowed exactly as the gem does; `*big.Int` for the full
  64-bit unsigned/signed range.
- **Floats** — IEEE-754 double (`float64`), the gem's default for `Float`; the
  unpacker also accepts `float32` (`0xca`).
- **Strings vs binary** — a Go `string` (and `Symbol`) packs as the **str** family
  (UTF-8); a `Bin` / `[]byte` packs as the **bin** family (ASCII-8BIT), matching
  how the gem distinguishes String encodings. Symbols pack as strings by default.
- **Arrays & maps** — `fixarray`/`array16`/`array32`, `fixmap`/`map16`/`map32`;
  maps round-trip as an **insertion-ordered** `*Map` (a plain Go map packs in a
  deterministic key order).
- **Extensions** — `fixext 1/2/4/8/16` and `ext 8/16/32`, plus the reserved
  **`Time` extension** (type `-1`) in its 32/64/96-bit forms for `time.Time`.
- **Streaming** — `Packer` accumulates successive `Write`s; `Unpacker` reads
  objects one at a time from a buffer.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x) on Linux, macOS, and Windows.

## Install

```sh
go get github.com/go-ruby-msgpack/msgpack
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-msgpack/msgpack"
)

func main() {
	m := msgpack.NewMap()
	m.Set("name", "web")
	m.Set("ports", []any{80, 443})
	m.Set("raw", msgpack.Bin([]byte{0xde, 0xad}))

	b, _ := msgpack.Pack(m) // MessagePack.pack — minimal widths, str/bin/map families
	fmt.Printf("%x\n", b)

	v, _ := msgpack.Unpack(b) // MessagePack.unpack — maps come back as *msgpack.Map
	fmt.Printf("%T\n", v)     // *msgpack.Map
}
```

## Ruby value model

`Pack` / `Unpack` round-trip an `any` drawn from a small, fixed set of Go types, so
a host can map its own object graph to and from this package:

| Ruby              | Go (Pack accepts)                   | Go (Unpack returns)   |
| ----------------- | ----------------------------------- | --------------------- |
| `nil`             | `nil`                               | `nil`                 |
| `true` / `false`  | `bool`                              | `bool`                |
| `Integer`         | `int`, `int8..64`, `uint8..64`, `*big.Int` | `int64` / `uint64` |
| `Float`           | `float64`, `float32`                | `float64`             |
| `String` (UTF-8)  | `string`                            | `string`              |
| `String` (binary) | `msgpack.Bin`, `[]byte`             | `msgpack.Bin`         |
| `Symbol`          | `msgpack.Symbol`                    | `string` (symbol→str) |
| `Array`           | `[]any`                             | `[]any`               |
| `Hash`            | `*msgpack.Map`, `map[string]any`, `map[any]any` | `*msgpack.Map` (ordered) |
| `Time`            | `time.Time`                         | `time.Time` (ext `-1`)|
| ext               | `*msgpack.Ext`                      | `*msgpack.Ext`        |

A Go `string` packs as **str** (UTF-8) and `Bin`/`[]byte` as **bin** (ASCII-8BIT),
since a Go string carries no encoding tag. A plain Go map packs in a deterministic
key order; a `*msgpack.Map` preserves insertion order, and `Unpack` always returns
maps as `*msgpack.Map` so key order round-trips.

## API

```go
// Pack serialises a Ruby value to MessagePack bytes (MessagePack.pack).
func Pack(v any) ([]byte, error)

// Unpack parses MessagePack bytes into a Ruby value (MessagePack.unpack).
func Unpack(b []byte) (any, error)

// Packer is a streaming encoder; Unpacker a streaming decoder.
type Packer struct{ /* ... */ }
func NewPacker() *Packer
func (p *Packer) Write(v any) error
func (p *Packer) Bytes() []byte
func (p *Packer) Reset()

type Unpacker struct{ /* ... */ }
func NewUnpacker(b []byte) *Unpacker
func (u *Unpacker) Read() (any, error)
func (u *Unpacker) Remaining() int
func (u *Unpacker) Reset(b []byte)

type Symbol string
type Bin    []byte
type Ext    struct { Type int8; Payload []byte }
type Map    struct { /* insertion-ordered Hash */ }
func NewMap() *Map
func (m *Map) Set(key, val any)
func (m *Map) Get(key any) (any, bool)
func (m *Map) Pairs() []Pair
func (m *Map) Len() int
```

## Tests & coverage

The suite pairs deterministic, ruby-free golden-byte tests (which alone hold
coverage at 100%, so the qemu cross-arch and Windows lanes pass the gate) with a
**differential gem oracle**: a wide corpus is packed here and compared
byte-for-byte to `MessagePack::Factory#dump`, and gem-packed bytes are unpacked
here — round-tripping integers (every minimal-width boundary), str/bin, arrays,
ordered maps, and the `Time` extension in both directions. The oracle scripts
`$stdout.binmode` and skip themselves where `ruby` (or the `msgpack` gem) is
absent.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-msgpack/msgpack authors.

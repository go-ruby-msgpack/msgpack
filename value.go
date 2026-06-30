// Copyright (c) the go-ruby-msgpack/msgpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package msgpack is a pure-Go (CGO-free) MRI-faithful MessagePack codec for the
// Ruby value model. It is the deterministic, interpreter-independent core of
// Ruby's `msgpack` gem: [Pack] renders a tree of Ruby values to MessagePack bytes
// and [Unpack] parses such bytes back, so Unpack(Pack(x)) round-trips the values
// Ruby programs serialise — byte-for-byte to the MessagePack spec and the gem,
// without any Ruby runtime.
//
// # Ruby value model
//
// A Ruby value is represented by an [any] drawn from a small, fixed set of Go
// types so a host (such as go-embedded-ruby) can map its own object graph to and
// from this package:
//
//	Ruby            Go (Pack accepts)               Go (Unpack returns)
//	----            -----------------               -------------------
//	nil             nil                             nil
//	true / false    bool                            bool
//	Integer         int, int8..64, uint8..64        int64 or uint64
//	Float           float64, float32                float64
//	String (UTF-8)  string                          string
//	String (binary) Bin([]byte), []byte             Bin (ASCII-8BIT bytes)
//	Symbol          Symbol                          string (symbol→str default)
//	Array           []any                           []any
//	Hash            *Map (ordered), map[...]any      *Map (insertion order)
//	Time            time.Time                       time.Time (ext type -1)
//	ext             *Ext (type, payload)            *Ext
//
// Pack also accepts a plain Go map[string]any / map[any]any (sorted by a stable
// key order for determinism); Unpack always yields an ordered [*Map] for a map so
// key order is preserved on round-trip.
//
// Strings carry no encoding tag in Go, so Pack maps a Go string to the
// MessagePack str family (UTF-8) and a [Bin] / []byte to the bin family
// (ASCII-8BIT), matching how the gem distinguishes String encodings.
package msgpack

import "math/big"

// Value is the interface satisfied by every Ruby value this package handles. It
// is purely documentary — the public API uses any — but a host may use it to
// constrain its own adapters.
type Value = any

// Symbol is a Ruby Symbol (`:name`). By default the gem packs a Symbol as a str,
// so Pack emits Symbol as the str family and Unpack returns a plain string.
type Symbol string

// Bin is a Ruby binary (ASCII-8BIT) String. Pack emits it as the MessagePack bin
// family and Unpack returns a Bin for the bin family, distinguishing it from a
// UTF-8 str (which maps to a Go string).
type Bin []byte

// Ext is a MessagePack extension object: an application-defined type byte and a
// raw payload. The reserved Time type is -1; Pack/Unpack handle [time.Time] via
// that type directly, so a host uses Ext for any other extension.
type Ext struct {
	Type    int8
	Payload []byte
}

// Pair is one entry of an ordered map.
type Pair struct {
	Key Value
	Val Value
}

// Map is an insertion-ordered Ruby Hash. Unpack returns maps as *Map so key order
// round-trips; Pack accepts *Map, a plain Go map (emitted in a stable key order),
// or a []Pair via NewMap.
type Map struct {
	pairs []Pair
	index map[any]int // identity index for scalar (comparable) keys
}

// NewMap returns an empty ordered Map.
func NewMap() *Map { return &Map{index: map[any]int{}} }

// Len reports the number of entries.
func (m *Map) Len() int { return len(m.pairs) }

// Pairs returns the entries in insertion order. The slice must not be mutated.
func (m *Map) Pairs() []Pair { return m.pairs }

// Set inserts or replaces the entry for key. A comparable key replaces an
// existing equal key; a non-comparable key (slice / map …) is always appended.
func (m *Map) Set(key, val Value) {
	if m.index == nil {
		m.index = map[any]int{}
	}
	if comparableKey(key) {
		if i, ok := m.index[key]; ok {
			m.pairs[i].Val = val
			return
		}
		m.index[key] = len(m.pairs)
	}
	m.pairs = append(m.pairs, Pair{Key: key, Val: val})
}

// Get returns the value for a comparable key and whether it was present.
func (m *Map) Get(key Value) (Value, bool) {
	if comparableKey(key) {
		if i, ok := m.index[key]; ok {
			return m.pairs[i].Val, true
		}
	}
	return nil, false
}

// comparableKey reports whether key may be used as a Go map index (so Set can
// deduplicate it). Slices, maps, *Map and *Ext are the non-comparable shapes.
func comparableKey(key Value) bool {
	switch key.(type) {
	case []any, *Map, *Ext, Bin:
		return false
	}
	return true
}

// asBigInt is the canonical big-integer view of an integer Value, used by the
// packer; it is never called with a non-integer.
func asBigInt(v Value) *big.Int {
	switch n := v.(type) {
	case int:
		return big.NewInt(int64(n))
	case int8:
		return big.NewInt(int64(n))
	case int16:
		return big.NewInt(int64(n))
	case int32:
		return big.NewInt(int64(n))
	case int64:
		return big.NewInt(n)
	case uint:
		return new(big.Int).SetUint64(uint64(n))
	case uint8:
		return new(big.Int).SetUint64(uint64(n))
	case uint16:
		return new(big.Int).SetUint64(uint64(n))
	case uint32:
		return new(big.Int).SetUint64(uint64(n))
	case uint64:
		return new(big.Int).SetUint64(n)
	case uintptr:
		return new(big.Int).SetUint64(uint64(n))
	}
	return nil
}

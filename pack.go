// Copyright (c) the go-ruby-msgpack/msgpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package msgpack

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"sort"
	"time"
)

// Packer is a streaming MessagePack encoder. Successive Write calls append their
// encodings to an internal buffer; Bytes returns the accumulated output. The zero
// Packer is ready to use. It mirrors the gem's MessagePack::Packer.
type Packer struct {
	buf []byte
}

// NewPacker returns a fresh, empty Packer.
func NewPacker() *Packer { return &Packer{} }

// Bytes returns the bytes written so far. The slice aliases the internal buffer
// until the next Write; copy it if it must outlive further writes.
func (p *Packer) Bytes() []byte { return p.buf }

// Reset discards any buffered output, readying the Packer for reuse.
func (p *Packer) Reset() { p.buf = p.buf[:0] }

// Write encodes one Ruby value and appends it to the buffer, matching the gem's
// Packer#write. A value outside the package model returns an error.
func (p *Packer) Write(v Value) error {
	switch x := v.(type) {
	case nil:
		p.buf = append(p.buf, 0xc0)
	case bool:
		if x {
			p.buf = append(p.buf, 0xc3)
		} else {
			p.buf = append(p.buf, 0xc2)
		}
	case int:
		p.writeInt(int64(x))
	case int8:
		p.writeInt(int64(x))
	case int16:
		p.writeInt(int64(x))
	case int32:
		p.writeInt(int64(x))
	case int64:
		p.writeInt(x)
	case uint:
		p.writeUint(uint64(x))
	case uint8:
		p.writeUint(uint64(x))
	case uint16:
		p.writeUint(uint64(x))
	case uint32:
		p.writeUint(uint64(x))
	case uint64:
		p.writeUint(x)
	case uintptr:
		p.writeUint(uint64(x))
	case *big.Int:
		return p.writeBig(x)
	case float32:
		p.writeFloat64(float64(x))
	case float64:
		p.writeFloat64(x)
	case string:
		p.writeStr(x)
	case Symbol:
		p.writeStr(string(x))
	case Bin:
		p.writeBin(x)
	case []byte:
		p.writeBin(x)
	case []any:
		return p.writeArray(x)
	case *Map:
		return p.writeMap(x.pairs)
	case map[string]any:
		return p.writeGoMap(x)
	case map[any]any:
		return p.writeGoAnyMap(x)
	case *Ext:
		p.writeExt(x.Type, x.Payload)
	case time.Time:
		p.writeTime(x)
	default:
		return fmt.Errorf("msgpack: cannot pack %T", v)
	}
	return nil
}

// writeInt encodes a signed integer in the minimal MessagePack width, matching
// the gem: positive values prefer the unsigned ladder, negatives the signed one.
func (p *Packer) writeInt(n int64) {
	if n >= 0 {
		p.writeUint(uint64(n))
		return
	}
	switch {
	case n >= -32:
		p.buf = append(p.buf, byte(n)) // negative fixint 111xxxxx
	case n >= math.MinInt8:
		p.buf = append(p.buf, 0xd0, byte(int8(n)))
	case n >= math.MinInt16:
		p.buf = append(p.buf, 0xd1)
		p.buf = binary.BigEndian.AppendUint16(p.buf, uint16(int16(n)))
	case n >= math.MinInt32:
		p.buf = append(p.buf, 0xd2)
		p.buf = binary.BigEndian.AppendUint32(p.buf, uint32(int32(n)))
	default:
		p.buf = append(p.buf, 0xd3)
		p.buf = binary.BigEndian.AppendUint64(p.buf, uint64(n))
	}
}

// writeUint encodes an unsigned integer in the minimal MessagePack width.
func (p *Packer) writeUint(n uint64) {
	switch {
	case n < 0x80:
		p.buf = append(p.buf, byte(n)) // positive fixint 0xxxxxxx
	case n <= math.MaxUint8:
		p.buf = append(p.buf, 0xcc, byte(n))
	case n <= math.MaxUint16:
		p.buf = append(p.buf, 0xcd)
		p.buf = binary.BigEndian.AppendUint16(p.buf, uint16(n))
	case n <= math.MaxUint32:
		p.buf = append(p.buf, 0xce)
		p.buf = binary.BigEndian.AppendUint32(p.buf, uint32(n))
	default:
		p.buf = append(p.buf, 0xcf)
		p.buf = binary.BigEndian.AppendUint64(p.buf, n)
	}
}

// bigMaxU64 / bigMinI64 bound the values *big.Int can encode as a MessagePack
// integer; outside that range the gem itself raises, so we error.
var (
	bigMaxU64 = new(big.Int).SetUint64(math.MaxUint64)
	bigMinI64 = big.NewInt(math.MinInt64)
)

// writeBig encodes a big.Int that fits the unsigned/signed 64-bit integer range.
func (p *Packer) writeBig(n *big.Int) error {
	if n.Sign() >= 0 {
		if n.Cmp(bigMaxU64) > 0 {
			return fmt.Errorf("msgpack: integer %s too big to pack", n)
		}
		p.writeUint(n.Uint64())
		return nil
	}
	if n.Cmp(bigMinI64) < 0 {
		return fmt.Errorf("msgpack: integer %s too small to pack", n)
	}
	p.writeInt(n.Int64())
	return nil
}

// writeFloat64 encodes a double-precision float (the gem's default for Float).
func (p *Packer) writeFloat64(f float64) {
	p.buf = append(p.buf, 0xcb)
	p.buf = binary.BigEndian.AppendUint64(p.buf, math.Float64bits(f))
}

// writeStr encodes a UTF-8 string in the minimal str family.
func (p *Packer) writeStr(s string) {
	n := len(s)
	switch {
	case n < 32:
		p.buf = append(p.buf, 0xa0|byte(n)) // fixstr 101xxxxx
	case n <= math.MaxUint8:
		p.buf = append(p.buf, 0xd9, byte(n))
	case n <= math.MaxUint16:
		p.buf = append(p.buf, 0xda)
		p.buf = binary.BigEndian.AppendUint16(p.buf, uint16(n))
	default:
		p.buf = append(p.buf, 0xdb)
		p.buf = binary.BigEndian.AppendUint32(p.buf, uint32(n))
	}
	p.buf = append(p.buf, s...)
}

// writeBin encodes binary (ASCII-8BIT) bytes in the minimal bin family.
func (p *Packer) writeBin(b []byte) {
	n := len(b)
	switch {
	case n <= math.MaxUint8:
		p.buf = append(p.buf, 0xc4, byte(n))
	case n <= math.MaxUint16:
		p.buf = append(p.buf, 0xc5)
		p.buf = binary.BigEndian.AppendUint16(p.buf, uint16(n))
	default:
		p.buf = append(p.buf, 0xc6)
		p.buf = binary.BigEndian.AppendUint32(p.buf, uint32(n))
	}
	p.buf = append(p.buf, b...)
}

// writeArray encodes a sequence header then each element.
func (p *Packer) writeArray(a []any) error {
	p.writeArrayHeader(len(a))
	for _, e := range a {
		if err := p.Write(e); err != nil {
			return err
		}
	}
	return nil
}

// writeArrayHeader emits the minimal array-family prefix for n elements.
func (p *Packer) writeArrayHeader(n int) {
	switch {
	case n < 16:
		p.buf = append(p.buf, 0x90|byte(n)) // fixarray 1001xxxx
	case n <= math.MaxUint16:
		p.buf = append(p.buf, 0xdc)
		p.buf = binary.BigEndian.AppendUint16(p.buf, uint16(n))
	default:
		p.buf = append(p.buf, 0xdd)
		p.buf = binary.BigEndian.AppendUint32(p.buf, uint32(n))
	}
}

// writeMapHeader emits the minimal map-family prefix for n entries.
func (p *Packer) writeMapHeader(n int) {
	switch {
	case n < 16:
		p.buf = append(p.buf, 0x80|byte(n)) // fixmap 1000xxxx
	case n <= math.MaxUint16:
		p.buf = append(p.buf, 0xde)
		p.buf = binary.BigEndian.AppendUint16(p.buf, uint16(n))
	default:
		p.buf = append(p.buf, 0xdf)
		p.buf = binary.BigEndian.AppendUint32(p.buf, uint32(n))
	}
}

// writeMap encodes an ordered list of key/value pairs (a *Map).
func (p *Packer) writeMap(pairs []Pair) error {
	p.writeMapHeader(len(pairs))
	for _, kv := range pairs {
		if err := p.Write(kv.Key); err != nil {
			return err
		}
		if err := p.Write(kv.Val); err != nil {
			return err
		}
	}
	return nil
}

// writeGoMap encodes a plain Go string-keyed map in sorted key order for
// determinism (a plain Go map has no insertion order).
func (p *Packer) writeGoMap(m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	p.writeMapHeader(len(keys))
	for _, k := range keys {
		p.writeStr(k)
		if err := p.Write(m[k]); err != nil {
			return err
		}
	}
	return nil
}

// writeGoAnyMap encodes a plain Go any-keyed map. Keys are sorted by their packed
// bytes so emission is deterministic regardless of Go map iteration order.
func (p *Packer) writeGoAnyMap(m map[any]any) error {
	type kv struct {
		keyBytes []byte
		key, val any
	}
	entries := make([]kv, 0, len(m))
	for k, v := range m {
		kb, err := Pack(k)
		if err != nil {
			return err
		}
		entries = append(entries, kv{kb, k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return string(entries[i].keyBytes) < string(entries[j].keyBytes)
	})
	p.writeMapHeader(len(entries))
	for _, e := range entries {
		p.buf = append(p.buf, e.keyBytes...)
		if err := p.Write(e.val); err != nil {
			return err
		}
	}
	return nil
}

// writeExt emits an extension object of the given type and payload in the minimal
// ext family (fixext 1/2/4/8/16, else ext8/16/32).
func (p *Packer) writeExt(typ int8, payload []byte) {
	n := len(payload)
	switch n {
	case 1:
		p.buf = append(p.buf, 0xd4)
	case 2:
		p.buf = append(p.buf, 0xd5)
	case 4:
		p.buf = append(p.buf, 0xd6)
	case 8:
		p.buf = append(p.buf, 0xd7)
	case 16:
		p.buf = append(p.buf, 0xd8)
	default:
		switch {
		case n <= math.MaxUint8:
			p.buf = append(p.buf, 0xc7, byte(n))
		case n <= math.MaxUint16:
			p.buf = append(p.buf, 0xc8)
			p.buf = binary.BigEndian.AppendUint16(p.buf, uint16(n))
		default:
			p.buf = append(p.buf, 0xc9)
			p.buf = binary.BigEndian.AppendUint32(p.buf, uint32(n))
		}
	}
	p.buf = append(p.buf, byte(typ))
	p.buf = append(p.buf, payload...)
}

// timeExtType is the reserved MessagePack extension type for a timestamp (-1).
const timeExtType int8 = -1

// writeTime encodes a Time using the reserved timestamp extension (type -1),
// selecting the 32/64/96-bit form per the MessagePack spec, exactly as the gem's
// MessagePack::Time::Packer does.
func (p *Packer) writeTime(t time.Time) {
	sec := t.Unix()
	nsec := uint32(t.Nanosecond())
	if uint64(sec)>>34 == 0 { // seconds fits 34 bits (covers the spec's 64/32 forms)
		data64 := uint64(nsec)<<34 | uint64(sec)
		if data64&0xffffffff00000000 == 0 { // nsec==0 && sec fits 32 bits
			var b [4]byte
			binary.BigEndian.PutUint32(b[:], uint32(data64))
			p.writeExt(timeExtType, b[:])
			return
		}
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], data64)
		p.writeExt(timeExtType, b[:])
		return
	}
	var b [12]byte
	binary.BigEndian.PutUint32(b[:4], nsec)
	binary.BigEndian.PutUint64(b[4:], uint64(sec))
	p.writeExt(timeExtType, b[:])
}

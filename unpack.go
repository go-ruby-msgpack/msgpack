// Copyright (c) the go-ruby-msgpack/msgpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package msgpack

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
)

// errEOF is returned when the buffer ends mid-object; errTrailing when bytes
// remain after a complete object where exactly one was expected.
var (
	errEOF      = errors.New("msgpack: unexpected end of input")
	errTrailing = errors.New("msgpack: trailing bytes after value")
)

// Unpacker is a streaming MessagePack decoder over a byte buffer. Successive Read
// calls decode the next object; off advances past consumed bytes. It mirrors the
// gem's MessagePack::Unpacker.
type Unpacker struct {
	buf []byte
	off int
}

// NewUnpacker returns an Unpacker reading from b. The buffer is not copied.
func NewUnpacker(b []byte) *Unpacker { return &Unpacker{buf: b} }

// Reset points the Unpacker at a fresh buffer, readying it for reuse.
func (u *Unpacker) Reset(b []byte) { u.buf = b; u.off = 0 }

// Remaining reports how many unread bytes are left in the buffer.
func (u *Unpacker) Remaining() int { return len(u.buf) - u.off }

// Read decodes the next MessagePack object from the buffer.
func (u *Unpacker) Read() (Value, error) {
	c, err := u.readByte()
	if err != nil {
		return nil, err
	}
	switch {
	case c <= 0x7f: // positive fixint
		return int64(c), nil
	case c >= 0xe0: // negative fixint
		return int64(int8(c)), nil
	case c >= 0x80 && c <= 0x8f: // fixmap
		return u.readMap(int(c & 0x0f))
	case c >= 0x90 && c <= 0x9f: // fixarray
		return u.readArray(int(c & 0x0f))
	case c >= 0xa0 && c <= 0xbf: // fixstr
		return u.readStr(int(c & 0x1f))
	}
	switch c {
	case 0xc0:
		return nil, nil
	case 0xc2:
		return false, nil
	case 0xc3:
		return true, nil
	case 0xc4:
		return u.readBin(1)
	case 0xc5:
		return u.readBin(2)
	case 0xc6:
		return u.readBin(4)
	case 0xc7:
		return u.readExt(1)
	case 0xc8:
		return u.readExt(2)
	case 0xc9:
		return u.readExt(4)
	case 0xca:
		return u.readFloat32()
	case 0xcb:
		return u.readFloat64()
	case 0xcc:
		return u.readUint(1)
	case 0xcd:
		return u.readUint(2)
	case 0xce:
		return u.readUint(4)
	case 0xcf:
		return u.readUint(8)
	case 0xd0:
		return u.readSint(1)
	case 0xd1:
		return u.readSint(2)
	case 0xd2:
		return u.readSint(4)
	case 0xd3:
		return u.readSint(8)
	case 0xd4:
		return u.readFixExt(1)
	case 0xd5:
		return u.readFixExt(2)
	case 0xd6:
		return u.readFixExt(4)
	case 0xd7:
		return u.readFixExt(8)
	case 0xd8:
		return u.readFixExt(16)
	case 0xd9:
		return u.readStrN(1)
	case 0xda:
		return u.readStrN(2)
	case 0xdb:
		return u.readStrN(4)
	case 0xdc:
		return u.readArrayN(2)
	case 0xdd:
		return u.readArrayN(4)
	case 0xde:
		return u.readMapN(2)
	case 0xdf:
		return u.readMapN(4)
	}
	return nil, fmt.Errorf("msgpack: invalid prefix byte 0x%02x", c)
}

// readByte consumes and returns one byte, or errEOF.
func (u *Unpacker) readByte() (byte, error) {
	if u.off >= len(u.buf) {
		return 0, errEOF
	}
	c := u.buf[u.off]
	u.off++
	return c, nil
}

// take consumes n bytes and returns them as a slice into the buffer.
func (u *Unpacker) take(n int) ([]byte, error) {
	if n < 0 || u.off+n > len(u.buf) {
		return nil, errEOF
	}
	b := u.buf[u.off : u.off+n]
	u.off += n
	return b, nil
}

// readLen reads a big-endian length of size bytes (1, 2 or 4).
func (u *Unpacker) readLen(size int) (int, error) {
	b, err := u.take(size)
	if err != nil {
		return 0, err
	}
	switch size {
	case 1:
		return int(b[0]), nil
	case 2:
		return int(binary.BigEndian.Uint16(b)), nil
	default:
		return int(binary.BigEndian.Uint32(b)), nil
	}
}

// readUint reads an unsigned integer of size bytes, returning the narrowest of
// int64 / uint64 the gem would (uint64 only when it overflows int64).
func (u *Unpacker) readUint(size int) (Value, error) {
	b, err := u.take(size)
	if err != nil {
		return nil, err
	}
	var n uint64
	for _, x := range b {
		n = n<<8 | uint64(x)
	}
	if n <= math.MaxInt64 {
		return int64(n), nil
	}
	return n, nil
}

// readSint reads a signed integer of size bytes as int64.
func (u *Unpacker) readSint(size int) (Value, error) {
	b, err := u.take(size)
	if err != nil {
		return nil, err
	}
	var n uint64
	if b[0]&0x80 != 0 {
		n = ^uint64(0) // sign-extend
	}
	for _, x := range b {
		n = n<<8 | uint64(x)
	}
	return int64(n), nil
}

// readFloat32 reads a 4-byte float and widens it to float64 (the gem's Float).
func (u *Unpacker) readFloat32() (Value, error) {
	b, err := u.take(4)
	if err != nil {
		return nil, err
	}
	return float64(math.Float32frombits(binary.BigEndian.Uint32(b))), nil
}

// readFloat64 reads an 8-byte float.
func (u *Unpacker) readFloat64() (Value, error) {
	b, err := u.take(8)
	if err != nil {
		return nil, err
	}
	return math.Float64frombits(binary.BigEndian.Uint64(b)), nil
}

// readStr reads a UTF-8 string of n bytes.
func (u *Unpacker) readStr(n int) (Value, error) {
	b, err := u.take(n)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// readStrN reads a str family whose length prefix is sizeBytes wide.
func (u *Unpacker) readStrN(sizeBytes int) (Value, error) {
	n, err := u.readLen(sizeBytes)
	if err != nil {
		return nil, err
	}
	return u.readStr(n)
}

// readBin reads a bin family whose length prefix is sizeBytes wide, returning a
// Bin (ASCII-8BIT) so it is distinct from a UTF-8 str.
func (u *Unpacker) readBin(sizeBytes int) (Value, error) {
	n, err := u.readLen(sizeBytes)
	if err != nil {
		return nil, err
	}
	b, err := u.take(n)
	if err != nil {
		return nil, err
	}
	out := make(Bin, n)
	copy(out, b)
	return out, nil
}

// readArray reads n elements into a []any.
func (u *Unpacker) readArray(n int) (Value, error) {
	a := make([]any, n)
	for i := range a {
		v, err := u.Read()
		if err != nil {
			return nil, err
		}
		a[i] = v
	}
	return a, nil
}

// readArrayN reads an array family whose count prefix is sizeBytes wide.
func (u *Unpacker) readArrayN(sizeBytes int) (Value, error) {
	n, err := u.readLen(sizeBytes)
	if err != nil {
		return nil, err
	}
	return u.readArray(n)
}

// readMap reads n key/value pairs into an ordered *Map.
func (u *Unpacker) readMap(n int) (Value, error) {
	m := NewMap()
	for range n {
		k, err := u.Read()
		if err != nil {
			return nil, err
		}
		v, err := u.Read()
		if err != nil {
			return nil, err
		}
		m.Set(k, v)
	}
	return m, nil
}

// readMapN reads a map family whose count prefix is sizeBytes wide.
func (u *Unpacker) readMapN(sizeBytes int) (Value, error) {
	n, err := u.readLen(sizeBytes)
	if err != nil {
		return nil, err
	}
	return u.readMap(n)
}

// readFixExt reads a fixext of payload length n (1/2/4/8/16): the type byte then
// the payload.
func (u *Unpacker) readFixExt(n int) (Value, error) {
	typ, err := u.readByte()
	if err != nil {
		return nil, err
	}
	return u.extValue(int8(typ), n)
}

// readExt reads an ext8/16/32 whose length prefix is sizeBytes wide.
func (u *Unpacker) readExt(sizeBytes int) (Value, error) {
	n, err := u.readLen(sizeBytes)
	if err != nil {
		return nil, err
	}
	typ, err := u.readByte()
	if err != nil {
		return nil, err
	}
	return u.extValue(int8(typ), n)
}

// extValue reads n payload bytes for the extension type and materialises it: the
// reserved Time type (-1) becomes a time.Time, any other type a *Ext.
func (u *Unpacker) extValue(typ int8, n int) (Value, error) {
	b, err := u.take(n)
	if err != nil {
		return nil, err
	}
	if typ == timeExtType {
		return decodeTime(b)
	}
	payload := make([]byte, n)
	copy(payload, b)
	return &Ext{Type: typ, Payload: payload}, nil
}

// decodeTime decodes a timestamp extension payload (4, 8 or 12 bytes) into a UTC
// time.Time, inverting writeTime and matching MessagePack::Time::Unpacker.
func decodeTime(b []byte) (Value, error) {
	switch len(b) {
	case 4:
		sec := int64(binary.BigEndian.Uint32(b))
		return time.Unix(sec, 0).UTC(), nil
	case 8:
		data := binary.BigEndian.Uint64(b)
		nsec := int64(data >> 34)
		sec := int64(data & 0x3ffffffff)
		return time.Unix(sec, nsec).UTC(), nil
	case 12:
		nsec := int64(binary.BigEndian.Uint32(b[:4]))
		sec := int64(binary.BigEndian.Uint64(b[4:]))
		return time.Unix(sec, nsec).UTC(), nil
	default:
		return nil, fmt.Errorf("msgpack: invalid timestamp length %d", len(b))
	}
}

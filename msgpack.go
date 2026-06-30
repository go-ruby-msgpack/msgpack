// Copyright (c) the go-ruby-msgpack/msgpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package msgpack

// Pack serialises a Ruby value to MessagePack bytes, matching MessagePack.pack /
// Object#to_msgpack from the `msgpack` gem. v is drawn from the package value
// model (see the package doc); a value outside that model returns an error rather
// than panicking. Integers use the minimal MessagePack width, strings use the str
// family (UTF-8) and [Bin]/[]byte the bin family, and [time.Time] uses the
// reserved Time extension (type -1), each byte-for-byte to the gem.
func Pack(v Value) ([]byte, error) {
	var p Packer
	if err := p.Write(v); err != nil {
		return nil, err
	}
	return p.Bytes(), nil
}

// Unpack parses MessagePack bytes into a Ruby value, matching
// MessagePack.unpack / MessagePack.load. Maps come back as an ordered [*Map]
// (key order preserved), arrays as []any, the str family as a Go string, the bin
// family as [Bin], the Time extension as [time.Time], and any other extension as
// [*Ext]. Trailing bytes after the first object are an error (use [Unpacker] to
// read a stream).
func Unpack(b []byte) (Value, error) {
	u := NewUnpacker(b)
	v, err := u.Read()
	if err != nil {
		return nil, err
	}
	if u.off != len(u.buf) {
		return nil, errTrailing
	}
	return v, nil
}

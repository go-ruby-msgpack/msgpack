// Copyright (c) the go-ruby-msgpack/msgpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package msgpack

import (
	"bytes"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// intToS renders an int as decimal, used by the map oracle to build Ruby literals.
func intToS(n int) string { return strconv.Itoa(n) }

// eqValue compares two decoded Values structurally (slices, *Map, Bin, Time).
func eqValue(a, b Value) bool {
	switch av := a.(type) {
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !eqValue(av[i], bv[i]) {
				return false
			}
		}
		return true
	case *Map:
		bv, ok := b.(*Map)
		if !ok || av.Len() != bv.Len() {
			return false
		}
		for i, p := range av.pairs {
			q := bv.pairs[i]
			if !eqValue(p.Key, q.Key) || !eqValue(p.Val, q.Val) {
				return false
			}
		}
		return true
	case Bin:
		bv, ok := b.(Bin)
		return ok && bytes.Equal(av, bv)
	case time.Time:
		bv, ok := b.(time.Time)
		return ok && av.Equal(bv) && av.Nanosecond() == bv.Nanosecond()
	case *Ext:
		bv, ok := b.(*Ext)
		return ok && av.Type == bv.Type && bytes.Equal(av.Payload, bv.Payload)
	default:
		return reflect.DeepEqual(a, b)
	}
}

// packHex packs v and returns the hex of the bytes, failing the test on error.
func packHex(t *testing.T, v Value) string {
	t.Helper()
	b, err := Pack(v)
	if err != nil {
		t.Fatalf("Pack(%#v): %v", v, err)
	}
	return hexOf(b)
}

// hexOf renders bytes as a lowercase hex string.
func hexOf(b []byte) string {
	const hexd = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, x := range b {
		out = append(out, hexd[x>>4], hexd[x&0x0f])
	}
	return string(out)
}

// TestPackGoldenBytes pins every family's encoding to the spec/gem bytes without
// any Ruby runtime, so the qemu and Windows lanes verify the exact encoding too.
func TestPackGoldenBytes(t *testing.T) {
	bi64max := new(big.Int).SetUint64(math.MaxUint64)
	bi64min := big.NewInt(math.MinInt64)
	cases := []struct {
		name string
		v    Value
		hex  string
	}{
		{"nil", nil, "c0"},
		{"true", true, "c3"},
		{"false", false, "c2"},
		{"posfix", 127, "7f"},
		{"uint8", 128, "cc80"},
		{"uint16", 256, "cd0100"},
		{"uint32", 65536, "ce00010000"},
		{"uint64", uint64(1) << 32, "cf0000000100000000"},
		{"u64max", bi64max, "cfffffffffffffffff"},
		{"negfix", -1, "ff"},
		{"int8", -33, "d0df"},
		{"int16", -129, "d1ff7f"},
		{"int32", -32769, "d2ffff7fff"},
		{"int64", int64(math.MinInt32) - 1, "d3ffffffff7fffffff"},
		{"i64min", bi64min, "d38000000000000000"},
		{"float", 1.5, "cb3ff8000000000000"},
		{"float32", float32(1.5), "cb3ff8000000000000"},
		{"emptystr", "", "a0"},
		{"fixstr", "hi", "a26869"},
		{"sym", Symbol("a"), "a161"},
		{"bin", Bin([]byte("ab")), "c4026162"},
		{"bytes", []byte("ab"), "c4026162"},
		{"emptyarr", []any{}, "90"},
		{"arr", []any{1, 2}, "920102"},
		{"emptymap", NewMap(), "80"},
		{"ext1", &Ext{Type: 5, Payload: []byte{0xaa}}, "d405aa"},
		{"ext2", &Ext{Type: 5, Payload: []byte{1, 2}}, "d5050102"},
		{"ext4", &Ext{Type: 5, Payload: []byte{1, 2, 3, 4}}, "d60501020304"},
		{"ext8", &Ext{Type: 5, Payload: bytes.Repeat([]byte{1}, 8)}, "d7050101010101010101"},
		{"ext16", &Ext{Type: 5, Payload: bytes.Repeat([]byte{1}, 16)}, "d80501010101010101010101010101010101"},
		{"ext3", &Ext{Type: 5, Payload: []byte{1, 2, 3}}, "c70305010203"},
		{"time32", time.Unix(1234567890, 0).UTC(), "d6ff499602d2"},
		{"time64", time.Unix(1234567890, 500).UTC(), "d7ff000007d0499602d2"},
		{"time96", time.Unix(-1, 0).UTC(), "c70cff00000000ffffffffffffffff"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := packHex(t, c.v); got != c.hex {
				t.Errorf("Pack(%s) = %s, want %s", c.name, got, c.hex)
			}
		})
	}
}

// TestPackWideHeaders exercises the 16/32-bit length prefixes of every family
// without allocating gigabytes (the 32-bit cases use modest sizes; the prefix
// branch is what matters).
func TestPackWideHeaders(t *testing.T) {
	// str8 / str16 / str32 boundaries.
	if got := packHex(t, randStr(32))[:4]; got != "d920" {
		t.Errorf("str8 prefix = %s", got)
	}
	if got := packHex(t, randStr(256))[:6]; got != "da0100" {
		t.Errorf("str16 prefix = %s", got)
	}
	if got := packHex(t, randStr(1<<16))[:10]; got != "db00010000" {
		t.Errorf("str32 prefix = %s", got)
	}
	// bin16 / bin32.
	if got := packHex(t, Bin(make([]byte, 256)))[:6]; got != "c50100" {
		t.Errorf("bin16 prefix = %s", got)
	}
	if got := packHex(t, Bin(make([]byte, 1<<16)))[:10]; got != "c600010000" {
		t.Errorf("bin32 prefix = %s", got)
	}
	// array16 / array32.
	if got := packHex(t, makeAny(16))[:6]; got != "dc0010" {
		t.Errorf("array16 prefix = %s", got)
	}
	if got := packHex(t, makeAny(1<<16))[:10]; got != "dd00010000" {
		t.Errorf("array32 prefix = %s", got)
	}
	// map16 / map32.
	if got := packHex(t, makeMap(16))[:6]; got != "de0010" {
		t.Errorf("map16 prefix = %s", got)
	}
	if got := packHex(t, makeMap(1<<16))[:10]; got != "df00010000" {
		t.Errorf("map32 prefix = %s", got)
	}
	// ext16 / ext32.
	if got := packHex(t, &Ext{Type: 1, Payload: make([]byte, 256)})[:6]; got != "c80100" {
		t.Errorf("ext16 prefix = %s", got)
	}
	if got := packHex(t, &Ext{Type: 1, Payload: make([]byte, 1<<16)})[:10]; got != "c900010000" {
		t.Errorf("ext32 prefix = %s", got)
	}
}

// TestUnpackWideHeaders round-trips every 16/32-bit-prefixed family through Pack
// then Unpack, covering the wide decode arms (bin16/32, str16/32, array16/32,
// map16/32, ext16/32) and the fixext 2/4/16 sizes the golden test does not reach.
func TestUnpackWideHeaders(t *testing.T) {
	corpus := []Value{
		randStr(256), randStr(1 << 16), // str16, str32
		Bin(make([]byte, 256)), Bin(make([]byte, 1<<16)), // bin16, bin32
		makeAnyVals(16), makeAnyVals(1 << 16), // array16, array32
		makeMap(16), makeMap(1 << 16), // map16, map32
		&Ext{Type: 7, Payload: []byte{1, 2}},                // fixext2
		&Ext{Type: 7, Payload: []byte{1, 2, 3, 4}},          // fixext4
		&Ext{Type: 7, Payload: bytes.Repeat([]byte{1}, 16)}, // fixext16
		&Ext{Type: 7, Payload: make([]byte, 256)},           // ext16
		&Ext{Type: 7, Payload: make([]byte, 1<<16)},         // ext32
	}
	for _, v := range corpus {
		b, err := Pack(v)
		if err != nil {
			t.Fatalf("Pack: %v", err)
		}
		got, err := Unpack(b)
		if err != nil {
			t.Fatalf("Unpack wide %x...: %v", b[:6], err)
		}
		if !eqValue(got, v) {
			t.Errorf("wide round-trip mismatch for %T", v)
		}
	}
	// readBin's length-prefix truncation (bin16 header missing its 2nd length byte).
	if _, err := Unpack([]byte{0xc5, 0x00}); err == nil {
		t.Error("want bin16 length-truncation error")
	}
}

// makeAnyVals returns a []any of n distinct int64 values for array round-trips.
func makeAnyVals(n int) []any {
	a := make([]any, n)
	for i := range a {
		a[i] = int64(i % 128)
	}
	return a
}

// TestPackIntWidths checks every signed/unsigned Go integer type narrows to the
// minimal width.
func TestPackIntWidths(t *testing.T) {
	for _, c := range []struct {
		v   Value
		hex string
	}{
		{int8(-1), "ff"},
		{int16(256), "cd0100"},
		{int32(-32769), "d2ffff7fff"},
		{int64(1) << 40, "cf0000010000000000"},
		{uint(200), "ccc8"},
		{uint8(200), "ccc8"},
		{uint16(300), "cd012c"},
		{uint32(70000), "ce00011170"},
		{uint64(1) << 33, "cf0000000200000000"},
		{uintptr(5), "05"},
		{int(-2), "fe"},
	} {
		if got := packHex(t, c.v); got != c.hex {
			t.Errorf("Pack(%T %v) = %s, want %s", c.v, c.v, got, c.hex)
		}
	}
}

// TestPackGoMaps covers the plain Go map adapters (sorted-key determinism).
func TestPackGoMaps(t *testing.T) {
	b, err := Pack(map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	// sorted: a then b -> 82 a1 61 01 a1 62 02
	if hexOf(b) != "82a16101a16202" {
		t.Errorf("string map = %s", hexOf(b))
	}
	b, err = Pack(map[any]any{int64(2): "two", int64(1): "one"})
	if err != nil {
		t.Fatal(err)
	}
	// keys sorted by packed bytes: 01 < 02
	if hexOf(b) != "8201a36f6e6502a374776f" {
		t.Errorf("any map = %s", hexOf(b))
	}
}

// TestPackErrors covers every error branch of the packer.
func TestPackErrors(t *testing.T) {
	if _, err := Pack(complex(1, 2)); err == nil {
		t.Error("want error for unsupported type")
	}
	tooBig := new(big.Int).Lsh(big.NewInt(1), 64) // 2**64 > uint64 max
	if _, err := Pack(tooBig); err == nil {
		t.Error("want error for oversize bignum")
	}
	tooSmall := new(big.Int).Sub(big.NewInt(math.MinInt64), big.NewInt(1))
	if _, err := Pack(tooSmall); err == nil {
		t.Error("want error for undersize bignum")
	}
	// Error propagation through arrays and maps (unsupported element / value / key).
	if _, err := Pack([]any{complex(1, 2)}); err == nil {
		t.Error("want array element error")
	}
	m := NewMap()
	m.Set("k", complex(1, 2))
	if _, err := Pack(m); err == nil {
		t.Error("want map value error")
	}
	mk := NewMap()
	mk.Set(complex(1, 2), 1)
	if _, err := Pack(mk); err == nil {
		t.Error("want map key error")
	}
	if _, err := Pack(map[string]any{"k": complex(1, 2)}); err == nil {
		t.Error("want go-map value error")
	}
	if _, err := Pack(map[any]any{"k": complex(1, 2)}); err == nil {
		t.Error("want go-any-map value error")
	}
	if _, err := Pack(map[any]any{complex(1, 2): 1}); err == nil {
		t.Error("want go-any-map key error")
	}
}

// TestRoundTrip packs then unpacks a wide corpus and checks structural equality.
func TestRoundTrip(t *testing.T) {
	m := NewMap()
	m.Set("n", int64(1))
	m.Set(Symbol("s"), "v") // symbol key -> "s"
	inner := NewMap()
	inner.Set("d", true)
	m.Set("c", inner)

	corpus := []Value{
		nil, true, false,
		int64(0), int64(127), int64(128), int64(65536), int64(1) << 40,
		int64(-1), int64(-33), int64(-129), int64(-32769), int64(math.MinInt64),
		1.5, math.Inf(-1),
		"", "hi", randStr(40), randStr(300),
		Bin([]byte{0, 1, 2}), Bin(make([]byte, 300)),
		[]any{int64(1), "two", []any{true, nil}},
		m,
		time.Unix(1234567890, 0).UTC(),
		time.Unix(1234567890, 500).UTC(),
		time.Unix(-5, 123).UTC(),
		&Ext{Type: 9, Payload: []byte{1, 2, 3}},
	}
	for _, v := range corpus {
		b, err := Pack(v)
		if err != nil {
			t.Fatalf("Pack(%#v): %v", v, err)
		}
		got, err := Unpack(b)
		if err != nil {
			t.Fatalf("Unpack(%x): %v", b, err)
		}
		// Symbol-keyed map round-trips with the key as a plain string; rebuild the
		// expected shape for the map case.
		want := v
		if mm, ok := v.(*Map); ok {
			want = symKeysToStr(mm)
		}
		if !eqValue(got, want) {
			t.Errorf("round-trip %#v -> %#v", v, got)
		}
	}
}

// symKeysToStr returns a copy of m with Symbol keys converted to strings, matching
// how the wire (symbol→str) round-trips.
func symKeysToStr(m *Map) *Map {
	out := NewMap()
	for _, p := range m.pairs {
		k := p.Key
		if s, ok := k.(Symbol); ok {
			k = string(s)
		}
		v := p.Val
		if vm, ok := v.(*Map); ok {
			v = symKeysToStr(vm)
		}
		out.Set(k, v)
	}
	return out
}

// TestUnpackFloat32 covers the ca prefix (the packer never emits it; the gem can
// when asked, so the unpacker must accept it).
func TestUnpackFloat32(t *testing.T) {
	// ca 3f c0 00 00 = float32(1.5)
	v, err := Unpack([]byte{0xca, 0x3f, 0xc0, 0x00, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if v != 1.5 {
		t.Errorf("float32 unpack = %v", v)
	}
}

// TestUnpackUint64Big checks a cf value above int64 max returns a uint64.
func TestUnpackUint64Big(t *testing.T) {
	b, _ := Pack(new(big.Int).SetUint64(math.MaxUint64))
	v, err := Unpack(b)
	if err != nil {
		t.Fatal(err)
	}
	if u, ok := v.(uint64); !ok || u != math.MaxUint64 {
		t.Errorf("uint64 unpack = %#v", v)
	}
}

// TestUnpackErrors covers every decode error branch.
func TestUnpackErrors(t *testing.T) {
	for name, b := range map[string][]byte{
		"empty":         {},
		"truncFixstr":   {0xa2, 'h'},
		"truncStr8len":  {0xd9},
		"truncStr8body": {0xd9, 0x02, 'h'},
		"truncBin8":     {0xc4, 0x02, 0x00},
		"truncUint":     {0xcc},
		"truncSint":     {0xd0},
		"truncFloat32":  {0xca, 0x00},
		"truncFloat64":  {0xcb, 0x00},
		"truncArrayLen": {0xdc, 0x00},
		"truncArrayEl":  {0x92, 0x01},
		"truncMapLen":   {0xde, 0x00},
		"truncMapKey":   {0x81},
		"truncMapVal":   {0x81, 0xa1, 'k'},
		"truncFixExt":   {0xd4},
		"truncFixExtPl": {0xd4, 0x05},
		"truncExtLen":   {0xc7},
		"truncExtType":  {0xc7, 0x02},
		"truncExtPl":    {0xc7, 0x02, 0x05, 0x01},
		"badPrefix":     {0xc1},
		"trailing":      {0xc0, 0xc0},
	} {
		if _, err := Unpack(b); err == nil {
			t.Errorf("Unpack(%s %x): want error", name, b)
		}
	}
}

// TestUnpackBadTimestamp feeds a Time-ext with an illegal payload length (built as
// a raw ext3 of type -1) and expects an error.
func TestUnpackBadTimestamp(t *testing.T) {
	// c7 03 ff 00 00 00 : ext8, len 3, type -1, 3 payload bytes -> invalid ts len.
	if _, err := Unpack([]byte{0xc7, 0x03, 0xff, 0x00, 0x00, 0x00}); err == nil {
		t.Error("want invalid-timestamp error")
	}
}

// TestPackerStreaming exercises the Packer's streaming surface (NewPacker, Reset,
// successive Write, Bytes) and the Unpacker's (NewUnpacker, Read, Remaining,
// Reset) reading several objects from one buffer.
func TestPackerStreaming(t *testing.T) {
	p := NewPacker()
	for _, v := range []Value{int64(1), "two", true} {
		if err := p.Write(v); err != nil {
			t.Fatal(err)
		}
	}
	buf := append([]byte(nil), p.Bytes()...)

	u := NewUnpacker(buf)
	var got []Value
	for u.Remaining() > 0 {
		v, err := u.Read()
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, v)
	}
	if !eqValue(got, []any{int64(1), "two", true}) {
		t.Errorf("stream = %#v", got)
	}

	// Reset both and reuse.
	p.Reset()
	if len(p.Bytes()) != 0 {
		t.Error("Reset did not clear packer")
	}
	_ = p.Write(int64(42))
	u.Reset(p.Bytes())
	v, err := u.Read()
	if err != nil || v != int64(42) {
		t.Errorf("after reset = %v, %v", v, err)
	}
	if u.Remaining() != 0 {
		t.Errorf("Remaining after reset+read = %d", u.Remaining())
	}
}

// TestMapAPI covers the *Map helpers (Set replace, Get hit/miss, non-comparable
// keys, NewMap-less zero value via Set, Pairs).
func TestMapAPI(t *testing.T) {
	var m Map // zero value: Set must lazily init index
	m.Set("a", 1)
	m.Set("a", 2) // replace
	if v, ok := m.Get("a"); !ok || v != 2 {
		t.Errorf("replace get = %v %v", v, ok)
	}
	if _, ok := m.Get("missing"); ok {
		t.Error("missing key reported present")
	}
	// Non-comparable key is always appended and never found by Get.
	m.Set([]any{1}, "x")
	m.Set([]any{1}, "y")
	if _, ok := m.Get([]any{1}); ok {
		t.Error("non-comparable key should not be Get-able")
	}
	if m.Len() != 3 {
		t.Errorf("len = %d, want 3", m.Len())
	}
	if len(m.Pairs()) != 3 {
		t.Errorf("pairs len = %d", len(m.Pairs()))
	}
	// comparableKey for the remaining non-comparable shapes.
	for _, k := range []Value{NewMap(), &Ext{}, Bin{1}} {
		if comparableKey(k) {
			t.Errorf("comparableKey(%T) = true", k)
		}
	}
	if !comparableKey("s") {
		t.Error("comparableKey(string) = false")
	}
}

// TestAsBigInt covers every integer arm of asBigInt and its non-integer guard.
func TestAsBigInt(t *testing.T) {
	for _, v := range []Value{
		int(1), int8(1), int16(1), int32(1), int64(1),
		uint(1), uint8(1), uint16(1), uint32(1), uint64(1), uintptr(1),
	} {
		if asBigInt(v).Cmp(big.NewInt(1)) != 0 {
			t.Errorf("asBigInt(%T) != 1", v)
		}
	}
	if asBigInt("notint") != nil {
		t.Error("asBigInt(non-integer) should be nil")
	}
}

// randStr returns a deterministic string of n 'a' bytes for header tests.
func randStr(n int) string { return string(bytes.Repeat([]byte{'a'}, n)) }

// makeAny returns a []any of n nils for array-header tests.
func makeAny(n int) []any { return make([]any, n) }

// makeMap returns a *Map of n int->int entries for map-header tests.
func makeMap(n int) *Map {
	m := NewMap()
	for i := range n {
		m.Set(int64(i), int64(i))
	}
	return m
}

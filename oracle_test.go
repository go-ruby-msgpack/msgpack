// Copyright (c) the go-ruby-msgpack/msgpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package msgpack

import (
	"bytes"
	"encoding/hex"
	"math"
	"math/big"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// rubyBin locates a usable `ruby` once. The oracle tests skip themselves when it
// is absent (the qemu cross-arch lanes and the Windows lane), so the deterministic
// suite alone drives the 100% gate there. It also skips when the `msgpack` gem is
// not installed.
func rubyBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping msgpack-gem oracle")
	}
	if err := exec.Command(path, "-rmsgpack", "-e", "1").Run(); err != nil {
		t.Skip("msgpack gem not installed; skipping oracle")
	}
	return path
}

// gemPreamble sets up a Factory with the timestamp extension registered for Time,
// and a helper `pk(x)` that packs and prints the bytes as hex. Scripts binmode
// $stdout so Windows text-mode never pollutes the hex output (it does not here —
// the oracle skips on Windows — but the discipline mirrors the org convention).
const gemPreamble = `$stdout.binmode
ENV['TZ'] = 'UTC'
require 'msgpack'
require 'msgpack/time'
F = MessagePack::Factory.new
F.register_type(MessagePack::Timestamp::TYPE, Time,
  packer: MessagePack::Time::Packer, unpacker: MessagePack::Time::Unpacker)
def pk(x); print F.dump(x).unpack1('H*'); end
`

// gemPack runs the gem's packer on a Ruby expression and returns the bytes.
func gemPack(t *testing.T, bin, rubyExpr string) []byte {
	t.Helper()
	cmd := exec.Command(bin, "-e", gemPreamble+"pk("+rubyExpr+")")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error: %v\nexpr: %s\noutput:\n%s", err, rubyExpr, out)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("decode gem hex %q: %v", out, err)
	}
	return b
}

// gemUnpackInspect runs the gem's unpacker on the given bytes and returns the
// inspected (`p`) result, so we can confirm our Pack output decodes to the value.
func gemUnpackInspect(t *testing.T, bin string, b []byte) string {
	t.Helper()
	script := gemPreamble +
		"data = [" + "'" + hex.EncodeToString(b) + "'].pack('H*')\n" +
		"p F.unpack(data)\n"
	cmd := exec.Command(bin, "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ruby unpack error: %v\noutput:\n%s", err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

// TestOraclePackMatchesGem packs a corpus here and asserts the bytes are identical
// to the gem's Factory#dump for the equivalent Ruby value — the byte-for-byte
// parity claim, across every supported family and minimal-width boundary.
func TestOraclePackMatchesGem(t *testing.T) {
	bin := rubyBin(t)
	bi, _ := new(big.Int).SetString("18446744073709551615", 10) // 2**64-1

	cases := []struct {
		name string
		v    Value
		expr string // Ruby expression producing the same value
	}{
		{"nil", nil, "nil"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"zero", 0, "0"},
		{"fixintMax", 127, "127"},
		{"uint8", 128, "128"},
		{"uint8Max", 255, "255"},
		{"uint16", 256, "256"},
		{"uint16Max", 65535, "65535"},
		{"uint32", 65536, "65536"},
		{"uint32Max", int64(math.MaxUint32), "4294967295"},
		{"uint64", int64(1) << 32, "4294967296"},
		{"uint64Max", bi, "18446744073709551615"},
		{"negFixint", -1, "-1"},
		{"negFixintMin", -32, "-32"},
		{"int8", -33, "-33"},
		{"int8Min", -128, "-128"},
		{"int16", -129, "-129"},
		{"int16Min", -32768, "-32768"},
		{"int32", -32769, "-32769"},
		{"int32Min", math.MinInt32, "-2147483648"},
		{"int64", int64(math.MinInt32) - 1, "-2147483649"},
		{"int64Min", int64(math.MinInt64), "-9223372036854775808"},
		{"float", 1.5, "1.5"},
		{"floatZero", 0.0, "0.0"},
		{"floatInf", math.Inf(1), "Float::INFINITY"},
		{"emptyStr", "", `""`},
		{"str", "hi", `"hi"`},
		{"fixstrMax", strings.Repeat("a", 31), `"a"*31`},
		{"str8", strings.Repeat("a", 32), `"a"*32`},
		{"str8Max", strings.Repeat("a", 255), `"a"*255`},
		{"str16", strings.Repeat("a", 256), `"a"*256`},
		{"str16Max", strings.Repeat("a", 65535), `"a"*65535`},
		{"str32", strings.Repeat("a", 65536), `"a"*65536`},
		{"utf8", "héllo→", `"héllo→"`},
		{"symbol", Symbol("foo"), `"foo"`},
		{"bin", Bin([]byte("ab")), `"ab".b`},
		{"binNul", Bin([]byte{0, 1, 2}), `"\x00\x01\x02".b`},
		{"bin16", Bin(bytes.Repeat([]byte{7}, 256)), `("\x07".b)*256`},
		{"emptyArr", []any{}, "[]"},
		{"arr", []any{1, 2, 3}, "[1,2,3]"},
		{"nestedArr", []any{[]any{1}, []any{2}}, "[[1],[2]]"},
		{"arr16", makeIntSeq(16), "(0...16).to_a"},
		{"time32", time.Unix(1234567890, 0).UTC(), "Time.at(1234567890).utc"},
		{"time64", time.Unix(1234567890, 500).UTC(), "Time.at(1234567890, 500, :nsec).utc"},
		{"time96", time.Unix(-1, 0).UTC(), "Time.at(-1).utc"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Pack(c.v)
			if err != nil {
				t.Fatalf("Pack(%s): %v", c.name, err)
			}
			want := gemPack(t, bin, c.expr)
			if !bytes.Equal(got, want) {
				t.Errorf("Pack(%s) = %x, gem = %x", c.name, got, want)
			}
		})
	}
}

// TestOracleMapPackMatchesGem checks ordered-map packing matches the gem (which
// preserves Hash insertion order), including symbol-keyed entries (symbol→str).
func TestOracleMapPackMatchesGem(t *testing.T) {
	bin := rubyBin(t)

	t.Run("strkeys", func(t *testing.T) {
		m := NewMap()
		m.Set("a", 1)
		m.Set("b", []any{1, 2})
		m.Set("c", true)
		got, err := Pack(m)
		if err != nil {
			t.Fatal(err)
		}
		want := gemPack(t, bin, `{"a"=>1,"b"=>[1,2],"c"=>true}`)
		if !bytes.Equal(got, want) {
			t.Errorf("map = %x, gem = %x", got, want)
		}
	})

	t.Run("symkeys", func(t *testing.T) {
		m := NewMap()
		m.Set(Symbol("foo"), 1)
		m.Set(Symbol("bar"), 2)
		got, _ := Pack(m)
		// :foo / :bar pack as the strings "foo" / "bar" by default.
		want := gemPack(t, bin, `{"foo"=>1,"bar"=>2}`)
		if !bytes.Equal(got, want) {
			t.Errorf("symmap = %x, gem = %x", got, want)
		}
	})

	t.Run("map16", func(t *testing.T) {
		m := NewMap()
		var parts []string
		for i := range 16 {
			m.Set(int64(i), int64(i))
			parts = append(parts, intToS(i)+"=>"+intToS(i))
		}
		got, _ := Pack(m)
		want := gemPack(t, bin, "{"+strings.Join(parts, ",")+"}")
		if !bytes.Equal(got, want) {
			t.Errorf("map16 = %x, gem = %x", got, want)
		}
	})
}

// TestOracleRoundTripThroughGem packs here, has the gem unpack, and asserts the
// gem's inspected value — confirming our bytes decode to the intended value, not
// only that they equal the gem's own encoding.
func TestOracleRoundTripThroughGem(t *testing.T) {
	bin := rubyBin(t)
	cases := []struct {
		v       Value
		inspect string
	}{
		{int64(300), "300"},
		{"hello", `"hello"`},
		{[]any{int64(1), "two", true, nil}, `[1, "two", true, nil]`},
		{time.Unix(1234567890, 500).UTC(), `2009-02-13 23:31:30.0000005 +0000`},
	}
	for _, c := range cases {
		b, err := Pack(c.v)
		if err != nil {
			t.Fatal(err)
		}
		if got := gemUnpackInspect(t, bin, b); got != c.inspect {
			t.Errorf("gem unpack(Pack(%#v)) = %q, want %q", c.v, got, c.inspect)
		}
	}
}

// TestOracleUnpackGemBytes has the gem pack a value, then unpacks it here and
// checks the loaded shape — so the unpacker accepts genuine gem output, both
// directions of parity.
func TestOracleUnpackGemBytes(t *testing.T) {
	bin := rubyBin(t)

	b := gemPack(t, bin, `{"a"=>1,"b"=>[1,2,3],"c"=>{"d"=>true}}`)
	v, err := Unpack(b)
	if err != nil {
		t.Fatalf("Unpack gem map: %v", err)
	}
	m := v.(*Map)
	if bv, _ := m.Get("b"); !eqValue(bv, []any{int64(1), int64(2), int64(3)}) {
		t.Errorf("gem-packed b = %#v", bv)
	}
	cv, _ := m.Get("c")
	if dv, _ := cv.(*Map).Get("d"); dv != true {
		t.Errorf("gem-packed c.d = %#v", dv)
	}

	tb := gemPack(t, bin, `Time.at(1234567890, 500, :nsec).utc`)
	tv, err := Unpack(tb)
	if err != nil {
		t.Fatalf("Unpack gem time: %v", err)
	}
	tt, ok := tv.(time.Time)
	if !ok || tt.Unix() != 1234567890 || tt.Nanosecond() != 500 {
		t.Errorf("gem-packed time = %#v", tv)
	}

	bb := gemPack(t, bin, `"raw".b`)
	bv, err := Unpack(bb)
	if err != nil {
		t.Fatalf("Unpack gem bin: %v", err)
	}
	if _, ok := bv.(Bin); !ok || string(bv.(Bin)) != "raw" {
		t.Errorf("gem-packed bin = %#v", bv)
	}
}

// makeIntSeq returns []any{int64(0)..int64(n-1)} for the array-header oracle.
func makeIntSeq(n int) []any {
	a := make([]any, n)
	for i := range a {
		a[i] = int64(i)
	}
	return a
}

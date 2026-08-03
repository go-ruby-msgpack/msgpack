// Copyright (c) the go-ruby-msgpack/msgpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package msgpack

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// msgpackTestSuite is the canonical MessagePack conformance corpus
// kawanet/msgpack-test-suite (dist/msgpack-test-suite.json). It groups the spec's
// value families (nil/bool/bin/int/float/bignum/str/array/map/nested/timestamp/
// ext) and, for each value, lists every valid MessagePack byte encoding (the
// minimal one plus the wider fixint/8/16/32/64 and int-vs-float variants). It is
// vendored verbatim so this package gates on the reference's own vectors.
//
//go:embed msgpack-test-suite.json
var msgpackTestSuite []byte

// suiteCase is one corpus entry: the type-tagged value fields are ignored here
// (the round-trip check does not need them) and Encodings holds every dash-
// separated lowercase-hex encoding the spec sanctions for that value.
type suiteCase struct {
	group     string
	line      int // 1-based index within its group, for a stable ratchet key
	Encodings []string
}

// parseSuite decodes the corpus JSON into a flat, ordered list of cases.
func parseSuite(t *testing.T) []suiteCase {
	t.Helper()
	var raw map[string][]map[string]json.RawMessage
	if err := json.Unmarshal(msgpackTestSuite, &raw); err != nil {
		t.Fatalf("decode corpus: %v", err)
	}
	groups := make([]string, 0, len(raw))
	for g := range raw {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	var cases []suiteCase
	for _, g := range groups {
		for i, entry := range raw[g] {
			var encs []string
			if err := json.Unmarshal(entry["msgpack"], &encs); err != nil {
				t.Fatalf("%s[%d]: decode msgpack encodings: %v", g, i, err)
			}
			cases = append(cases, suiteCase{group: g, line: i + 1, Encodings: encs})
		}
	}
	return cases
}

// key is the stable ratchet identifier for a case, e.g. "41.map.yaml#2".
func (c suiteCase) key() string {
	return c.group + "#" + itoa(c.line)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// decodeHex turns a dash-separated lowercase-hex encoding ("de-00-01") into bytes.
func decodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// encodeHex renders bytes back to the corpus's dash-separated lowercase hex.
func encodeHex(b []byte) string {
	parts := make([]string, len(b))
	for i, x := range b {
		parts[i] = hex.EncodeToString([]byte{x})
	}
	return strings.Join(parts, "-")
}

// roundTrips reports whether every listed encoding of a case Unpacks without
// error and re-Packs to a byte string the corpus also lists for that value. This
// is the MessagePack conformance check: each wide-header / int-vs-float variant
// must decode, and the decoded value must serialise back to one of the spec's
// sanctioned encodings (the codec's canonical form).
func roundTrips(t *testing.T, c suiteCase) bool {
	set := make(map[string]bool, len(c.Encodings))
	for _, e := range c.Encodings {
		set[e] = true
	}
	for _, e := range c.Encodings {
		v, err := Unpack(decodeHex(t, e))
		if err != nil {
			return false
		}
		out, err := Pack(v)
		if err != nil {
			return false
		}
		if !set[encodeHex(out)] {
			return false
		}
	}
	return true
}

// suiteKnownFailing is the frozen set of msgpack-test-suite cases (keyed by
// "group#line") this package does not yet round-trip against the spec's
// sanctioned encodings. It is a shrink-only conformance RATCHET: every case NOT
// listed here must round-trip, so no change may introduce a new encoding
// regression, and a listed case that starts passing is reported so the entry can
// be removed. Baseline captured 2026-08-03.
var suiteKnownFailing = map[string]bool{}

// TestMsgpackTestSuiteConformance is the differential round-trip gate against the
// canonical kawanet/msgpack-test-suite corpus. Every case outside
// suiteKnownFailing must decode all of its listed encodings and re-encode to a
// spec-sanctioned form; a new failure fails CI and a listed case that now passes
// is reported so the ratchet can be tightened.
func TestMsgpackTestSuiteConformance(t *testing.T) {
	cases := parseSuite(t)
	if len(cases) < 80 {
		t.Fatalf("expected ~85 corpus cases, parsed %d", len(cases))
	}
	pass, vectors, vectorsPass := 0, 0, 0
	var newFail, fixed []string
	for _, c := range cases {
		ok := roundTrips(t, c)
		vectors += len(c.Encodings)
		if ok {
			pass++
			vectorsPass += len(c.Encodings)
		}
		switch {
		case ok && suiteKnownFailing[c.key()]:
			fixed = append(fixed, c.key())
		case !ok && !suiteKnownFailing[c.key()]:
			newFail = append(newFail, c.key())
		}
	}
	t.Logf("msgpack-test-suite: %d/%d cases round-trip (%.2f%%), %d/%d encoding "+
		"vectors, %d known gaps", pass, len(cases), 100*float64(pass)/float64(len(cases)),
		vectorsPass, vectors, len(suiteKnownFailing))
	if len(fixed) > 0 {
		sort.Strings(fixed)
		t.Errorf("cases now passing that are still listed in suiteKnownFailing: %v\n"+
			"remove them to tighten the ratchet", fixed)
	}
	if len(newFail) > 0 {
		sort.Strings(newFail)
		t.Errorf("REGRESSION: %d msgpack-test-suite case(s) that must round-trip now "+
			"fail: %v", len(newFail), newFail)
	}
}

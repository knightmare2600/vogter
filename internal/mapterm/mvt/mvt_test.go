package mvt

import (
	"bytes"
	"compress/gzip"
	"reflect"
	"testing"
)

// The polygon and line from the specification's examples.
var specTile = encodeTile(testLayer{
	name:   "water",
	keys:   []string{"class", "rank"},
	values: [][]byte{strValue("lake"), intValue(3)},
	features: []testFeature{
		{id: 7, typ: Polygon, tags: []uint32{0, 0, 1, 1},
			geom: []uint32{cmd(1, 1), zz(3), zz(6), cmd(2, 2), zz(5), zz(6), zz(12), zz(22), cmd(7, 1)}},
		{typ: LineString, geom: []uint32{cmd(1, 1), zz(2), zz(2), cmd(2, 2), zz(0), zz(8), zz(8), zz(0),
			cmd(1, 1), zz(-9), zz(-10), cmd(2, 1), zz(1), zz(1)}},
		{typ: Point, geom: []uint32{cmd(1, 2), zz(5), zz(7), zz(-2), zz(1)}},
	},
})

func TestDecodeSpecExamples(t *testing.T) {
	tile, err := Decode(specTile)
	if err != nil {
		t.Fatal(err)
	}
	l := tile.Layers["water"]
	if l == nil || l.Extent != 4096 || len(l.Features) != 3 {
		t.Fatalf("%+v", l)
	}
	poly := l.Features[0]
	if poly.ID != 7 || poly.Props["class"] != "lake" || poly.Props["rank"] != int64(3) {
		t.Errorf("%+v", poly)
	}
	if want := [][]Pt{{{3, 6}, {8, 12}, {20, 34}, {3, 6}}}; !reflect.DeepEqual(poly.Geometry, want) {
		t.Errorf("polygon %v", poly.Geometry)
	}
	if want := [][]Pt{{{2, 2}, {2, 10}, {10, 10}}, {{1, 0}, {2, 1}}}; !reflect.DeepEqual(l.Features[1].Geometry, want) {
		t.Errorf("multi-line %v", l.Features[1].Geometry)
	}
	if want := [][]Pt{{{5, 7}, {3, 8}}}; !reflect.DeepEqual(l.Features[2].Geometry, want) {
		t.Errorf("multi-point %v", l.Features[2].Geometry)
	}
}

func TestDecodeGzipAndExtent(t *testing.T) {
	raw := encodeTile(testLayer{name: "x", extent: 512, features: []testFeature{{typ: Point, geom: []uint32{cmd(1, 1), zz(1), zz(1)}}}})
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(raw)
	zw.Close()
	tile, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if tile.Layers["x"].Extent != 512 {
		t.Error(tile.Layers["x"].Extent)
	}
}

func TestDecodeEmpty(t *testing.T) {
	tile, err := Decode(nil)
	if err != nil || len(tile.Layers) != 0 {
		t.Fatalf("%v %v", tile, err)
	}
}

func TestDecodeRejectsBadInput(t *testing.T) {
	bad := map[string][]byte{
		"truncated":      specTile[:len(specTile)-3],
		"bad command":    encodeTile(testLayer{name: "x", features: []testFeature{{typ: LineString, geom: []uint32{cmd(5, 1), 0, 0}}}}),
		"short geometry": encodeTile(testLayer{name: "x", features: []testFeature{{typ: LineString, geom: []uint32{cmd(1, 3), zz(1)}}}}),
		"tag range":      encodeTile(testLayer{name: "x", features: []testFeature{{typ: Point, tags: []uint32{4, 4}, geom: []uint32{cmd(1, 1), 0, 0}}}}),
		"odd tags":       encodeTile(testLayer{name: "x", keys: []string{"k"}, features: []testFeature{{typ: Point, tags: []uint32{0}, geom: []uint32{cmd(1, 1), 0, 0}}}}),
		"zero extent":    append(encodeTile(), encodeTile(testLayer{name: "x"})...),
	}
	// A zero extent needs a hand-made layer, as the encoder omits zero.
	var le enc
	le.bytes(1, []byte("x"))
	le.key(5, wireVarint)
	le.varint(0)
	var te enc
	te.bytes(3, le.b)
	bad["zero extent"] = te.b
	for name, data := range bad {
		if _, err := Decode(data); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestUnknownFieldsAreSkipped(t *testing.T) {
	var e enc
	e.key(9, wireFixed32)
	e.b = append(e.b, 1, 2, 3, 4)
	e.key(10, wireFixed64)
	e.b = append(e.b, make([]byte, 8)...)
	data := append(e.b, specTile...)
	if _, err := Decode(data); err != nil {
		t.Fatal(err)
	}
}

func FuzzDecode(f *testing.F) {
	f.Add(specTile)
	f.Add([]byte{0x1a, 0x05, 0x0a, 0x01, 'x', 0x28, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		// Any input must return, never panic or hang.
		_, _ = Decode(data)
	})
}

func TestValueTypes(t *testing.T) {
	// Each value encoding the specification allows: string, float, double,
	// int64, uint64, sint64 and bool.
	var vals [][]byte
	add := func(f func(e *enc)) {
		var e enc
		f(&e)
		vals = append(vals, e.b)
	}
	add(func(e *enc) { e.bytes(1, []byte("rail")) })
	add(func(e *enc) { e.key(2, wireFixed32); e.b = append(e.b, 0, 0, 0xc0, 0x3f) })             // 1.5
	add(func(e *enc) { e.key(3, wireFixed64); e.b = append(e.b, 0, 0, 0, 0, 0, 0, 0x04, 0x40) }) // 2.5
	add(func(e *enc) { e.key(4, wireVarint); e.varint(4) })
	add(func(e *enc) { e.key(5, wireVarint); e.varint(7) })
	add(func(e *enc) { e.key(6, wireVarint); e.varint(uint64(zz(-3))) })
	add(func(e *enc) { e.key(7, wireVarint); e.varint(1) })
	keys := []string{"s", "f", "d", "i", "u", "z", "b"}
	tags := []uint32{}
	for i := range keys {
		tags = append(tags, uint32(i), uint32(i))
	}
	tile, err := Decode(encodeTile(testLayer{name: "x", keys: keys, values: vals,
		features: []testFeature{{typ: Point, tags: tags, geom: []uint32{cmd(1, 1), 0, 0}}}}))
	if err != nil {
		t.Fatal(err)
	}
	p := tile.Layers["x"].Features[0].Props
	want := map[string]any{"s": "rail", "f": 1.5, "d": 2.5, "i": int64(4), "u": uint64(7), "z": int64(-3), "b": true}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("%#v", p)
	}
}

func TestDecodeRejectsCorruptGzipAndHugeTiles(t *testing.T) {
	if _, err := Decode([]byte{0x1f, 0x8b, 0, 1, 2}); err == nil {
		t.Error("a corrupt gzip stream must fail")
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(make([]byte, maxTileBytes+10))
	zw.Close()
	if _, err := Decode(buf.Bytes()); err == nil {
		t.Error("a tile that inflates past the limit must be refused")
	}
}

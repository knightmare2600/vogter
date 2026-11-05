package mvt

// A minimal encoder for building test tiles, following the specification.

type enc struct{ b []byte }

func (e *enc) varint(v uint64) {
	for v >= 0x80 {
		e.b = append(e.b, byte(v)|0x80)
		v >>= 7
	}
	e.b = append(e.b, byte(v))
}

func (e *enc) key(field, wire int) { e.varint(uint64(field<<3 | wire)) }

func (e *enc) bytes(field int, b []byte) {
	e.key(field, wireBytes)
	e.varint(uint64(len(b)))
	e.b = append(e.b, b...)
}

func (e *enc) packed(field int, vs []uint32) {
	var p enc
	for _, v := range vs {
		p.varint(uint64(v))
	}
	e.bytes(field, p.b)
}

func zz(v int32) uint32 { return uint32((v << 1) ^ (v >> 31)) }

func cmd(id, count uint32) uint32 { return id | count<<3 }

type testFeature struct {
	id   uint64
	typ  GeomType
	tags []uint32
	geom []uint32
}

type testLayer struct {
	name     string
	extent   uint64
	keys     []string
	values   [][]byte
	features []testFeature
}

func strValue(s string) []byte {
	var e enc
	e.bytes(1, []byte(s))
	return e.b
}

func intValue(v int64) []byte {
	var e enc
	e.key(4, wireVarint)
	e.varint(uint64(v))
	return e.b
}

func encodeTile(layers ...testLayer) []byte {
	var t enc
	for _, l := range layers {
		var le enc
		le.key(15, wireVarint)
		le.varint(2)
		le.bytes(1, []byte(l.name))
		for _, f := range l.features {
			var fe enc
			if f.id != 0 {
				fe.key(1, wireVarint)
				fe.varint(f.id)
			}
			if len(f.tags) > 0 {
				fe.packed(2, f.tags)
			}
			fe.key(3, wireVarint)
			fe.varint(uint64(f.typ))
			fe.packed(4, f.geom)
			le.bytes(2, fe.b)
		}
		for _, k := range l.keys {
			le.bytes(3, []byte(k))
		}
		for _, v := range l.values {
			le.bytes(4, v)
		}
		if l.extent != 0 {
			le.key(5, wireVarint)
			le.varint(l.extent)
		}
		t.bytes(3, le.b)
	}
	return t.b
}

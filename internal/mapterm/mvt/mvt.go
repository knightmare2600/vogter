// Package mvt decodes Mapbox Vector Tiles (specification 2.1): layers of
// features whose geometry is encoded as commands in tile coordinates. It reads
// the protobuf wire format directly, with the standard library only.
package mvt

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"math"
)

// GeomType is a feature's geometry type.
type GeomType int

// Geometry types from the specification.
const (
	Unknown GeomType = iota
	Point
	LineString
	Polygon
)

// maxTileBytes bounds a decompressed tile, so a hostile response cannot
// exhaust memory.
const maxTileBytes = 16 << 20

// Pt is a position in tile coordinates, 0 to Extent along each axis.
type Pt struct{ X, Y int32 }

// Feature is one decoded feature. Geometry holds points for Point, one path per
// line for LineString, and every ring for Polygon, in tile coordinates.
type Feature struct {
	ID       uint64
	Type     GeomType
	Props    map[string]any
	Geometry [][]Pt
}

// Layer is one named layer.
type Layer struct {
	Name     string
	Extent   int
	Features []Feature
}

// Tile is a decoded tile, by layer name.
type Tile struct {
	Layers map[string]*Layer
}

// Decode parses a tile, gzipped or not.
func Decode(data []byte) (*Tile, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("mvt: %w", err)
		}
		plain, err := io.ReadAll(io.LimitReader(zr, maxTileBytes+1))
		if err != nil {
			return nil, fmt.Errorf("mvt: %w", err)
		}
		if len(plain) > maxTileBytes {
			return nil, errors.New("mvt: tile too large")
		}
		data = plain
	}
	t := &Tile{Layers: map[string]*Layer{}}
	r := reader{buf: data}
	for !r.done() {
		field, wire, err := r.key()
		if err != nil {
			return nil, err
		}
		if field != 3 || wire != wireBytes {
			if err := r.skip(wire); err != nil {
				return nil, err
			}
			continue
		}
		raw, err := r.bytes()
		if err != nil {
			return nil, err
		}
		l, err := decodeLayer(raw)
		if err != nil {
			return nil, err
		}
		t.Layers[l.Name] = l
	}
	return t, nil
}

func decodeLayer(data []byte) (*Layer, error) {
	l := &Layer{Extent: 4096}
	var keys []string
	var values []any
	var rawFeatures [][]byte
	r := reader{buf: data}
	for !r.done() {
		field, wire, err := r.key()
		if err != nil {
			return nil, err
		}
		switch {
		case field == 1 && wire == wireBytes:
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			l.Name = string(b)
		case field == 2 && wire == wireBytes:
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			rawFeatures = append(rawFeatures, b)
		case field == 3 && wire == wireBytes:
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			keys = append(keys, string(b))
		case field == 4 && wire == wireBytes:
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			v, err := decodeValue(b)
			if err != nil {
				return nil, err
			}
			values = append(values, v)
		case field == 5 && wire == wireVarint:
			v, err := r.varint()
			if err != nil {
				return nil, err
			}
			if v == 0 || v > 1<<20 {
				return nil, fmt.Errorf("mvt: layer %q: bad extent %d", l.Name, v)
			}
			l.Extent = int(v)
		default:
			if err := r.skip(wire); err != nil {
				return nil, err
			}
		}
	}
	// Features refer to keys and values by index, which may follow them.
	for _, raw := range rawFeatures {
		f, err := decodeFeature(raw, keys, values)
		if err != nil {
			return nil, fmt.Errorf("mvt: layer %q: %w", l.Name, err)
		}
		l.Features = append(l.Features, f)
	}
	return l, nil
}

func decodeFeature(data []byte, keys []string, values []any) (Feature, error) {
	var f Feature
	var tags, geom []uint32
	r := reader{buf: data}
	for !r.done() {
		field, wire, err := r.key()
		if err != nil {
			return f, err
		}
		switch {
		case field == 1 && wire == wireVarint:
			if f.ID, err = r.varint(); err != nil {
				return f, err
			}
		case field == 2 && wire == wireBytes:
			if tags, err = r.packed(tags); err != nil {
				return f, err
			}
		case field == 3 && wire == wireVarint:
			v, err := r.varint()
			if err != nil {
				return f, err
			}
			f.Type = GeomType(v)
		case field == 4 && wire == wireBytes:
			if geom, err = r.packed(geom); err != nil {
				return f, err
			}
		default:
			if err := r.skip(wire); err != nil {
				return f, err
			}
		}
	}
	if len(tags)%2 != 0 {
		return f, errors.New("odd tag count")
	}
	f.Props = make(map[string]any, len(tags)/2)
	for i := 0; i < len(tags); i += 2 {
		k, v := int(tags[i]), int(tags[i+1])
		if k >= len(keys) || v >= len(values) {
			return f, errors.New("tag index out of range")
		}
		f.Props[keys[k]] = values[v]
	}
	var err error
	f.Geometry, err = decodeGeometry(geom, f.Type)
	return f, err
}

// Geometry commands.
const (
	cmdMoveTo    = 1
	cmdLineTo    = 2
	cmdClosePath = 7
)

func zigzag(v uint32) int32 { return int32(v>>1) ^ -int32(v&1) }

// decodeGeometry follows the command stream. A cursor carries across commands
// and parts, as the specification requires.
func decodeGeometry(cmds []uint32, typ GeomType) ([][]Pt, error) {
	var parts [][]Pt
	var cur []Pt
	var x, y int32
	for i := 0; i < len(cmds); {
		id, count := cmds[i]&7, int(cmds[i]>>3)
		i++
		switch id {
		case cmdMoveTo, cmdLineTo:
			if count > (len(cmds)-i)/2 {
				return nil, errors.New("geometry command runs past its data")
			}
			for range count {
				x += zigzag(cmds[i])
				y += zigzag(cmds[i+1])
				i += 2
				if id == cmdMoveTo && typ != Point {
					if len(cur) > 0 {
						parts = append(parts, cur)
					}
					cur = nil
				}
				cur = append(cur, Pt{x, y})
			}
		case cmdClosePath:
			if len(cur) > 0 {
				cur = append(cur, cur[0])
			}
		default:
			return nil, fmt.Errorf("unknown geometry command %d", id)
		}
	}
	if len(cur) > 0 {
		parts = append(parts, cur)
	}
	return parts, nil
}

func decodeValue(data []byte) (any, error) {
	r := reader{buf: data}
	var out any
	for !r.done() {
		field, wire, err := r.key()
		if err != nil {
			return nil, err
		}
		switch {
		case field == 1 && wire == wireBytes:
			b, err := r.bytes()
			if err != nil {
				return nil, err
			}
			out = string(b)
		case field == 2 && wire == wireFixed32:
			v, err := r.fixed32()
			if err != nil {
				return nil, err
			}
			out = float64(math.Float32frombits(v))
		case field == 3 && wire == wireFixed64:
			v, err := r.fixed64()
			if err != nil {
				return nil, err
			}
			out = math.Float64frombits(v)
		case (field == 4 || field == 5 || field == 6 || field == 7) && wire == wireVarint:
			v, err := r.varint()
			if err != nil {
				return nil, err
			}
			switch field {
			case 4:
				out = int64(v)
			case 5:
				out = v
			case 6:
				out = int64(v>>1) ^ -int64(v&1)
			case 7:
				out = v != 0
			}
		default:
			if err := r.skip(wire); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// Wire types.
const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireFixed32 = 5
)

var errTruncated = errors.New("mvt: truncated data")

type reader struct {
	buf []byte
	pos int
}

func (r *reader) done() bool { return r.pos >= len(r.buf) }

func (r *reader) varint() (uint64, error) {
	var v uint64
	for shift := uint(0); shift < 64; shift += 7 {
		if r.pos >= len(r.buf) {
			return 0, errTruncated
		}
		b := r.buf[r.pos]
		r.pos++
		v |= uint64(b&0x7f) << shift
		if b < 0x80 {
			return v, nil
		}
	}
	return 0, errors.New("mvt: varint too long")
}

func (r *reader) key() (int, int, error) {
	k, err := r.varint()
	if err != nil {
		return 0, 0, err
	}
	return int(k >> 3), int(k & 7), nil
}

func (r *reader) bytes() ([]byte, error) {
	n, err := r.varint()
	if err != nil {
		return nil, err
	}
	if n > uint64(len(r.buf)-r.pos) {
		return nil, errTruncated
	}
	b := r.buf[r.pos : r.pos+int(n)]
	r.pos += int(n)
	return b, nil
}

func (r *reader) fixed32() (uint32, error) {
	if len(r.buf)-r.pos < 4 {
		return 0, errTruncated
	}
	b := r.buf[r.pos:]
	r.pos += 4
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24, nil
}

func (r *reader) fixed64() (uint64, error) {
	lo, err := r.fixed32()
	if err != nil {
		return 0, err
	}
	hi, err := r.fixed32()
	return uint64(lo) | uint64(hi)<<32, err
}

func (r *reader) packed(into []uint32) ([]uint32, error) {
	b, err := r.bytes()
	if err != nil {
		return nil, err
	}
	sub := reader{buf: b}
	for !sub.done() {
		v, err := sub.varint()
		if err != nil {
			return nil, err
		}
		into = append(into, uint32(v))
	}
	return into, nil
}

func (r *reader) skip(wire int) error {
	switch wire {
	case wireVarint:
		_, err := r.varint()
		return err
	case wireFixed64:
		_, err := r.fixed64()
		return err
	case wireBytes:
		_, err := r.bytes()
		return err
	case wireFixed32:
		_, err := r.fixed32()
		return err
	}
	return fmt.Errorf("mvt: unsupported wire type %d", wire)
}

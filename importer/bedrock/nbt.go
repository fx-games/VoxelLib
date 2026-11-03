package bedrock

import (
	"encoding/binary"
	"errors"
	"math"
)

// Tag is a decoded NBT value: int8..int64 as int64, float32/64 as float64,
// string, []byte (byte array), []any (list), map[string]any (compound),
// []int32, []int64.
type Tag = any

// nbtReader reads Bedrock's little-endian NBT (as stored on disk, not the
// network varint form).
type nbtReader struct {
	p   []byte
	err error
}

func (r *nbtReader) take(n int) []byte {
	if r.err != nil || n < 0 || n > len(r.p) {
		r.err = errors.New("nbt: short")
		return make([]byte, max(n, 0))
	}
	b := r.p[:n]
	r.p = r.p[n:]
	return b
}

func (r *nbtReader) u8() byte    { return r.take(1)[0] }
func (r *nbtReader) i16() int16  { return int16(binary.LittleEndian.Uint16(r.take(2))) }
func (r *nbtReader) i32() int32  { return int32(binary.LittleEndian.Uint32(r.take(4))) }
func (r *nbtReader) i64() int64  { return int64(binary.LittleEndian.Uint64(r.take(8))) }
func (r *nbtReader) str() string { return string(r.take(int(uint16(r.i16())))) }

// root reads a named root tag (a compound) and returns its name and value.
func (r *nbtReader) root() (string, map[string]any) {
	typ := r.u8()
	if typ != 10 {
		r.err = errors.New("nbt: root is not a compound")
		return "", nil
	}
	name := r.str()
	c, _ := r.payload(10, 0).(map[string]any)
	return name, c
}

func (r *nbtReader) payload(typ byte, depth int) Tag {
	if depth > 64 {
		r.err = errors.New("nbt: too deep")
		return nil
	}
	switch typ {
	case 1:
		return int64(int8(r.u8()))
	case 2:
		return int64(r.i16())
	case 3:
		return int64(r.i32())
	case 4:
		return r.i64()
	case 5:
		return float64(math.Float32frombits(uint32(r.i32())))
	case 6:
		return math.Float64frombits(uint64(r.i64()))
	case 7:
		return append([]byte(nil), r.take(int(r.i32()))...)
	case 8:
		return r.str()
	case 9:
		et := r.u8()
		n := int(r.i32())
		if n < 0 || n > len(r.p) {
			r.err = errors.New("nbt: bad list")
			return nil
		}
		l := make([]any, 0, n)
		for range n {
			l = append(l, r.payload(et, depth+1))
		}
		return l
	case 10:
		c := map[string]any{}
		for r.err == nil {
			t := r.u8()
			if t == 0 {
				break
			}
			name := r.str()
			c[name] = r.payload(t, depth+1)
		}
		return c
	case 11:
		n := int(r.i32())
		if n < 0 || 4*n > len(r.p) {
			r.err = errors.New("nbt: bad int array")
			return nil
		}
		a := make([]int32, n)
		for i := range a {
			a[i] = r.i32()
		}
		return a
	case 12:
		n := int(r.i32())
		if n < 0 || 8*n > len(r.p) {
			r.err = errors.New("nbt: bad long array")
			return nil
		}
		a := make([]int64, n)
		for i := range a {
			a[i] = r.i64()
		}
		return a
	}
	r.err = errors.New("nbt: unknown tag")
	return nil
}

// Int reads an integer tag of a compound (0 if missing).
func Int(c map[string]any, k string) int64 {
	v, _ := c[k].(int64)
	return v
}

// Str reads a string tag of a compound.
func Str(c map[string]any, k string) string {
	v, _ := c[k].(string)
	return v
}

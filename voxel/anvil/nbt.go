// Package anvil reads and writes Minecraft Java Edition worlds: NBT, region
// files (.mca) and chunk NBT for the 1.13 - 1.21+ layouts.
package anvil

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// NBT tag types.
const (
	TagEnd byte = iota
	TagByte
	TagShort
	TagInt
	TagLong
	TagFloat
	TagDouble
	TagByteArray
	TagString
	TagList
	TagCompound
	TagIntArray
	TagLongArray
)

// Compound is an NBT compound. Values are int8, int16, int32, int64, float32,
// float64, []byte, string, List, Compound, []int32, []int64.
type Compound map[string]any

// List is an NBT list with its element type.
type List struct {
	Type  byte
	Items []any
}

func (c Compound) Compound(k string) Compound { v, _ := c[k].(Compound); return v }
func (c Compound) List(k string) List         { v, _ := c[k].(List); return v }
func (c Compound) String(k string) string     { v, _ := c[k].(string); return v }
func (c Compound) Bytes(k string) []byte      { v, _ := c[k].([]byte); return v }
func (c Compound) Longs(k string) []int64     { v, _ := c[k].([]int64); return v }

// Int returns any integer tag widened to int.
func (c Compound) Int(k string) int {
	switch v := c[k].(type) {
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}

// Has reports whether a key exists.
func (c Compound) Has(k string) bool { _, ok := c[k]; return ok }

// ReadNBT reads an uncompressed named root compound.
func ReadNBT(r io.Reader) (Compound, string, error) {
	br := bufio.NewReader(r)
	d := &decoder{r: br}
	t := d.u8()
	if d.err != nil {
		return nil, "", d.err
	}
	if t != TagCompound {
		return nil, "", fmt.Errorf("nbt: root is tag %d, want compound", t)
	}
	name := d.str()
	v := d.payload(TagCompound, 0)
	if d.err != nil {
		return nil, "", d.err
	}
	return v.(Compound), name, nil
}

type decoder struct {
	r   *bufio.Reader
	err error
	buf [8]byte
}

func (d *decoder) read(n int) []byte {
	if d.err != nil {
		return d.buf[:n]
	}
	_, d.err = io.ReadFull(d.r, d.buf[:n])
	return d.buf[:n]
}
func (d *decoder) u8() byte    { return d.read(1)[0] }
func (d *decoder) u16() uint16 { return binary.BigEndian.Uint16(d.read(2)) }
func (d *decoder) u32() uint32 { return binary.BigEndian.Uint32(d.read(4)) }
func (d *decoder) u64() uint64 { return binary.BigEndian.Uint64(d.read(8)) }

func (d *decoder) str() string {
	n := int(d.u16())
	if d.err != nil {
		return ""
	}
	b := make([]byte, n)
	_, d.err = io.ReadFull(d.r, b)
	return string(b)
}

func (d *decoder) length() int {
	n := int(int32(d.u32()))
	if n < 0 || n > 1<<26 {
		d.err = errors.New("nbt: bad length")
		return 0
	}
	return n
}

func (d *decoder) payload(t byte, depth int) any {
	if depth > 512 {
		d.err = errors.New("nbt: too deep")
		return nil
	}
	switch t {
	case TagByte:
		return int8(d.u8())
	case TagShort:
		return int16(d.u16())
	case TagInt:
		return int32(d.u32())
	case TagLong:
		return int64(d.u64())
	case TagFloat:
		return math.Float32frombits(d.u32())
	case TagDouble:
		return math.Float64frombits(d.u64())
	case TagByteArray:
		n := d.length()
		b := make([]byte, n)
		if d.err == nil {
			_, d.err = io.ReadFull(d.r, b)
		}
		return b
	case TagString:
		return d.str()
	case TagList:
		et := d.u8()
		n := d.length()
		l := List{Type: et, Items: make([]any, 0, n)}
		for i := 0; i < n && d.err == nil; i++ {
			l.Items = append(l.Items, d.payload(et, depth+1))
		}
		return l
	case TagCompound:
		c := Compound{}
		for d.err == nil {
			ct := d.u8()
			if ct == TagEnd {
				break
			}
			name := d.str()
			c[name] = d.payload(ct, depth+1)
		}
		return c
	case TagIntArray:
		n := d.length()
		a := make([]int32, n)
		for i := range a {
			a[i] = int32(d.u32())
		}
		return a
	case TagLongArray:
		n := d.length()
		a := make([]int64, n)
		for i := range a {
			a[i] = int64(d.u64())
		}
		return a
	}
	d.err = fmt.Errorf("nbt: unknown tag %d", t)
	return nil
}

// WriteNBT writes a named root compound, uncompressed.
func WriteNBT(w io.Writer, name string, c Compound) error {
	bw := bufio.NewWriter(w)
	e := &encoder{w: bw}
	e.u8(TagCompound)
	e.str(name)
	e.payload(c)
	if e.err != nil {
		return e.err
	}
	return bw.Flush()
}

type encoder struct {
	w   *bufio.Writer
	err error
	buf [8]byte
}

func (e *encoder) write(b []byte) {
	if e.err == nil {
		_, e.err = e.w.Write(b)
	}
}
func (e *encoder) u8(v byte) { e.write([]byte{v}) }
func (e *encoder) u16(v uint16) {
	binary.BigEndian.PutUint16(e.buf[:2], v)
	e.write(e.buf[:2])
}
func (e *encoder) u32(v uint32) {
	binary.BigEndian.PutUint32(e.buf[:4], v)
	e.write(e.buf[:4])
}
func (e *encoder) u64(v uint64) {
	binary.BigEndian.PutUint64(e.buf[:8], v)
	e.write(e.buf[:8])
}
func (e *encoder) str(s string) { e.u16(uint16(len(s))); e.write([]byte(s)) }

func tagOf(v any) byte {
	switch v.(type) {
	case int8:
		return TagByte
	case int16:
		return TagShort
	case int32:
		return TagInt
	case int64:
		return TagLong
	case float32:
		return TagFloat
	case float64:
		return TagDouble
	case []byte:
		return TagByteArray
	case string:
		return TagString
	case List:
		return TagList
	case Compound:
		return TagCompound
	case []int32:
		return TagIntArray
	case []int64:
		return TagLongArray
	}
	return TagEnd
}

func (e *encoder) payload(v any) {
	switch x := v.(type) {
	case int8:
		e.u8(byte(x))
	case int16:
		e.u16(uint16(x))
	case int32:
		e.u32(uint32(x))
	case int64:
		e.u64(uint64(x))
	case float32:
		e.u32(math.Float32bits(x))
	case float64:
		e.u64(math.Float64bits(x))
	case []byte:
		e.u32(uint32(len(x)))
		e.write(x)
	case string:
		e.str(x)
	case List:
		t := x.Type
		if len(x.Items) > 0 {
			t = tagOf(x.Items[0])
		}
		e.u8(t)
		e.u32(uint32(len(x.Items)))
		for _, it := range x.Items {
			e.payload(it)
		}
	case Compound:
		for k, val := range x {
			t := tagOf(val)
			if t == TagEnd {
				continue
			}
			e.u8(t)
			e.str(k)
			e.payload(val)
		}
		e.u8(TagEnd)
	case []int32:
		e.u32(uint32(len(x)))
		for _, i := range x {
			e.u32(uint32(i))
		}
	case []int64:
		e.u32(uint32(len(x)))
		for _, i := range x {
			e.u64(uint64(i))
		}
	default:
		e.err = fmt.Errorf("nbt: cannot encode %T", v)
	}
}

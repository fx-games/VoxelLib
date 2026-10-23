package voxel

import "math/bits"

// PalettedContainer stores 4096 block states the way the reference's
// PalettedContainer does:
//
//   - bits == 0: single value, no data array at all (a section of pure air or
//     pure stone costs a few bytes),
//   - bits 4..8: indirect, a small local palette plus bit-packed indices,
//   - bits > 8: direct, global state ids bit-packed.
//
// Entries never span two longs (the 1.16+ layout), so Anvil block_states data
// can be adopted as-is without repacking.
type PalettedContainer struct {
	bits    uint8
	palette []StateID // indirect / single value
	data    []uint64
}

const directBits = 16

func NewPalettedContainer(fill StateID) *PalettedContainer {
	return &PalettedContainer{palette: []StateID{fill}}
}

func longsFor(b uint8) int {
	per := 64 / int(b)
	return (SectionVolume + per - 1) / per
}

// MayContain reports whether any state matching f may be stored: false
// only when no palette entry matches (a direct container always may).
func (p *PalettedContainer) MayContain(f func(StateID) bool) bool {
	if p.bits > 8 {
		return true
	}
	for _, s := range p.palette {
		if f(s) {
			return true
		}
	}
	return false
}

// Get returns the state at index (y<<8|z<<4|x).
func (p *PalettedContainer) Get(i int) StateID {
	if p.bits == 0 {
		return p.palette[0]
	}
	v := p.raw(i)
	if p.bits > 8 {
		return StateID(v)
	}
	return p.palette[v]
}

func (p *PalettedContainer) raw(i int) uint32 {
	b := uint(p.bits)
	per := 64 / b
	w := p.data[uint(i)/per]
	return uint32(w>>((uint(i)%per)*b)) & (1<<b - 1)
}

func (p *PalettedContainer) setRaw(i int, v uint32) {
	b := uint(p.bits)
	per := 64 / b
	idx := uint(i) / per
	sh := (uint(i) % per) * b
	mask := uint64(1<<b-1) << sh
	p.data[idx] = p.data[idx]&^mask | uint64(v)<<sh
}

// Set stores a state and returns the previous one, growing the palette or
// switching to direct mode as needed.
func (p *PalettedContainer) Set(i int, s StateID) StateID {
	old := p.Get(i)
	if old == s {
		return old
	}
	if p.bits > 8 {
		p.setRaw(i, uint32(s))
		return old
	}
	idx := -1
	for k, v := range p.palette {
		if v == s {
			idx = k
			break
		}
	}
	if idx < 0 {
		idx = len(p.palette)
		need := uint8(bits.Len(uint(idx)))
		if need > p.bits || p.bits == 0 {
			if need < 4 {
				need = 4
			}
			p.resize(need)
			if p.bits > 8 {
				p.setRaw(i, uint32(s))
				return old
			}
		}
		p.palette = append(p.palette, s)
	}
	p.setRaw(i, uint32(idx))
	return old
}

func (p *PalettedContainer) resize(nb uint8) {
	if nb > 8 {
		nb = directBits
	}
	n := &PalettedContainer{bits: nb, palette: p.palette, data: make([]uint64, longsFor(nb))}
	for i := 0; i < SectionVolume; i++ {
		var v uint32
		if p.bits != 0 {
			v = p.raw(i)
		}
		if nb > 8 {
			if p.bits <= 8 {
				v = uint32(p.palette[v])
			}
		}
		n.setRaw(i, v)
	}
	if nb > 8 {
		n.palette = nil
	}
	*p = *n
}

// Fill sets every entry to s and drops the data array.
func (p *PalettedContainer) Fill(s StateID) {
	p.bits, p.palette, p.data = 0, []StateID{s}, nil
}

// Unpack writes all 4096 states into dst, much faster than 4096 Get calls.
func (p *PalettedContainer) Unpack(dst *[SectionVolume]StateID) {
	if p.bits == 0 {
		v := p.palette[0]
		for i := range dst {
			dst[i] = v
		}
		return
	}
	b := uint(p.bits)
	per := 64 / b
	mask := uint64(1<<b - 1)
	i := 0
	for _, w := range p.data {
		for k := uint(0); k < per && i < SectionVolume; k++ {
			v := (w >> (k * b)) & mask
			if p.bits > 8 {
				dst[i] = StateID(v)
			} else {
				dst[i] = p.palette[v]
			}
			i++
		}
	}
}

// Pack rebuilds the container from 4096 states choosing the smallest layout.
func (p *PalettedContainer) Pack(src *[SectionVolume]StateID) {
	index := map[StateID]uint32{}
	pal := []StateID{}
	for _, s := range src {
		if _, ok := index[s]; !ok {
			index[s] = uint32(len(pal))
			pal = append(pal, s)
		}
	}
	if len(pal) == 1 {
		p.Fill(pal[0])
		return
	}
	nb := uint8(bits.Len(uint(len(pal) - 1)))
	if nb < 4 {
		nb = 4
	}
	if nb > 8 {
		nb = directBits
		pal = nil
	}
	p.bits, p.palette, p.data = nb, pal, make([]uint64, longsFor(nb))
	for i, s := range src {
		if nb > 8 {
			p.setRaw(i, uint32(s))
		} else {
			p.setRaw(i, index[s])
		}
	}
}

// Palette returns the local palette (nil in direct mode) for serialization.
func (p *PalettedContainer) Palette() []StateID { return p.palette }

// Bits returns bits per entry (0 for single value).
func (p *PalettedContainer) Bits() int { return int(p.bits) }

// Data returns the packed longs (nil for single value).
func (p *PalettedContainer) Data() []uint64 { return p.data }

// LoadPacked adopts Anvil-style data: a palette and longs packed with the
// given bits per entry (Anvil derives it from the palette length, minimum 4).
// spanning=true selects the pre-1.16 layout where entries cross longs.
func (p *PalettedContainer) LoadPacked(pal []StateID, data []uint64, b uint, spanning bool) {
	if len(pal) <= 1 || len(data) == 0 || b == 0 {
		s := Air
		if len(pal) > 0 {
			s = pal[0]
		}
		p.Fill(s)
		return
	}
	var tmp [SectionVolume]StateID
	mask := uint64(1)<<b - 1
	for i := 0; i < SectionVolume; i++ {
		var v uint64
		if spanning {
			bit := uint(i) * b
			w, off := bit/64, bit%64
			v = data[w] >> off
			if off+b > 64 && int(w+1) < len(data) {
				v |= data[w+1] << (64 - off)
			}
		} else {
			per := 64 / b
			if int(uint(i)/per) < len(data) {
				v = data[uint(i)/per] >> ((uint(i) % per) * b)
			}
		}
		v &= mask
		if int(v) < len(pal) {
			tmp[i] = pal[v]
		}
	}
	p.Pack(&tmp)
}

package netcode

import (
	"encoding/binary"
	"errors"
	"maps"
	"slices"
)

// Snapshot is the replicated state of one server frame: each entity's state,
// encoded by the game, by entity id. Entities missing from it do not exist.
type Snapshot map[uint32][]byte

// Delta compression follows Q3's snapshots: the server keeps what it sent to
// each client in its last Backup packets and encodes a new snapshot against
// the newest one the client acknowledged; when that one is gone it sends the
// whole snapshot. Entity states that differ are sent as a mask of the
// changed bytes plus those bytes, so a game should encode states with fixed
// field positions (quantized integers change few bytes).
const Backup = 32

// Encoding of a delta:
//
//	base   uvarint  sequence of the base snapshot, 0 for none
//	count  uvarint  entries, each:
//	  id   uvarint  entity id minus previous entry's id
//	  op   byte     opRemove, opFull (uvarint length, bytes) or opMask
//	                (uvarint length, mask of length bits, changed bytes)
const (
	opRemove = 0
	opFull   = 1
	opMask   = 2
)

type ring struct {
	seq  [Backup]uint32
	snap [Backup]Snapshot
}

func (r *ring) put(seq uint32, s Snapshot) {
	r.seq[seq%Backup], r.snap[seq%Backup] = seq, s
}

func (r *ring) get(seq uint32) (Snapshot, bool) {
	if seq == 0 || r.seq[seq%Backup] != seq {
		return nil, false
	}
	return r.snap[seq%Backup], true
}

// Encoder compresses the snapshots sent to one client.
type Encoder struct{ sent ring }

// Encode appends snap to dst, delta compressed against the snapshot sent
// under acked if it is still known, and remembers it under seq (the sequence
// the packet will go out under: Server.NextSeq). snap must not be changed
// afterwards.
func (e *Encoder) Encode(dst []byte, seq, acked uint32, snap Snapshot) []byte {
	base, ok := e.sent.get(acked)
	if !ok {
		acked = 0
	}
	e.sent.put(seq, snap)
	return AppendDelta(dst, acked, base, snap)
}

// AppendDelta appends the delta from base (sent under baseSeq; 0 and nil for
// a whole snapshot) to cur.
func AppendDelta(dst []byte, baseSeq uint32, base, cur Snapshot) []byte {
	dst = binary.AppendUvarint(dst, uint64(baseSeq))
	ids := make([]uint32, 0, len(cur)+len(base))
	for id, s := range cur {
		if b, ok := base[id]; !ok || string(b) != string(s) {
			ids = append(ids, id)
		}
	}
	for id := range base {
		if _, ok := cur[id]; !ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	dst = binary.AppendUvarint(dst, uint64(len(ids)))
	prev := uint32(0)
	for _, id := range ids {
		dst = binary.AppendUvarint(dst, uint64(id-prev))
		prev = id
		s, ok := cur[id]
		if !ok {
			dst = append(dst, opRemove)
			continue
		}
		b, had := base[id]
		if !had || len(b) != len(s) {
			dst = append(dst, opFull)
			dst = binary.AppendUvarint(dst, uint64(len(s)))
			dst = append(dst, s...)
			continue
		}
		dst = append(dst, opMask)
		dst = binary.AppendUvarint(dst, uint64(len(s)))
		mask := len(dst)
		dst = append(dst, make([]byte, (len(s)+7)/8)...)
		for i := range s {
			if s[i] != b[i] {
				dst[mask+i/8] |= 1 << (i % 8)
				dst = append(dst, s[i])
			}
		}
	}
	return dst
}

// ErrNoBase reports a delta against a snapshot the decoder no longer has; the
// packet is dropped (Q3: "delta from invalid frame") and the next one, which
// is encoded against a newer acknowledgement, will do.
var ErrNoBase = errors.New("netcode: delta base is gone")

var errBadDelta = errors.New("netcode: malformed snapshot delta")

// Decoder rebuilds the snapshots one client receives.
type Decoder struct{ got ring }

// Decode reads a delta from data, as received under seq, and returns the
// snapshot and the bytes after it.
func (d *Decoder) Decode(seq uint32, data []byte) (Snapshot, []byte, error) {
	baseSeq, n := binary.Uvarint(data)
	if n <= 0 {
		return nil, nil, errBadDelta
	}
	var base Snapshot
	if baseSeq != 0 {
		var ok bool
		if base, ok = d.got.get(uint32(baseSeq)); !ok {
			return nil, nil, ErrNoBase
		}
	}
	snap, rest, err := ReadDelta(base, data[n:])
	if err != nil {
		return nil, nil, err
	}
	d.got.put(seq, snap)
	return snap, rest, nil
}

// ReadDelta applies the entries of a delta (after its base sequence) to base
// and returns the new snapshot; base is not changed.
func ReadDelta(base Snapshot, data []byte) (Snapshot, []byte, error) {
	count, n := binary.Uvarint(data)
	if n <= 0 || count > uint64(len(data)) {
		return nil, nil, errBadDelta
	}
	data = data[n:]
	out := make(Snapshot, len(base)+int(count))
	maps.Copy(out, base)
	id := uint32(0)
	for range count {
		step, n := binary.Uvarint(data)
		if n <= 0 || len(data) < n+1 {
			return nil, nil, errBadDelta
		}
		id += uint32(step)
		op := data[n]
		data = data[n+1:]
		if op == opRemove {
			delete(out, id)
			continue
		}
		size, n := binary.Uvarint(data)
		if n <= 0 || size > MaxMessage {
			return nil, nil, errBadDelta
		}
		data = data[n:]
		switch op {
		case opFull:
			if uint64(len(data)) < size {
				return nil, nil, errBadDelta
			}
			out[id] = append([]byte(nil), data[:size]...)
			data = data[size:]
		case opMask:
			b := base[id]
			m := int(size+7) / 8
			if len(b) != int(size) || len(data) < m {
				return nil, nil, errBadDelta
			}
			mask := data[:m]
			data = data[m:]
			s := append([]byte(nil), b...)
			for i := range s {
				if mask[i/8]&(1<<(i%8)) != 0 {
					if len(data) == 0 {
						return nil, nil, errBadDelta
					}
					s[i], data = data[0], data[1:]
				}
			}
			out[id] = s
		default:
			return nil, nil, errBadDelta
		}
	}
	return out, data, nil
}

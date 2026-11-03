package netcode

import (
	"encoding/binary"
	"errors"
)

// Wire format of a sequenced packet (both directions):
//
//	seq     uint32  high bit set: this datagram is one fragment of the message
//	conn    uint16  connection id picked by the client (Q3's qport)
//	[start  uint32  fragment: byte offset of this fragment in the message
//	 length uint16] fragment: bytes in this fragment; < fragmentSize ends it
//	message
//
// and of a message:
//
//	ack      uint32  highest sequence received from the peer
//	relAck   uint32  last reliable message delivered from the peer
//	relFirst uint32  sequence of the first reliable message below
//	count    uvarint reliable messages, each as uvarint length + bytes
//	payload          the rest: the unreliable payload (a snapshot, user commands)
//
// Every message resends all reliable messages the peer has not acknowledged
// yet (Q3's reliable commands), so a lost packet costs no extra round trip on
// a LAN, where packets go out every tick anyway.
const (
	fragmentBit  = 1 << 31
	fragmentSize = 1200 // keeps datagrams below a 1500 byte MTU
	headerSize   = 6
	// MaxMessage is the largest message (payload plus pending reliable
	// messages) a channel carries; larger ones are fragmented.
	MaxMessage = 1 << 16
	// maxPendingReliable unacknowledged reliable messages fail the channel.
	maxPendingReliable = 256
)

var (
	errTooLarge   = errors.New("netcode: message too large")
	errOverflow   = errors.New("netcode: too many unacknowledged reliable messages")
	errBadMessage = errors.New("netcode: malformed message")
)

// channel is one end of a Q3 netchan: a sequenced, unreliable stream of
// messages that drops duplicate and out-of-order packets, carries reliable
// messages piggybacked until the peer acknowledges them, and fragments
// messages larger than one datagram.
type channel struct {
	conn uint16

	outSeq  uint32 // last sequence sent
	inSeq   uint32 // last sequence received whole
	peerAck uint32 // last of our sequences the peer has received
	dropped uint32 // incoming sequences never received

	relOut      [][]byte // unacknowledged reliable messages
	relOutFirst uint32   // sequence of relOut[0]
	relIn       uint32   // last reliable sequence delivered

	fragSeq uint32 // sequence being reassembled
	fragBuf []byte
}

func newChannel(conn uint16) *channel {
	return &channel{conn: conn, relOutFirst: 1}
}

// queueReliable adds a reliable message to every following packet until the
// peer acknowledges it.
func (c *channel) queueReliable(msg []byte) error {
	if len(c.relOut) >= maxPendingReliable {
		return errOverflow
	}
	c.relOut = append(c.relOut, append([]byte(nil), msg...))
	return nil
}

// transmit builds the next message around payload and returns it as one or
// more datagrams, along with the sequence it was sent under.
func (c *channel) transmit(payload []byte) ([][]byte, uint32, error) {
	msg := make([]byte, 0, 16+len(payload))
	msg = binary.LittleEndian.AppendUint32(msg, c.inSeq)
	msg = binary.LittleEndian.AppendUint32(msg, c.relIn)
	msg = binary.LittleEndian.AppendUint32(msg, c.relOutFirst)
	msg = binary.AppendUvarint(msg, uint64(len(c.relOut)))
	for _, r := range c.relOut {
		msg = binary.AppendUvarint(msg, uint64(len(r)))
		msg = append(msg, r...)
	}
	msg = append(msg, payload...)
	if len(msg) > MaxMessage {
		return nil, 0, errTooLarge
	}
	c.outSeq++
	seq := c.outSeq
	if len(msg) < fragmentSize {
		p := make([]byte, 0, headerSize+len(msg))
		p = binary.LittleEndian.AppendUint32(p, seq)
		p = binary.LittleEndian.AppendUint16(p, c.conn)
		return [][]byte{append(p, msg...)}, seq, nil
	}
	// Like Q3, the last fragment is the first one shorter than fragmentSize,
	// so a message of an exact multiple ends with an empty fragment.
	var out [][]byte
	for start := 0; ; start += fragmentSize {
		n := min(fragmentSize, len(msg)-start)
		p := make([]byte, 0, headerSize+6+n)
		p = binary.LittleEndian.AppendUint32(p, seq|fragmentBit)
		p = binary.LittleEndian.AppendUint16(p, c.conn)
		p = binary.LittleEndian.AppendUint32(p, uint32(start))
		p = binary.LittleEndian.AppendUint16(p, uint16(n))
		out = append(out, append(p, msg[start:start+n]...))
		if n < fragmentSize {
			return out, seq, nil
		}
	}
}

// received is what one whole incoming message delivered.
type received struct {
	seq      uint32
	reliable [][]byte
	payload  []byte
}

// process takes one datagram (header included, connection id already
// checked). It returns ok=false for datagrams that complete no message:
// fragments, duplicates and stale packets.
func (c *channel) process(p []byte) (r received, ok bool, err error) {
	if len(p) < headerSize {
		return r, false, errBadMessage
	}
	seq := binary.LittleEndian.Uint32(p)
	p = p[headerSize:]
	frag := seq&fragmentBit != 0
	seq &^= fragmentBit
	if seq <= c.inSeq {
		return r, false, nil // duplicate or out of order: Q3 drops these
	}
	msg := p
	if frag {
		if len(p) < 6 {
			return r, false, errBadMessage
		}
		start := binary.LittleEndian.Uint32(p)
		n := int(binary.LittleEndian.Uint16(p[4:]))
		p = p[6:]
		if n != len(p) || n > fragmentSize {
			return r, false, errBadMessage
		}
		if seq != c.fragSeq {
			c.fragSeq, c.fragBuf = seq, c.fragBuf[:0]
		}
		if int(start) != len(c.fragBuf) {
			return r, false, nil // a fragment went missing: drop the message
		}
		if len(c.fragBuf)+n > MaxMessage {
			c.fragBuf = c.fragBuf[:0]
			return r, false, errTooLarge
		}
		c.fragBuf = append(c.fragBuf, p...)
		if n == fragmentSize {
			return r, false, nil
		}
		msg = c.fragBuf
	}
	if len(msg) < 12 {
		return r, false, errBadMessage
	}
	ack := binary.LittleEndian.Uint32(msg)
	relAck := binary.LittleEndian.Uint32(msg[4:])
	relFirst := binary.LittleEndian.Uint32(msg[8:])
	msg = msg[12:]
	count, k := binary.Uvarint(msg)
	if k <= 0 {
		return r, false, errBadMessage
	}
	msg = msg[k:]
	rel := make([][]byte, 0, count)
	for range count {
		n, k := binary.Uvarint(msg)
		if k <= 0 || uint64(len(msg)-k) < n {
			return r, false, errBadMessage
		}
		rel = append(rel, msg[k:k+int(n)])
		msg = msg[k+int(n):]
	}
	if count > 0 && relFirst > c.relIn+1 {
		return r, false, errBadMessage // the peer skipped reliable messages
	}

	c.dropped += seq - c.inSeq - 1
	c.inSeq = seq
	if ack > c.peerAck && ack <= c.outSeq {
		c.peerAck = ack
	}
	if acked := int(int64(relAck) - int64(c.relOutFirst) + 1); acked > 0 {
		acked = min(acked, len(c.relOut))
		c.relOut = append(c.relOut[:0], c.relOut[acked:]...)
		c.relOutFirst += uint32(acked)
	}
	r.seq = seq
	for i, m := range rel {
		if relFirst+uint32(i) == c.relIn+1 {
			c.relIn++
			r.reliable = append(r.reliable, append([]byte(nil), m...))
		}
	}
	r.payload = append([]byte(nil), msg...)
	return r, true, nil
}

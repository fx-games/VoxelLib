package netcode

import (
	"encoding/binary"
	"math/rand/v2"
	"net"
	"sync"
	"time"
)

// Config tunes a Server or Client. The zero value works except for Protocol,
// which both ends must agree on.
type Config struct {
	// Protocol identifies the game and its version; peers with a different
	// one are refused and do not answer discovery.
	Protocol uint32
	// Timeout drops a peer that sent nothing for this long (default 20s,
	// so a peer whose game hitches, loading a world on a slow machine,
	// stays connected; a peer that leaves says so at once).
	Timeout time.Duration
	// MaxClients is the number of client slots on a server (default 4).
	MaxClients int

	// Network simulation for tests: Drop is the chance an outgoing datagram
	// is lost, Latency delays every one and Jitter adds up to that much more
	// at random (which reorders datagrams).
	Drop    float64
	Latency time.Duration
	Jitter  time.Duration
}

// keepalive is how long a link may stay quiet: a quarter of the timeout, at
// most a second.
func (c Config) keepalive() time.Duration { return min(c.timeout()/4, time.Second) }

func (c Config) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 20 * time.Second
}

// EventKind says what an Event reports.
type EventKind int

const (
	// Connected: a client took a slot (server) or the server accepted us
	// (client; Data is the server's reply to our hello).
	Connected EventKind = iota
	// Disconnected: the peer left or timed out; Reason says which.
	Disconnected
	// Reliable: a reliable message, delivered once and in order.
	Reliable
	// Message: the unreliable payload of a packet. Seq increases strictly:
	// late and duplicate packets never show up.
	Message
)

// Event is something that arrived from the network since the last Poll.
type Event struct {
	Kind   EventKind
	Client int    // server: the client's slot
	Seq    uint32 // Message: the packet's sequence
	Data   []byte
	Reason string
}

// Stats describes one connection.
type Stats struct {
	RTT     time.Duration // smoothed round trip time
	Dropped uint32        // incoming packets that never arrived
	Sent    uint32        // last sequence sent
	Acked   uint32        // last sequence the peer acknowledged
}

// Connectionless packets start with 0xFFFFFFFF where a sequence would be,
// followed by one of these and the protocol id.
const (
	connectionless = 0xFFFFFFFF

	pktGetInfo    = 1 // nonce u32
	pktInfo       = 2 // nonce u32, info bytes
	pktConnect    = 3 // conn u16, hello bytes
	pktAccept     = 4 // conn u16, slot u8, reply bytes
	pktRefuse     = 5 // conn u16, reason
	pktDisconnect = 6 // conn u16, reason
)

func connectionlessPacket(kind byte, protocol uint32, body []byte) []byte {
	p := make([]byte, 0, 9+len(body))
	p = binary.LittleEndian.AppendUint32(p, connectionless)
	p = append(p, kind)
	p = binary.LittleEndian.AppendUint32(p, protocol)
	return append(p, body...)
}

// parseConnectionless splits a connectionless packet of the given protocol.
func parseConnectionless(p []byte, protocol uint32) (kind byte, body []byte, ok bool) {
	if len(p) < 9 || binary.LittleEndian.Uint32(p) != connectionless ||
		binary.LittleEndian.Uint32(p[5:]) != protocol {
		return 0, nil, false
	}
	return p[4], p[9:], true
}

// socket is a UDP socket that can simulate loss and latency on send.
type socket struct {
	conn *net.UDPConn
	cfg  Config
	mu   sync.Mutex // rng
	rng  *rand.Rand
}

func newSocket(conn *net.UDPConn, cfg Config) *socket {
	return &socket{conn: conn, cfg: cfg, rng: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))}
}

func (s *socket) send(p []byte, to *net.UDPAddr) {
	if s.cfg.Drop == 0 && s.cfg.Latency == 0 && s.cfg.Jitter == 0 {
		s.conn.WriteToUDP(p, to)
		return
	}
	s.mu.Lock()
	drop := s.rng.Float64() < s.cfg.Drop
	delay := s.cfg.Latency
	if s.cfg.Jitter > 0 {
		delay += time.Duration(s.rng.Int64N(int64(s.cfg.Jitter)))
	}
	s.mu.Unlock()
	if drop {
		return
	}
	p = append([]byte(nil), p...)
	time.AfterFunc(delay, func() { s.conn.WriteToUDP(p, to) })
}

// link is a netchan to one peer plus the timing around it.
type link struct {
	addr     *net.UDPAddr
	ch       *channel
	lastRecv time.Time
	lastSend time.Time
	sentAt   [64]time.Time // by sequence, for the round trip time
	rtt      time.Duration
}

func newLink(addr *net.UDPAddr, conn uint16, now time.Time) *link {
	return &link{addr: addr, ch: newChannel(conn), lastRecv: now, lastSend: now}
}

// send transmits payload (and pending reliable messages) to the peer.
func (l *link) send(s *socket, payload []byte, now time.Time) (uint32, error) {
	dgrams, seq, err := l.ch.transmit(payload)
	if err != nil {
		return 0, err
	}
	for _, d := range dgrams {
		s.send(d, l.addr)
	}
	l.lastSend = now
	l.sentAt[seq%uint32(len(l.sentAt))] = now
	return seq, nil
}

// receive processes one datagram and appends what it delivered to evs.
func (l *link) receive(p []byte, now time.Time, client int, evs []Event) ([]Event, error) {
	before := l.ch.peerAck
	r, ok, err := l.ch.process(p)
	if err != nil || !ok {
		return evs, err
	}
	l.lastRecv = now
	if ack := l.ch.peerAck; ack != before && l.ch.outSeq-ack < uint32(len(l.sentAt)) {
		sample := now.Sub(l.sentAt[ack%uint32(len(l.sentAt))])
		if l.rtt == 0 {
			l.rtt = sample
		} else {
			l.rtt += (sample - l.rtt) / 8
		}
	}
	for _, m := range r.reliable {
		evs = append(evs, Event{Kind: Reliable, Client: client, Data: m})
	}
	return append(evs, Event{Kind: Message, Client: client, Seq: r.seq, Data: r.payload}), nil
}

// keepalive sends an empty packet when reliable messages wait for an
// acknowledgement and nothing went out lately, or when the link is idle, so
// acknowledgements flow and the peer does not time out.
func (l *link) keepalive(s *socket, now time.Time, every time.Duration) {
	idle := now.Sub(l.lastSend)
	if (len(l.ch.relOut) > 0 && idle >= 50*time.Millisecond) || idle >= every {
		l.send(s, nil, now)
	}
}

func (l *link) stats() Stats {
	return Stats{RTT: l.rtt, Dropped: l.ch.dropped, Sent: l.ch.outSeq, Acked: l.ch.peerAck}
}

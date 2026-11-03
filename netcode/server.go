package netcode

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"time"
)

// AcceptFunc decides whether a client may join, given its hello, and returns
// the reply the client receives with its Connected event (or an error, whose
// text the client receives as the refusal reason). It runs on the server's
// network goroutine and must not call back into the Server.
type AcceptFunc func(slot int, addr net.Addr, hello []byte) (reply []byte, err error)

// Server accepts up to Config.MaxClients clients on one UDP socket and
// answers LAN discovery on it.
type Server struct {
	cfg    Config
	sock   *socket
	accept AcceptFunc

	mu      sync.Mutex
	info    []byte
	clients []*serverClient // by slot; nil when free
	events  []Event
	closed  bool
	done    chan struct{}
}

type serverClient struct {
	*link
	reply []byte // resent when a connect is repeated
}

// Listen opens a server on addr (":28960", or ":0" for any port).
func Listen(addr string, cfg Config, accept AcceptFunc) (*Server, error) {
	ua, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", ua)
	if err != nil {
		return nil, err
	}
	if cfg.MaxClients <= 0 {
		cfg.MaxClients = 4
	}
	s := &Server{cfg: cfg, sock: newSocket(conn, cfg), accept: accept,
		clients: make([]*serverClient, cfg.MaxClients), done: make(chan struct{})}
	go s.read()
	return s, nil
}

// Addr is the address the server listens on.
func (s *Server) Addr() *net.UDPAddr { return s.sock.conn.LocalAddr().(*net.UDPAddr) }

// SetInfo sets what discovery replies carry (a server name, player count).
func (s *Server) SetInfo(info []byte) {
	s.mu.Lock()
	s.info = append([]byte(nil), info...)
	s.mu.Unlock()
}

func (s *Server) read() {
	defer close(s.done)
	buf := make([]byte, 2048)
	for {
		n, from, err := s.sock.conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue // e.g. ICMP port unreachable on Windows
		}
		s.handle(buf[:n], from, time.Now())
	}
}

func (s *Server) handle(p []byte, from *net.UDPAddr, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if kind, body, ok := parseConnectionless(p, s.cfg.Protocol); ok {
		s.handleConnectionless(kind, body, from, now)
		return
	}
	if len(p) < headerSize {
		return
	}
	conn := binary.LittleEndian.Uint16(p[4:])
	for slot, c := range s.clients {
		if c != nil && c.ch.conn == conn && c.addr.IP.Equal(from.IP) {
			c.addr = from // the port may change behind NAT (Q3's qport)
			evs, err := c.receive(p, now, slot, s.events)
			s.events = evs
			if err != nil {
				s.drop(slot, err.Error())
			}
			return
		}
	}
}

func (s *Server) handleConnectionless(kind byte, body []byte, from *net.UDPAddr, now time.Time) {
	reply := func(kind byte, body []byte) {
		s.sock.send(connectionlessPacket(kind, s.cfg.Protocol, body), from)
	}
	switch kind {
	case pktGetInfo:
		if len(body) >= 4 {
			reply(pktInfo, append(body[:4:4], s.info...))
		}
	case pktConnect:
		if len(body) < 2 {
			return
		}
		conn := binary.LittleEndian.Uint16(body)
		hello := body[2:]
		free := -1
		for slot, c := range s.clients {
			switch {
			case c == nil:
				if free < 0 {
					free = slot
				}
			case c.addr.String() == from.String() && c.ch.conn == conn:
				reply(pktAccept, append(binary.LittleEndian.AppendUint16(nil, conn), append([]byte{byte(slot)}, c.reply...)...))
				return
			case c.addr.String() == from.String():
				s.drop(slot, "reconnected") // the same client restarted
				if free < 0 || slot < free {
					free = slot
				}
			}
		}
		refuse := func(reason string) {
			reply(pktRefuse, append(binary.LittleEndian.AppendUint16(nil, conn), reason...))
		}
		if free < 0 {
			refuse("server is full")
			return
		}
		var r []byte
		if s.accept != nil {
			var err error
			if r, err = s.accept(free, from, hello); err != nil {
				refuse(err.Error())
				return
			}
		}
		s.clients[free] = &serverClient{link: newLink(from, conn, now), reply: r}
		s.events = append(s.events, Event{Kind: Connected, Client: free, Data: append([]byte(nil), hello...)})
		reply(pktAccept, append(binary.LittleEndian.AppendUint16(nil, conn), append([]byte{byte(free)}, r...)...))
	case pktDisconnect:
		if len(body) < 2 {
			return
		}
		conn := binary.LittleEndian.Uint16(body)
		for slot, c := range s.clients {
			if c != nil && c.ch.conn == conn && c.addr.IP.Equal(from.IP) {
				s.drop(slot, "left: "+string(body[2:]))
			}
		}
	}
}

// drop frees a slot; the caller holds mu.
func (s *Server) drop(slot int, reason string) {
	c := s.clients[slot]
	if c == nil {
		return
	}
	s.clients[slot] = nil
	// Q3 sends the disconnect a few times, as it is not acknowledged.
	p := connectionlessPacket(pktDisconnect, s.cfg.Protocol, append(binary.LittleEndian.AppendUint16(nil, c.ch.conn), reason...))
	for range 3 {
		s.sock.send(p, c.addr)
	}
	s.events = append(s.events, Event{Kind: Disconnected, Client: slot, Reason: reason})
}

// Poll returns what arrived since the last call, sends keepalives and drops
// clients that timed out. Call it once per server frame.
func (s *Server) Poll() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for slot, c := range s.clients {
		if c == nil {
			continue
		}
		if now.Sub(c.lastRecv) > s.cfg.timeout() {
			s.drop(slot, "timed out")
			continue
		}
		c.keepalive(s.sock, now)
	}
	evs := s.events
	s.events = nil
	return evs
}

var errNoClient = errors.New("netcode: no client in that slot")

// Send transmits an unreliable payload to a client, along with its pending
// reliable messages, and returns the sequence it went out under.
func (s *Server) Send(slot int, payload []byte) (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.client(slot)
	if c == nil {
		return 0, errNoClient
	}
	return c.send(s.sock, payload, time.Now())
}

// SendReliable queues a message the client receives exactly once, in order.
// It goes out with the next Send (or keepalive) and every one after it until
// the client acknowledges it.
func (s *Server) SendReliable(slot int, msg []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.client(slot)
	if c == nil {
		return errNoClient
	}
	if err := c.ch.queueReliable(msg); err != nil {
		s.drop(slot, err.Error())
		return err
	}
	return nil
}

// NextSeq is the sequence the next Send to the client will use.
func (s *Server) NextSeq(slot int) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.client(slot); c != nil {
		return c.ch.outSeq + 1
	}
	return 0
}

// Acked is the last sequence the client acknowledged: the newest packet it is
// known to have, and so the base for delta compression.
func (s *Server) Acked(slot int) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.client(slot); c != nil {
		return c.ch.peerAck
	}
	return 0
}

// Stats describes a client's connection.
func (s *Server) Stats(slot int) Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.client(slot); c != nil {
		return c.stats()
	}
	return Stats{}
}

// Kick disconnects a client.
func (s *Server) Kick(slot int, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client(slot) != nil {
		s.drop(slot, reason)
	}
}

func (s *Server) client(slot int) *serverClient {
	if slot < 0 || slot >= len(s.clients) {
		return nil
	}
	return s.clients[slot]
}

// Close disconnects every client and closes the socket.
func (s *Server) Close() error {
	s.mu.Lock()
	for slot := range s.clients {
		s.drop(slot, "server shut down")
	}
	s.closed = true
	s.mu.Unlock()
	err := s.sock.conn.Close()
	<-s.done
	return err
}

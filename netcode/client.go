package netcode

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"sync"
	"time"
)

// Client is a connection to one Server.
type Client struct {
	cfg  Config
	sock *socket
	slot int

	mu     sync.Mutex
	link   *link
	events []Event
	gone   bool // disconnected; the Disconnected event is queued
	done   chan struct{}

	accepted chan acceptResult // Dial waits on it
}

type acceptResult struct {
	slot  int
	reply []byte
	err   error
}

// Dial connects to a server, sending hello with the request, and waits up to
// the configured timeout for it to accept. It returns the server's reply.
func Dial(addr string, cfg Config, hello []byte) (*Client, []byte, error) {
	ua, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, nil, err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	c := &Client{cfg: cfg, sock: newSocket(conn, cfg), done: make(chan struct{}),
		link: newLink(ua, uint16(rand.Uint32()), now), accepted: make(chan acceptResult, 1)}
	go c.read()

	req := connectionlessPacket(pktConnect, cfg.Protocol, append(binary.LittleEndian.AppendUint16(nil, c.link.ch.conn), hello...))
	deadline := time.After(cfg.timeout())
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		c.sock.send(req, ua)
		select {
		case r := <-c.accepted:
			if r.err != nil {
				c.closeSocket()
				return nil, nil, r.err
			}
			c.mu.Lock()
			c.slot = r.slot
			c.link.lastRecv = time.Now()
			c.mu.Unlock()
			return c, r.reply, nil
		case <-tick.C:
		case <-deadline:
			c.closeSocket()
			return nil, nil, fmt.Errorf("netcode: no answer from %s", addr)
		}
	}
}

// Slot is the client's slot on the server.
func (c *Client) Slot() int { return c.slot }

func (c *Client) read() {
	defer close(c.done)
	buf := make([]byte, 2048)
	for {
		n, from, err := c.sock.conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		c.handle(buf[:n], from, time.Now())
	}
}

func (c *Client) handle(p []byte, from *net.UDPAddr, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gone || !from.IP.Equal(c.link.addr.IP) || from.Port != c.link.addr.Port {
		return
	}
	conn := c.link.ch.conn
	if kind, body, ok := parseConnectionless(p, c.cfg.Protocol); ok {
		if len(body) < 2 || binary.LittleEndian.Uint16(body) != conn {
			return
		}
		body = body[2:]
		switch kind {
		case pktAccept:
			if len(body) >= 1 {
				select {
				case c.accepted <- acceptResult{slot: int(body[0]), reply: append([]byte(nil), body[1:]...)}:
					c.events = append(c.events, Event{Kind: Connected, Data: append([]byte(nil), body[1:]...)})
				default: // a repeated accept
				}
			}
		case pktRefuse:
			select {
			case c.accepted <- acceptResult{err: fmt.Errorf("netcode: refused: %s", body)}:
			default:
			}
		case pktDisconnect:
			c.disconnect(string(body))
		}
		return
	}
	if len(p) < headerSize || binary.LittleEndian.Uint16(p[4:]) != conn {
		return
	}
	evs, err := c.link.receive(p, now, 0, c.events)
	c.events = evs
	if err != nil {
		c.disconnect(err.Error())
	}
}

// disconnect queues the Disconnected event once; the caller holds mu.
func (c *Client) disconnect(reason string) {
	if !c.gone {
		c.gone = true
		c.events = append(c.events, Event{Kind: Disconnected, Reason: reason})
	}
}

// Poll returns what arrived since the last call and sends a keepalive when
// needed. A Disconnected event ends the connection; Close it afterwards.
func (c *Client) Poll() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if !c.gone {
		if now.Sub(c.link.lastRecv) > c.cfg.timeout() {
			c.disconnect("timed out")
		} else {
			c.link.keepalive(c.sock, now)
		}
	}
	evs := c.events
	c.events = nil
	return evs
}

var errGone = errors.New("netcode: disconnected")

// Send transmits an unreliable payload along with pending reliable messages
// and returns the sequence it went out under.
func (c *Client) Send(payload []byte) (uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gone {
		return 0, errGone
	}
	return c.link.send(c.sock, payload, time.Now())
}

// SendReliable queues a message the server receives exactly once, in order.
func (c *Client) SendReliable(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gone {
		return errGone
	}
	if err := c.link.ch.queueReliable(msg); err != nil {
		c.disconnect(err.Error())
		return err
	}
	return nil
}

// Acked is the last sequence the server acknowledged.
func (c *Client) Acked() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.link.ch.peerAck
}

// Stats describes the connection.
func (c *Client) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.link.stats()
}

// Close tells the server we leave and closes the socket.
func (c *Client) Close() error {
	c.mu.Lock()
	if !c.gone {
		p := connectionlessPacket(pktDisconnect, c.cfg.Protocol, binary.LittleEndian.AppendUint16(nil, c.link.ch.conn))
		for range 3 {
			c.sock.send(p, c.link.addr)
		}
		c.gone = true
	}
	c.mu.Unlock()
	return c.closeSocket()
}

func (c *Client) closeSocket() error {
	err := c.sock.conn.Close()
	<-c.done
	return err
}

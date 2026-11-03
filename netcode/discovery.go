package netcode

import (
	"encoding/binary"
	"math/rand/v2"
	"net"
	"time"
)

// Found is a server that answered discovery.
type Found struct {
	Addr *net.UDPAddr
	Info []byte // what the server set with SetInfo
	Ping time.Duration
}

// Discover broadcasts a query to port on every IPv4 network this machine is
// on (and to localhost) and collects the servers of cfg.Protocol that answer
// within wait, like Q3's LAN server list.
func Discover(port int, cfg Config, wait time.Duration) ([]Found, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	nonce := rand.Uint32()
	q := connectionlessPacket(pktGetInfo, cfg.Protocol, binary.LittleEndian.AppendUint32(nil, nonce))
	start := time.Now()
	for _, ip := range broadcastAddrs() {
		conn.WriteToUDP(q, &net.UDPAddr{IP: ip, Port: port})
	}

	var out []Found
	seen := map[string]bool{}
	buf := make([]byte, 2048)
	conn.SetReadDeadline(start.Add(wait))
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return out, nil
			}
			continue
		}
		kind, body, ok := parseConnectionless(buf[:n], cfg.Protocol)
		if !ok || kind != pktInfo || len(body) < 4 || binary.LittleEndian.Uint32(body) != nonce {
			continue
		}
		// A server on this machine answers both the loopback and the LAN
		// query; keep the first answer per server port and IP.
		key := from.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Found{Addr: from, Info: append([]byte(nil), body[4:]...), Ping: time.Since(start)})
	}
}

// broadcastAddrs lists the limited broadcast address, each interface's
// directed broadcast address and localhost.
func broadcastAddrs() []net.IP {
	out := []net.IP{net.IPv4bcast, net.IPv4(127, 0, 0, 1)}
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil || len(n.Mask) == 0 {
				continue
			}
			ip, mask := n.IP.To4(), net.IP(n.Mask).To4()
			if mask == nil {
				continue
			}
			b := make(net.IP, 4)
			for i := range b {
				b[i] = ip[i] | ^mask[i]
			}
			out = append(out, b)
		}
	}
	return out
}

package netcode

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"net"
	"testing"
	"time"
)

const testProtocol = 0x7e57

func pair(t *testing.T, cfg Config) (*Server, *Client) {
	t.Helper()
	srv, err := Listen("127.0.0.1:0", cfg, func(slot int, _ net.Addr, hello []byte) ([]byte, error) {
		if string(hello) == "bad" {
			return nil, fmt.Errorf("go away")
		}
		return fmt.Appendf(nil, "welcome %d", slot), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	cl, reply, err := Dial(srv.Addr().String(), cfg, []byte("hi"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	if string(reply) != "welcome 0" || cl.Slot() != 0 {
		t.Fatalf("reply %q slot %d", reply, cl.Slot())
	}
	return srv, cl
}

func TestChannelFragmentsAndDropsStale(t *testing.T) {
	a, b := newChannel(7), newChannel(7)
	big := bytes.Repeat([]byte("0123456789"), 500)
	dg, seq, err := a.transmit(big)
	if err != nil || seq != 1 || len(dg) < 4 {
		t.Fatalf("transmit: %v seq %d %d datagrams", err, seq, len(dg))
	}
	for i, d := range dg {
		r, ok, err := b.process(d)
		if err != nil || ok != (i == len(dg)-1) {
			t.Fatalf("fragment %d: ok %v err %v", i, ok, err)
		}
		if ok && !bytes.Equal(r.payload, big) {
			t.Fatal("reassembled payload differs")
		}
	}
	if _, ok, _ := b.process(dg[len(dg)-1]); ok {
		t.Fatal("duplicate delivered")
	}
	// A message of an exact multiple of the fragment size ends with an empty fragment.
	exact := make([]byte, 2*fragmentSize-16)
	dg, _, _ = a.transmit(exact[:len(exact)-1])
	for _, d := range dg {
		b.process(d)
	}
	if b.inSeq != 2 {
		t.Fatalf("inSeq %d", b.inSeq)
	}
	// Out of order: 4 arrives before 3, so 3 is dropped.
	d3, _, _ := a.transmit([]byte("three"))
	d4, _, _ := a.transmit([]byte("four"))
	if r, ok, _ := b.process(d4[0]); !ok || string(r.payload) != "four" {
		t.Fatal("4 not delivered")
	}
	if _, ok, _ := b.process(d3[0]); ok {
		t.Fatal("late 3 delivered")
	}
	if b.dropped != 1 {
		t.Fatalf("dropped %d", b.dropped)
	}
}

func TestReliableOverLossyLink(t *testing.T) {
	cfg := Config{Protocol: testProtocol, Drop: 0.3, Latency: 2 * time.Millisecond, Jitter: 3 * time.Millisecond}
	srv, cl := pair(t, cfg)
	const n = 200
	for i := range n {
		if err := cl.SendReliable(fmt.Append(nil, "c", i)); err != nil {
			t.Fatal(err)
		}
		if err := srv.SendReliable(0, fmt.Append(nil, "s", i)); err != nil {
			t.Fatal(err)
		}
	}
	var gotS, gotC []string
	var lastSeqS, lastSeqC uint32
	deadline := time.Now().Add(10 * time.Second)
	for frames := 0; (len(gotS) < n || len(gotC) < n || frames < 200) && time.Now().Before(deadline); frames++ {
		cl.Send([]byte("cmd"))
		srv.Send(0, []byte("snap"))
		for _, e := range srv.Poll() {
			switch e.Kind {
			case Reliable:
				gotS = append(gotS, string(e.Data))
			case Message:
				if e.Seq <= lastSeqS {
					t.Fatalf("server got seq %d after %d", e.Seq, lastSeqS)
				}
				lastSeqS = e.Seq
			case Disconnected:
				t.Fatal("client dropped:", e.Reason)
			}
		}
		for _, e := range cl.Poll() {
			switch e.Kind {
			case Reliable:
				gotC = append(gotC, string(e.Data))
			case Message:
				if e.Seq <= lastSeqC {
					t.Fatalf("client got seq %d after %d", e.Seq, lastSeqC)
				}
				lastSeqC = e.Seq
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	for i := range n {
		if i >= len(gotS) || gotS[i] != fmt.Sprint("c", i) || i >= len(gotC) || gotC[i] != fmt.Sprint("s", i) {
			t.Fatalf("reliable stream broken at %d: server got %d, client got %d", i, len(gotS), len(gotC))
		}
	}
	if len(gotS) != n || len(gotC) != n {
		t.Fatalf("duplicates: %d %d", len(gotS), len(gotC))
	}
	if st := srv.Stats(0); st.Dropped == 0 || st.RTT <= 0 {
		t.Fatalf("stats %+v", st)
	}
}

func TestLargeUnreliableMessage(t *testing.T) {
	srv, cl := pair(t, Config{Protocol: testProtocol})
	big := make([]byte, 40000)
	for i := range big {
		big[i] = byte(i * 7)
	}
	srv.Send(0, big)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range cl.Poll() {
			if e.Kind == Message && bytes.Equal(e.Data, big) {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("40 kB message did not arrive")
}

func TestRefuseFullAndLeave(t *testing.T) {
	cfg := Config{Protocol: testProtocol, MaxClients: 1, Timeout: time.Second}
	srv, cl := pair(t, cfg)
	if _, _, err := Dial(srv.Addr().String(), cfg, []byte("hi")); err == nil {
		t.Fatal("second client got into a full server")
	}
	cl.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, e := range srv.Poll() {
			if e.Kind == Disconnected {
				if _, _, err := Dial(srv.Addr().String(), cfg, []byte("bad")); err == nil {
					t.Fatal("hello refused by AcceptFunc got in")
				}
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no disconnect event")
}

func TestTimeout(t *testing.T) {
	cfg := Config{Protocol: testProtocol, Timeout: 300 * time.Millisecond}
	srv, cl := pair(t, cfg)
	srv.Close() // the server vanishes; the client must notice
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range cl.Poll() {
			if e.Kind == Disconnected {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("client never timed out")
}

func TestDiscover(t *testing.T) {
	srv, err := Listen(":0", Config{Protocol: testProtocol}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	srv.SetInfo([]byte("arena"))
	found, err := Discover(srv.Addr().Port, Config{Protocol: testProtocol}, 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 || string(found[0].Info) != "arena" {
		t.Fatalf("found %+v", found)
	}
	other, _ := Discover(srv.Addr().Port, Config{Protocol: testProtocol + 1}, 100*time.Millisecond)
	if len(other) != 0 {
		t.Fatal("a server of another protocol answered")
	}
}

func TestDeltaRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	var enc Encoder
	var dec Decoder
	cur := Snapshot{}
	acked := uint32(0)
	full := 0
	for seq := uint32(1); seq <= 500; seq++ {
		next := Snapshot{}
		for id, s := range cur {
			if rng.IntN(20) == 0 {
				continue // removed
			}
			s = append([]byte(nil), s...)
			if rng.IntN(3) == 0 {
				s[rng.IntN(len(s))] = byte(rng.Uint32())
			}
			if rng.IntN(50) == 0 {
				s = append(s, 1) // resized
			}
			next[id] = s
		}
		for range rng.IntN(3) {
			s := make([]byte, 1+rng.IntN(30))
			for i := range s {
				s[i] = byte(rng.Uint32())
			}
			next[uint32(rng.IntN(1000))] = s
		}
		cur = next
		data := enc.Encode(nil, seq, acked, cur)
		data = append(data, 0xAB)
		if rng.IntN(4) == 0 {
			continue // lost: not decoded, not acknowledged
		}
		got, rest, err := dec.Decode(seq, data)
		if err != nil {
			t.Fatalf("seq %d: %v", seq, err)
		}
		if len(rest) != 1 || rest[0] != 0xAB {
			t.Fatalf("seq %d: rest %v", seq, rest)
		}
		if len(got) != len(cur) {
			t.Fatalf("seq %d: %d entities, want %d", seq, len(got), len(cur))
		}
		for id, s := range cur {
			if !bytes.Equal(got[id], s) {
				t.Fatalf("seq %d entity %d differs", seq, id)
			}
		}
		if rng.IntN(3) != 0 {
			acked = seq // the ack arrives (late acks keep the older base)
		}
		if seq-acked >= Backup {
			full++
		}
	}
	_ = full
	// A small change against a base costs a few bytes.
	base := Snapshot{1: make([]byte, 40)}
	changed := Snapshot{1: append(make([]byte, 39), 9)}
	if n := len(AppendDelta(nil, 5, base, changed)); n > 12 {
		t.Fatalf("one changed byte took %d bytes", n)
	}
}

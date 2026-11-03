// Package netcode is a small LAN game network layer after Quake 3's: a
// client/server netchan over UDP plus the helpers a fast-paced game builds on.
// It knows nothing about the game; payloads are bytes.
//
//   - Server (Listen) and Client (Dial) exchange sequenced packets. Late and
//     duplicate packets are dropped, never delivered out of order; messages
//     larger than a datagram are fragmented. Each packet carries the
//     sender's unacknowledged reliable messages (SendReliable), so those
//     arrive exactly once, in order, without extra round trips.
//   - Poll returns Events (Connected, Disconnected, Reliable, Message), sends
//     keepalives and times peers out; call it once per game frame.
//   - Discover finds servers on the LAN by broadcast.
//   - Encoder and Decoder delta compress Snapshots (entity id → state)
//     against the last one the peer acknowledged.
//
// A game in Q3's style sends a snapshot to every client every server frame
// (unreliable: a lost one is superseded by the next) and has clients send
// their newest few user commands in every packet, so one lost packet loses
// no input. Events that must arrive (chat, block edits) go reliable.
package netcode

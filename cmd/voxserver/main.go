// Command voxserver is the dedicated LAN multiplayer server: the explorer's
// game without a window (no GPU, no player of its own), with a Quake-style
// console on stdin.
//
//	go run ./cmd/voxserver -name "Friday game" -seed 42
//	go run ./cmd/voxserver -map minecity -mode tdm
//
// Console commands: status, say <text>, kick <slot|name>, quit.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/memmaker/voxellib/importer/tacz"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/weapon"
)

func main() {
	addr := flag.String("addr", fmt.Sprintf(":%d", mp.DefaultPort), "UDP address to listen on")
	name := flag.String("name", "", "game name shown in the LAN list (default: the host name)")
	seed := flag.Int64("seed", 0, "world seed (0: random)")
	gen := flag.String("gen", "overworld", "world generator: overworld or flat")
	mapName := flag.String("map", "", "play on a VoxelMap (a folder of -maps, e.g. minecity) instead of a generated world")
	maps := flag.String("maps", mp.MapsDir, "directory of the VoxelMaps players can play on (every player needs the same)")
	modeName := flag.String("mode", "dm", "game mode: dm (deathmatch), tdm (team deathmatch) or br (battle royale)")
	interval := flag.Duration("zone-interval", mp.DefaultZoneInterval, "battle royale: how long the energy wall stands before closing in")
	shrink := flag.Duration("zone-shrink", mp.DefaultShrinkTime, "battle royale: how long the wall takes to close in")
	flag.Parse()
	mp.MapsDir = *maps
	if *mapName != "" {
		*gen = "map:" + *mapName
	}
	mode, err := mp.ParseMode(*modeName)
	if err != nil {
		log.Fatal(err)
	}
	if *name == "" {
		h, _ := os.Hostname()
		h, _, _ = strings.Cut(h, ".")
		*name = h + "'s server"
	}
	if *seed == 0 {
		*seed = time.Now().UnixNano() & 0xFFFFFFF
	}
	log.Printf("generating the %s world (seed %d) for %s...", *gen, *seed, mode)
	srv, err := mp.StartServer(mp.ServerConfig{Addr: *addr, Name: *name, Seed: *seed, Generator: *gen,
		Mode: mode, ZoneInterval: *interval, ShrinkTime: *shrink, Reg: voxel.NewRegistry(), Gun: loadGuns()})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%q listening on %s; type 'help' for commands", *name, srv.Addr())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines) // stdin closed (run in the background): keep serving
	}()
	for {
		select {
		case <-quit:
			stop(srv)
			return
		case l, ok := <-lines:
			if !ok {
				lines = nil
				continue
			}
			if !command(srv, l) {
				stop(srv)
				return
			}
		}
	}
}

func stop(srv *mp.Server) {
	log.Print("shutting down")
	srv.Close()
}

// command runs one console line; false on quit.
func command(srv *mp.Server, line string) bool {
	cmd, arg, _ := strings.Cut(strings.TrimSpace(line), " ")
	arg = strings.TrimSpace(arg)
	switch strings.ToLower(cmd) {
	case "":
	case "help", "?":
		fmt.Println("status            list players, by kills")
		fmt.Println("say <text>        message everyone")
		fmt.Println("kick <slot|name>  disconnect a player")
		fmt.Println("time              show the time of day")
		fmt.Println("time set <t>      day, noon, night, midnight or ticks (0-23999)")
		fmt.Println("time add <ticks>  move the time on")
		fmt.Println("time stop|run     freeze or resume the day cycle")
		fmt.Println("quit              stop the server")
	case "status":
		cfg := srv.Config()
		ps := srv.Ranking()
		fmt.Printf("%s on %s: %s, %s seed %d, %d player(s)\n", cfg.Name, srv.Addr(), cfg.Mode, cfg.Generator, cfg.Seed, len(ps))
		fmt.Println(roundLine(srv.Round()))
		if len(ps) > 0 {
			fmt.Printf("%-4s %-16s %-5s %6s %5s %6s %4s %6s\n", "slot", "name", "team", "health", "kills", "deaths", "wins", "ping")
		}
		for _, p := range ps {
			h := strconv.Itoa(p.Health)
			switch {
			case p.Out:
				h = "out"
			case p.Dead:
				h = "dead"
			}
			fmt.Printf("%-4d %-16s %-5s %6s %5d %6d %4d %4dms\n", p.Slot, p.Name, mp.TeamName(p.Team), h, p.Kills, p.Deaths, p.Wins, p.Ping.Milliseconds())
		}
	case "say":
		if arg != "" {
			srv.Say("[server] " + arg)
		}
	case "kick":
		slot, err := strconv.Atoi(arg)
		if err != nil {
			slot = -1
			for _, p := range srv.Status() {
				if strings.EqualFold(p.Name, arg) {
					slot = p.Slot
				}
			}
		}
		if !srv.Kick(slot, "kicked by the server") {
			fmt.Printf("no player %q\n", arg)
		}
	case "time":
		timeCommand(srv, arg)
	case "quit", "exit":
		return false
	default:
		fmt.Printf("unknown command %q (try help)\n", cmd)
	}
	return true
}

// roundLine describes the match state.
func roundLine(r mp.Round) string {
	secs := func(t int) string { return (time.Duration(t) * time.Second / mp.TickRate).String() }
	switch r.Mode {
	case mp.TeamDeathmatch:
		return fmt.Sprintf("Red %d : %d Blue", r.RedKills, r.BlueKills)
	case mp.BattleRoyale:
		switch r.Phase {
		case mp.PhaseWaiting:
			return "waiting for a second player"
		case mp.PhaseCountdown:
			return "the round starts in " + secs(r.Ticks)
		case mp.PhaseOver:
			return "round over"
		}
		z := r.Zone
		state := "closes in " + secs(r.Ticks)
		if r.Shrinking {
			state = "closing, " + secs(r.Ticks) + " to go"
		}
		return fmt.Sprintf("%d alive; zone %.0f blocks at %.0f,%.0f, %s", r.Alive, z.Side(), (z.MinX+z.MaxX)/2, (z.MinZ+z.MaxZ)/2, state)
	}
	return ""
}

// timeCommand is the reference's /time (and doDaylightCycle as stop/run).
func timeCommand(srv *mp.Server, arg string) {
	t, frozen := srv.Time()
	sub, val, _ := strings.Cut(arg, " ")
	val = strings.TrimSpace(val)
	switch strings.ToLower(sub) {
	case "":
		state := "running"
		if frozen {
			state = "stopped"
		}
		fmt.Printf("time %d (day %d, %d of %d), %s\n", t, t/mp.DayLength+1, t%mp.DayLength, mp.DayLength, state)
		return
	case "set":
		n, ok := map[string]int64{"day": mp.TimeDay, "noon": mp.TimeNoon, "night": mp.TimeNight, "midnight": mp.TimeMid}[strings.ToLower(val)]
		if !ok {
			v, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				fmt.Printf("time set: %q is not day, noon, night, midnight or a number\n", val)
				return
			}
			n = v
		}
		t = n
	case "add":
		v, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			fmt.Printf("time add: %q is not a number\n", val)
			return
		}
		t += v
	case "stop":
		frozen = true
	case "run":
		frozen = false
	default:
		fmt.Printf("unknown time command %q (try help)\n", sub)
		return
	}
	srv.SetTime(t, frozen)
	timeCommand(srv, "")
}

// loadGuns resolves the guns players may hold from the TACZ pack (nil when it is
// missing: run from the repo root).
func loadGuns() func(string, tacz.Choice) (weapon.Gun, error) {
	p, err := tacz.LoadDefault()
	if err != nil {
		log.Printf("guns disabled: %v", err)
		return nil
	}
	return p.Gun
}

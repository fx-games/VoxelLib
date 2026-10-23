package app

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"time"

	"github.com/memmaker/voxellib/voxel/render"
)

// startProfiles starts the Debug.Bench profiles named by the environment:
// VOXELLIB_CPUPROFILE (CPU), VOXELLIB_MEMPROFILE (allocations) and
// VOXELLIB_BLOCKPROFILE (blocking), the last two written when the returned
// func runs.
func startProfiles() func() {
	cpu, mem, block := os.Getenv("VOXELLIB_CPUPROFILE"), os.Getenv("VOXELLIB_MEMPROFILE"), os.Getenv("VOXELLIB_BLOCKPROFILE")
	if block != "" {
		runtime.SetBlockProfileRate(int(10 * time.Microsecond))
	}
	if cpu != "" {
		f, err := os.Create(cpu)
		if err == nil {
			err = pprof.StartCPUProfile(f)
		}
		if err != nil {
			log.Print(err)
			cpu = ""
		}
	}
	if mem != "" {
		runtime.MemProfileRate = 64 << 10
	}
	return func() {
		if cpu != "" {
			pprof.StopCPUProfile()
		}
		for name, path := range map[string]string{"allocs": mem, "block": block} {
			if path == "" {
				continue
			}
			f, err := os.Create(path)
			if err == nil {
				err = pprof.Lookup(name).WriteTo(f, 0)
				f.Close()
			}
			if err != nil {
				log.Print(err)
			}
		}
	}
}

// printBenchExtra reports tick times, GPU time (VOXELLIB_BENCH_GPU: submit
// to idle) and the last frame's draws and memory.
func printBenchExtra(ticks, gpu []time.Duration, s render.Stats) {
	stat := func(ds []time.Duration) string {
		if len(ds) == 0 {
			return "n/a"
		}
		var sum time.Duration
		for _, d := range ds {
			sum += d
		}
		slices.Sort(ds)
		return fmt.Sprintf("mean %.2fms p99 %.2fms max %.2fms", ms(sum/time.Duration(len(ds))), ms(ds[len(ds)*99/100]), ms(ds[len(ds)-1]))
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("bench: %d ticks %s | gpu %s\n", len(ticks), stat(ticks), stat(gpu))
	fmt.Printf("bench: %d draws (%d terrain, %d ranges), %d/%d visible sections, %d MiB vertices | heap %d MiB, sys %d MiB, %d GCs\n",
		render.FrameDraws(), s.DrawCalls, s.DrawRanges, s.Visible, s.Sections, s.VertexMemory>>20, m.HeapInuse>>20, m.Sys>>20, m.NumGC)
}

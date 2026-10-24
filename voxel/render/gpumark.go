package render

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/gogpu/wgpu"
)

// GPU time per part of the frame for a bench (VOXELLIB_BENCH_PASSES): each
// GPUMark submits what was recorded since the last one and waits for the
// GPU, so the parts run alone. It costs a submit per mark; bench only.
var (
	gpuMarks    = os.Getenv("VOXELLIB_BENCH_PASSES") != ""
	markPart    string
	gpuPartTime = map[string][]time.Duration{}
	gpuOrder    []string
)

// GPUMark times the GPU work recorded since the last mark (under that
// mark's label) and starts the part named next. Not inside the world part of
// a shader-pack frame, whose pass is open (the pack code marks its passes).
func GPUMark(next string) {
	if dev.pk == nil {
		gpuMark(next)
	}
}

// gpuMark is GPUMark where no pass is open.
func gpuMark(next string) {
	if !gpuMarks || dev.enc == nil {
		return
	}
	t := time.Now()
	// Inside a pack frame submit must not resolve it nor open a clear pass.
	pk, cn, cd := dev.pk, dev.clearNext, dev.clearDepth
	dev.pk, dev.clearNext, dev.clearDepth = nil, false, false
	submit(false)
	dev.pk, dev.clearNext, dev.clearDepth = pk, cn, cd
	dev.device.WaitIdle()
	if markPart != "" {
		if _, ok := gpuPartTime[markPart]; !ok {
			gpuOrder = append(gpuOrder, markPart)
		}
		gpuPartTime[markPart] = append(gpuPartTime[markPart], time.Since(t))
	}
	markPart = next
	var err error
	if dev.enc, err = dev.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "frame"}); err != nil {
		panic(err)
	}
}

// ResetGPUMarks drops the part times so far (the bench warmup).
func ResetGPUMarks() { clear(gpuPartTime); gpuOrder = nil }

// GPUMarkReport is the part times' p50 and mean, in frame order.
func GPUMarkReport() string {
	var b strings.Builder
	for _, k := range gpuOrder {
		ds := gpuPartTime[k]
		var sum time.Duration
		for _, d := range ds {
			sum += d
		}
		slices.Sort(ds)
		fmt.Fprintf(&b, " %s p50 %.2fms mean %.2fms;", k, ms(ds[len(ds)/2]), ms(sum/time.Duration(len(ds))))
	}
	return b.String()
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

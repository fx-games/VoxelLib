package engine

import (
	"log"
	"runtime"

	"golang.org/x/sys/windows"
)

// threadPriorityBelowNormal is THREAD_PRIORITY_BELOW_NORMAL.
const threadPriorityBelowNormal = -1

var setThreadPriority = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadPriority")

// workerThread pins the calling goroutine to its OS thread and lowers that
// thread's priority below the render and tick threads', so the scheduler
// runs a frame first and generation, lighting and meshing in what is left
// (on a 2-core machine they would otherwise take the render thread's core).
func workerThread() {
	runtime.LockOSThread()
	h, err := windows.GetCurrentThread()
	if err != nil {
		log.Printf("engine: GetCurrentThread: %v", err)
		return
	}
	prio := int32(threadPriorityBelowNormal) // passed as the int it is
	if r, _, err := setThreadPriority.Call(uintptr(h), uintptr(prio)); r == 0 {
		log.Printf("engine: SetThreadPriority: %v", err)
	}
}

func renderThread() {}

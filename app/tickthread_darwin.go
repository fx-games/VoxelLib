package app

/*
#include <pthread/qos.h>
*/
import "C"

import "runtime"

// tickThread pins the tick goroutine to its own OS thread at the render
// thread's QoS: unpinned, it shares Go threads with the background workers
// and macOS runs it at default QoS, which under load stretches a tick from
// a few ms to hundreds.
func tickThread() {
	runtime.LockOSThread()
	C.pthread_set_qos_class_self_np(C.QOS_CLASS_USER_INTERACTIVE, 0)
}

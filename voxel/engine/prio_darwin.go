package engine

/*
#include <pthread.h>
#include <pthread/qos.h>
*/
import "C"

import "runtime"

// workerThread pins the calling goroutine to its OS thread and gives that
// thread a lower QoS than the render thread, so macOS preempts background
// meshing/generation instead of delaying a frame (Sodium does the same with
// its chunk builder threads).
func workerThread() {
	runtime.LockOSThread()
	C.pthread_set_qos_class_self_np(C.QOS_CLASS_USER_INITIATED, 0)
}

// renderThread raises the calling (GL) thread to the highest QoS.
func renderThread() {
	C.pthread_set_qos_class_self_np(C.QOS_CLASS_USER_INTERACTIVE, 0)
}

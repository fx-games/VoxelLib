//go:build !darwin

package engine

import "runtime"

// workerThread pins the calling goroutine to its OS thread. Thread priority
// is only adjusted on macOS.
func workerThread() { runtime.LockOSThread() }

func renderThread() {}

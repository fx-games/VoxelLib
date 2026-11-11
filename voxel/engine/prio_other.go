//go:build !darwin && !windows

package engine

import "runtime"

// workerThread pins the calling goroutine to its OS thread. Thread priority
// is adjusted on macOS and Windows only.
func workerThread() { runtime.LockOSThread() }

func renderThread() {}

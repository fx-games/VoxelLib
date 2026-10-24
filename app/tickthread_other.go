//go:build !darwin

package app

import "runtime"

// tickThread pins the tick goroutine to its own OS thread.
func tickThread() { runtime.LockOSThread() }

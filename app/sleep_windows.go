package app

import "time"

// osSleep: ponytail: plain time.Sleep on Windows; use a high-resolution
// waitable timer if frame pacing with -vsync limit hitches there.
func osSleep(d time.Duration) { time.Sleep(d) }

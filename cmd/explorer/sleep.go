package main

/*
#include <time.h>
static void sleepNanos(long long ns) {
	struct timespec ts = { ns / 1000000000LL, ns % 1000000000LL };
	nanosleep(&ts, NULL);
}
*/
import "C"

import "time"

// osSleep blocks the calling OS thread in the kernel. time.Sleep is woken by
// the Go scheduler, which lags up to ~10ms when every P is busy with
// background workers; the kernel wakes the high-priority render thread on
// time.
func osSleep(d time.Duration) { C.sleepNanos(C.longlong(d)) }

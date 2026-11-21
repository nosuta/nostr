//go:build !tinygo

package nip13

import "testing"

// skipIfNoTimerPreemption is a no-op for standard Go, which can preempt
// goroutines in tight loops and deliver timer-based cancellation.
func skipIfNoTimerPreemption(t *testing.T) {}

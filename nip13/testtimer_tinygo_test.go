//go:build tinygo

package nip13

import "testing"

// skipIfNoTimerPreemption skips tests whose cancellation depends on a timer
// firing while a goroutine is in a tight CPU loop. TinyGo/wasm schedules
// cooperatively: without a blocking call the loop never yields, so the timer
// callback cannot run and the test spins forever.
func skipIfNoTimerPreemption(t *testing.T) {
	t.Helper()
	t.Skip("TinyGo/wasm cannot preempt a tight loop to fire timers")
}

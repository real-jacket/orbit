package telemetry

import (
	"testing"
	"time"
)

func TestRateTurnsCountersIntoPerSecondBytes(t *testing.T) {
	if got := rate(1000, 3000, 2*time.Second); got != 1000 {
		t.Errorf("rate = %v, want 1000", got)
	}
	if got := rate(0, 0, time.Second); got != 0 {
		t.Errorf("idle rate = %v, want 0", got)
	}
	// A counter that went backwards was reset or wrapped: reporting a negative
	// rate would draw a spike the machine never had.
	if got := rate(3000, 1000, time.Second); got != 0 {
		t.Errorf("wrapped counter rate = %v, want 0", got)
	}
	if got := rate(1000, 2000, 0); got != 0 {
		t.Errorf("zero-elapsed rate = %v, want 0", got)
	}
}

func TestNetDeltaSumsCountingInterfacesOnly(t *testing.T) {
	previous := netCounters{
		"en0":   {rx: 1000, tx: 500},
		"utun8": {rx: 100, tx: 50},
		"lo0":   {rx: 9000, tx: 9000},
		"awdl0": {rx: 7000, tx: 7000},
	}
	current := netCounters{
		"en0":   {rx: 1500, tx: 900},
		"utun8": {rx: 250, tx: 150},
		"lo0":   {rx: 9900, tx: 9900},
		"awdl0": {rx: 7900, tx: 7900},
	}
	rx, tx := netDelta(previous, current)
	if rx != 650 || tx != 500 {
		t.Errorf("netDelta = (%d, %d), want (650, 500)", rx, tx)
	}
}

func TestNetDeltaIgnoresWrappedAndNewInterfaces(t *testing.T) {
	previous := netCounters{"en0": {rx: 5000, tx: 5000}}
	current := netCounters{
		"en0":  {rx: 100, tx: 100},   // wrapped
		"en11": {rx: 9000, tx: 9000}, // appeared just now, no previous reading
	}
	rx, tx := netDelta(previous, current)
	if rx != 0 || tx != 0 {
		t.Errorf("netDelta = (%d, %d), want (0, 0)", rx, tx)
	}
}

func TestCountsForNetSkipsLoopbackAndPeerToPeer(t *testing.T) {
	cases := map[string]bool{
		"lo0":   false,
		"awdl0": false,
		"llw0":  false,
		"en0":   true,
		"utun8": true,
		"gif0":  true,
	}
	for name, want := range cases {
		if got := countsForNet(name); got != want {
			t.Errorf("countsForNet(%q) = %v, want %v", name, got, want)
		}
	}
}

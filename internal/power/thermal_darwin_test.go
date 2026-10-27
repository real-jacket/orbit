//go:build darwin

package power

import "testing"

// The app must be able to reach -[NSProcessInfo thermalState]: the Swift build
// read ProcessInfo.thermalState, so a false here means the status page is
// degraded to "nominal" for good.
func TestAppKitThermalStateIsReachable(t *testing.T) {
	state, ok := appkitThermalState()
	if !ok {
		t.Fatal("NSProcessInfo thermalState unreachable; thermal pressure will always read nominal")
	}
	if state < thermalStateNominal || state > thermalStateCritical {
		t.Errorf("thermal state = %d, outside the documented 0...3 range", state)
	}
	// A second call goes through the cached selectors.
	if again, ok := appkitThermalState(); !ok || again != state {
		t.Errorf("second call = (%d, %v), want (%d, true)", again, ok, state)
	}
}

// thermalLabel must still answer with a name the frontend understands.
func TestThermalLabelIsKnownWithAppKit(t *testing.T) {
	switch thermalLabel() {
	case "nominal", "fair", "serious", "critical":
	default:
		t.Fatalf("unexpected thermal label %q", thermalLabel())
	}
}

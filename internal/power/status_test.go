package power

import "testing"

func TestTakeReportsThermalAndPressure(t *testing.T) {
	s := Take()
	switch s.Thermal {
	case "nominal", "fair", "serious", "critical":
	default:
		t.Errorf("unexpected thermal label %q", s.Thermal)
	}
	switch s.MemPressure {
	case "normal", "warn", "critical":
	default:
		t.Errorf("unexpected memory pressure label %q", s.MemPressure)
	}
}

func TestParseIoregReadsFlatProperties(t *testing.T) {
	out := `
+-o AppleSmartBattery  <class AppleSmartBattery, id 0x100000abc>
  {
    "CurrentCapacity" = 100
    "MaxCapacity" = 100
    "DesignCapacity" = 4382
    "NominalChargeCapacity" = 4200
    "CycleCount" = 266
    "Temperature" = 2981
    "TimeRemaining" = 65535
    "IsCharging" = No
    "ExternalConnected" = Yes
    "FullyCharged" = Yes
  }
`
	props := parseIoreg(out)
	if got := props["CycleCount"]; got != "266" {
		t.Errorf("CycleCount = %q", got)
	}
	if !flag(props, "ExternalConnected") {
		t.Error("ExternalConnected should be true")
	}
	if flag(props, "IsCharging") {
		t.Error("IsCharging should be false")
	}
	if got := chargePercent(props); got == nil || *got != 100 {
		t.Errorf("charge percent = %v", got)
	}
	if got := healthPercent(props); got == nil || *got != 95 {
		t.Errorf("health percent = %v", got)
	}
	if got := minutesEstimate(props); got != nil {
		t.Errorf("a 65535-minute estimate should be no estimate, got %d", *got)
	}
}

func TestChargePercentCapsAt100(t *testing.T) {
	props := map[string]string{"CurrentCapacity": "105", "MaxCapacity": "100"}
	if got := chargePercent(props); got == nil || *got != 100 {
		t.Errorf("charge percent = %v, want 100", got)
	}
}

func TestHealthPercentCapsAt100(t *testing.T) {
	props := map[string]string{"DesignCapacity": "4000", "NominalChargeCapacity": "4200"}
	if got := healthPercent(props); got == nil || *got != 100 {
		t.Errorf("health percent = %v, want 100", got)
	}
}

func TestHealthPercentFallsBackToRawMaxCapacity(t *testing.T) {
	props := map[string]string{"DesignCapacity": "4000", "AppleRawMaxCapacity": "3600"}
	if got := healthPercent(props); got == nil || *got != 90 {
		t.Errorf("health percent = %v, want 90", got)
	}
}

func TestMemoryPressureLabelIsKnown(t *testing.T) {
	switch memoryPressureLabel() {
	case "normal", "warn", "critical":
	default:
		t.Fatal("unknown memory pressure label")
	}
}

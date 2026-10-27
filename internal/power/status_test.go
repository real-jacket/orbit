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

func TestAdapterWattsReadsNestedDictionary(t *testing.T) {
	// ioreg prints AdapterDetails on one line, with no spaces around the inner
	// "=" signs. Copied from a MacBookPro18,4 on a 65 W charger.
	out := `
+-o AppleSmartBattery  <class AppleSmartBattery, id 0x100000abc>
  {
    "ExternalConnected" = Yes
    "AdapterDetails" = {"IsWireless"=No,"AdapterID"=0,"AdapterVoltage"=20000,"FamilyCode"=18446744073172697098,"AdapterPowerTier"=2,"Watts"=65,"Current"=3250}
  }
`
	props := parseIoreg(out)
	if got := adapterWatts(props); got == nil || *got != 65 {
		t.Errorf("adapter watts = %v, want 65", got)
	}
}

func TestAdapterWattsPrefersFlatProperty(t *testing.T) {
	props := map[string]string{"Watts": "96", "AdapterDetails": `{"Watts"=65}`}
	if got := adapterWatts(props); got == nil || *got != 96 {
		t.Errorf("adapter watts = %v, want 96", got)
	}
}

func TestAdapterWattsAbsentWhenUnplugged(t *testing.T) {
	props := map[string]string{"ExternalConnected": "No"}
	if got := adapterWatts(props); got != nil {
		t.Errorf("adapter watts = %d, want none", *got)
	}
}

func TestNestedNumberIgnoresLookalikes(t *testing.T) {
	dict := `{"Watts"=65,"MaxWatts"=96}`
	if got := nestedNumber(dict, "Watts"); got == nil || *got != 65 {
		t.Errorf("nestedNumberOf Watts = %v, want 65", got)
	}
	if got := nestedNumber("", "Watts"); got != nil {
		t.Errorf("empty dictionary should yield no number, got %d", *got)
	}
	if got := nestedNumber(dict, "Missing"); got != nil {
		t.Errorf("missing key should yield no number, got %d", *got)
	}
}

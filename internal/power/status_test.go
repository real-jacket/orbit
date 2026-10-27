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

// ioreg prints a tree: the properties of the matched node sit behind "|". This
// fixture is trimmed from the real output of `ioreg -rn AppleSmartBattery` on a
// MacBookPro18,4 running macOS 27.2 — the shape that used to parse to nothing.
const ioregTreeOutput = `
+-o AppleSmartBattery  <class AppleSmartBattery, id 0x100000a00, registered, matched, active, busy 0 (27 ms), retain 13>
  | {
  |   "TimeRemaining" = 65535
  |   "ExternalConnected" = Yes
  |   "AdapterDetails" = {"IsWireless"=No,"AdapterID"=0,"AdapterVoltage"=20000,"FamilyCode"=18446744073172697098,"AdapterPowerTier"=2,"Watts"=65,"Current"=3250}
  |   "MaxCapacity" = 100
  |   "CurrentCapacity" = 100
  |   "BatteryData" = {"CurrentCapacity"=100,"AppleRawMaxCapacity"=4843,"DesignCapacity"=6075,"NominalChargeCapacity"=4993,"FullyCharged"=1}
  |   "CycleCount" = 266
  |   "FullyCharged" = Yes
  |   "IsCharging" = No
  | }
`

func TestParseIoregReadsTreeIndentedProperties(t *testing.T) {
	props := parseIoreg(ioregTreeOutput)
	if got := props["CycleCount"]; got != "266" {
		t.Errorf("CycleCount = %q, want 266", got)
	}
	if got := props["ExternalConnected"]; got != "Yes" {
		t.Errorf("ExternalConnected = %q, want Yes", got)
	}
	if got := props["MaxCapacity"]; got != "100" {
		t.Errorf("MaxCapacity = %q, want 100", got)
	}
	if _, ok := props["BatteryData"]; !ok {
		t.Error("BatteryData should be captured as one line")
	}
	if got := adapterWatts(props); got == nil || *got != 65 {
		t.Errorf("adapter watts = %v, want 65", got)
	}
}

func TestBatteryFromIORegBuildsTheCardTheUIExpects(t *testing.T) {
	b := batteryFromIOReg(ioregTreeOutput)
	if b == nil {
		t.Fatal("batteryFromIOReg returned nil; the status page would hide the battery card")
	}
	if b.Percent == nil || *b.Percent != 100 {
		t.Errorf("percent = %v, want 100", b.Percent)
	}
	if b.Cycles == nil || *b.Cycles != 266 {
		t.Errorf("cycles = %v, want 266", b.Cycles)
	}
	if b.AdapterWatts == nil || *b.AdapterWatts != 65 {
		t.Errorf("adapter watts = %v, want 65", b.AdapterWatts)
	}
	if b.Charging {
		t.Error("charging should be false")
	}
	if !b.Plugged {
		t.Error("plugged should be true")
	}
	if !b.Full {
		t.Error("full should be true")
	}
	// TimeRemaining 65535 means "still calculating"; Temperature does not exist
	// on this machine at all, so the card has no figure for it.
	if b.Minutes != nil {
		t.Errorf("minutes = %d, want none", *b.Minutes)
	}
	// Apple silicon keeps the capacity figures inside BatteryData:
	// NominalChargeCapacity 4993 against DesignCapacity 6075 is 82%.
	if b.HealthPct == nil || *b.HealthPct != 82 {
		t.Errorf("healthPct = %v, want 82", b.HealthPct)
	}
	if b.TempC != nil {
		t.Errorf("tempC = %v, want none", *b.TempC)
	}
}

func TestBatteryFromIORegRejectsUnrelatedOutput(t *testing.T) {
	if b := batteryFromIOReg("+-o Root  <class IORegistryEntry>\n"); b != nil {
		t.Errorf("battery = %+v, want nil", b)
	}
}

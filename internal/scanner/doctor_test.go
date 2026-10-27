package scanner

import "testing"

func TestDoctorChecksHaveTitlesAndTriStateResults(t *testing.T) {
	checks := Shared.DoctorChecks()
	if len(checks) == 0 {
		t.Fatal("no checks reported; the machine can always answer at least one of them")
	}
	seen := map[string]bool{}
	for _, check := range checks {
		title, _ := check["title"].(string)
		if title == "" {
			t.Errorf("%v has no title", check)
		}
		if seen[title] {
			t.Errorf("duplicate check %q", title)
		}
		seen[title] = true
		if detail, _ := check["detail"].(string); detail == "" {
			t.Errorf("%s has no detail", title)
		}
		switch check["passed"].(type) {
		case bool, nil:
		default:
			t.Errorf("%s passed = %v (%T), want bool or nil for 未知", title, check["passed"], check["passed"])
		}
	}
}

func TestThermalCheckMapsEveryState(t *testing.T) {
	cases := map[string]any{
		"nominal":  true,
		"fair":     false,
		"serious":  false,
		"critical": false,
		"":         nil,
		"wat":      nil,
	}
	for state, want := range cases {
		if got := thermalCheck(state)["passed"]; got != want {
			t.Errorf("thermal %q passed = %v, want %v", state, got, want)
		}
	}
}

func TestMemoryPressureCheckMapsEveryState(t *testing.T) {
	cases := map[string]any{
		"normal":   true,
		"warn":     false,
		"critical": false,
		"":         nil,
	}
	for state, want := range cases {
		if got := memoryPressureCheck(state)["passed"]; got != want {
			t.Errorf("pressure %q passed = %v, want %v", state, got, want)
		}
	}
}

func TestDiskFreeCheckUsesRealThresholds(t *testing.T) {
	// 990 GB of 1 TB free is fine, 120 GB (12%) is a warning, 50 GB (5%) fails.
	if got := diskFreeCheck(990, 1000)["passed"]; got != true {
		t.Errorf("99%% free passed = %v, want true", got)
	}
	if got := diskFreeCheck(120, 1000)["passed"]; got != false {
		t.Errorf("12%% free passed = %v, want false", got)
	}
	if got := diskFreeCheck(50, 1000)["passed"]; got != false {
		t.Errorf("5%% free passed = %v, want false", got)
	}
	// An unreadable volume is unknown, not a failure.
	if got := diskFreeCheck(0, 0)["passed"]; got != nil {
		t.Errorf("unknown volume passed = %v, want nil", got)
	}
}

func TestBatteryHealthCheckIsLeftOutWithoutAHealthReading(t *testing.T) {
	if _, ok := batteryHealthCheck(nil, intPtr(120), intPtr(80)); ok {
		t.Error("health check reported without a health reading")
	}
	check, ok := batteryHealthCheck(intPtr(81), intPtr(266), intPtr(100))
	if !ok {
		t.Fatal("health check missing for a healthy battery")
	}
	if check["passed"] != true {
		t.Errorf("81%% health passed = %v, want true", check["passed"])
	}
	if check, _ := batteryHealthCheck(intPtr(55), nil, nil); check["passed"] != false {
		t.Errorf("55%% health passed = %v, want false", check["passed"])
	}
}

func TestDoctorRowKeepsUnknownUnknown(t *testing.T) {
	if row := doctorRow("x", nil, "d"); row["passed"] != nil {
		t.Errorf("unknown row passed = %v, want nil", row["passed"])
	}
	if row := doctorRow("x", pass(), "d"); row["passed"] != true {
		t.Errorf("pass row passed = %v, want true", row["passed"])
	}
	if row := doctorRow("x", fail(), "d"); row["passed"] != false {
		t.Errorf("fail row passed = %v, want false", row["passed"])
	}
}

func intPtr(v int) *int { return &v }

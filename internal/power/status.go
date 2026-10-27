// Package power reports battery, thermal and memory-pressure facts for the
// status page and the menu-bar popover.
//
// Everything here comes from public interfaces: the AppleSmartBattery
// IORegistry entry (read through ioreg), the loginwindow's thermal pressure
// sysctl, and kern.memorystatus_vm_pressure_level.
package power

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// NSProcessInfoThermalState, the values -[NSProcessInfo thermalState] returns.
const (
	thermalStateNominal  = 0
	thermalStateFair     = 1
	thermalStateSerious  = 2
	thermalStateCritical = 3
)

// Snapshot is the JSON-ready state of the machine's power and pressure.
type Snapshot struct {
	Thermal     string   `json:"thermal"`
	MemPressure string   `json:"memPressure"`
	Battery     *Battery `json:"battery,omitempty"`
}

// Battery is the state of the machine's battery.
type Battery struct {
	Percent      *int     `json:"percent,omitempty"`
	HealthPct    *int     `json:"healthPct,omitempty"`
	Cycles       *int     `json:"cycles,omitempty"`
	TempC        *float64 `json:"tempC,omitempty"`
	Minutes      *int     `json:"minutes,omitempty"`
	AdapterWatts *int     `json:"adapterWatts,omitempty"`
	Charging     bool     `json:"charging"`
	Plugged      bool     `json:"plugged"`
	Full         bool     `json:"full"`
}

// Take returns the machine's power and pressure state. Battery is absent on
// Macs without one.
func Take() Snapshot {
	s := Snapshot{
		Thermal:     thermalLabel(),
		MemPressure: memoryPressureLabel(),
	}
	s.Battery = batteryInfo()
	return s
}

// thermalLabel maps the thermal pressure the machine reports to the names the
// frontend knows. This is ProcessInfo.thermalState in the Swift build, so ask
// Foundation for the same value first; the sysctl that backs it is only a
// fallback, and "nominal" is the answer of last resort.
func thermalLabel() string {
	if state, ok := appkitThermalState(); ok {
		switch state {
		case thermalStateFair:
			return "fair"
		case thermalStateSerious:
			return "serious"
		case thermalStateCritical:
			return "critical"
		}
		return "nominal"
	}
	switch thermalPressureLevel() {
	case 1:
		return "fair"
	case 2:
		return "serious"
	case 3:
		return "critical"
	}
	return "nominal"
}

var (
	thermalOnce  sync.Once
	thermalLevel int
)

// thermalPressureLevel reads kern.thermal_pressure_level: 0 is nominal, 1
// fair, 2 serious, 3 critical.
func thermalPressureLevel() int {
	thermalOnce.Do(func() {
		out, err := exec.Command("sysctl", "-n", "kern.thermal_pressure_level").Output()
		if err != nil {
			return
		}
		thermalLevel, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	})
	return thermalLevel
}

var (
	pressureOnce  sync.Once
	pressureLevel int
)

// memoryPressureLabel is the memory pressure the kernel reports, as the
// frontend names it.
func memoryPressureLabel() string {
	pressureOnce.Do(func() {
		out, err := exec.Command("sysctl", "-n", "kern.memorystatus_vm_pressure_level").Output()
		if err != nil {
			return
		}
		pressureLevel, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	})
	switch pressureLevel {
	case 4:
		return "critical"
	case 2:
		return "warn"
	}
	return "normal"
}

// batteryInfo reads the AppleSmartBattery IORegistry entry through ioreg,
// whose plain-text output the app can parse without IOKit.
func batteryInfo() *Battery {
	out, err := exec.Command("ioreg", "-rn", "AppleSmartBattery").Output()
	if err != nil {
		return nil
	}
	return batteryFromIOReg(string(out))
}

// batteryFromIOReg turns ioreg's text output into a Battery, or nil when the
// machine has no battery the registry knows about.
func batteryFromIOReg(out string) *Battery {
	props := parseIoreg(out)
	if len(props) == 0 {
		return nil
	}
	b := &Battery{
		Percent:   chargePercent(props),
		HealthPct: healthPercent(props),
		Cycles:    number(props, "CycleCount"),
		Minutes:   minutesEstimate(props),
		Charging:  flag(props, "IsCharging"),
		Plugged:   flag(props, "ExternalConnected"),
		Full:      flag(props, "FullyCharged"),
	}
	if temp := number(props, "Temperature"); temp != nil {
		c := float64(*temp) / 100.0
		b.TempC = &c
	}
	b.AdapterWatts = adapterWatts(props)
	return b
}

// parseIoreg reads the "key" = value lines of ioreg's output.
//
// ioreg prints the registry as a tree, so the properties of the matched node are
// indented behind "|" characters:
//
//	|   "CycleCount" = 266
//
// Nested dictionaries and arrays share the line with their key, which leaves the
// value intact for nestedNumber to look inside.
func parseIoreg(out string) map[string]string {
	props := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "|"))
		if !strings.HasPrefix(line, `"`) {
			continue
		}
		sep := strings.Index(line, `" = `)
		if sep < 1 {
			continue
		}
		key := line[1:sep]
		value := strings.TrimSpace(line[sep+4:])
		props[key] = strings.Trim(strings.TrimSuffix(value, ","), `"`)
	}
	return props
}

// number reads an integer property.
func number(props map[string]string, key string) *int {
	raw, ok := props[key]
	if !ok {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &n
}

// flag reads a boolean property, which ioreg prints as Yes or No.
func flag(props map[string]string, key string) bool {
	return props[key] == "Yes"
}

// chargePercent is the current charge level, in percent.
func chargePercent(props map[string]string) *int {
	current := number(props, "CurrentCapacity")
	maximum := number(props, "MaxCapacity")
	if current == nil || maximum == nil || *maximum <= 0 {
		return nil
	}
	v := *current * 100 / *maximum
	if v > 100 {
		v = 100
	}
	return &v
}

// healthPercent is the full-charge capacity against the design capacity, in
// percent.
func healthPercent(props map[string]string) *int {
	design := number(props, "DesignCapacity")
	if design == nil || *design <= 0 {
		return nil
	}
	full := number(props, "NominalChargeCapacity")
	if full == nil {
		full = number(props, "AppleRawMaxCapacity")
	}
	if full == nil {
		return nil
	}
	v := *full * 100 / *design
	if v > 100 {
		v = 100
	}
	return &v
}

// minutesEstimate is the minutes to empty or to full. Values outside the
// range mean "still calculating".
func minutesEstimate(props map[string]string) *int {
	minutes := number(props, "TimeRemaining")
	if minutes == nil || *minutes <= 0 || *minutes >= 6000 {
		return nil
	}
	return minutes
}

// adapterWatts is the wattage of the connected power adapter.
//
// The value lives inside the AdapterDetails dictionary, which ioreg prints on a
// single line as {"IsWireless"=No,"Watts"=65,...}. Some machines expose a flat
// Watts property instead, so try that first.
func adapterWatts(props map[string]string) *int {
	if flat := number(props, "Watts"); flat != nil {
		return flat
	}
	return nestedNumber(props["AdapterDetails"], "Watts")
}

// nestedNumber pulls an integer out of an ioreg dictionary printed in line.
func nestedNumber(dict, key string) *int {
	if dict == "" {
		return nil
	}
	needle := `"` + key + `"`
	at := strings.Index(dict, needle)
	if at < 0 {
		return nil
	}
	rest := dict[at+len(needle):]
	eq := strings.Index(rest, "=")
	if eq < 0 {
		return nil
	}
	rest = strings.TrimSpace(rest[eq+1:])
	end := 0
	for end < len(rest) && (rest[end] == '-' || rest[end] >= '0' && rest[end] <= '9') {
		end++
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return nil
	}
	return &n
}

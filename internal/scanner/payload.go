package scanner

import (
	"sort"

	"github.com/mymoles/mole-studio/internal/power"
)

// FullScanPayload is what the frontend's onNativeRealScanComplete receives: the
// state of the whole machine, as the Swift app's buildFullScanPayload sent it.
type FullScanPayload struct {
	FreeDiskGb      float64          `json:"freeDiskGb"`
	TotalDiskGb     float64          `json:"totalDiskGb"`
	MemTotalGb      float64          `json:"memTotalGb"`
	CPUModel        string           `json:"cpuModel"`
	AppsList        []map[string]any `json:"appsList"`
	OrphanLeftovers []map[string]any `json:"orphanLeftoversList"`
	LargeFiles      []map[string]any `json:"largeFiles"`
	InstallerFiles  []map[string]any `json:"installerFiles"`
	PurgeItems      []map[string]any `json:"purgeItems"`
	CleanGroups     []map[string]any `json:"cleanGroups"`
	AIMoonBands     []map[string]any `json:"aiMoonBands"`
	// HomePath is the absolute home directory. The page cannot expand ~ on its
	// own, so without it every "move to Trash" in the space analyzer failed.
	HomePath string `json:"homePath"`
	// Whitelist is the editable protection list, without the FINDER_METADATA
	// sentinel and with composite "a & b" lines split apart.
	Whitelist []string `json:"whitelist"`
	// WhitelistWarnings are the entries mole refused to load, so the page can
	// say why a line it shows is not protecting anything.
	WhitelistWarnings []string `json:"whitelistWarnings"`
}

// BuildFullScanPayload measures the whole machine for the frontend's scan
// complete event.
func (s *Scanner) BuildFullScanPayload() FullScanPayload {
	freeGB, totalGB := s.queryDiskSpace()
	_, memTotalGB := s.queryMemoryUsage()
	return FullScanPayload{
		FreeDiskGb:        round2(freeGB),
		TotalDiskGb:       round2(totalGB),
		MemTotalGb:        round2(memTotalGB),
		CPUModel:          s.queryCPUModel(),
		AppsList:          s.scanInstalledApps(),
		OrphanLeftovers:   s.scanOrphanedLeftovers(),
		LargeFiles:        s.scanLargeFiles(),
		InstallerFiles:    s.scanInstallerPackages(),
		PurgeItems:        s.scanBuildArtifacts(),
		CleanGroups:       s.scanCleanGroups(),
		AIMoonBands:       s.scanAICaches(),
		HomePath:          s.home,
		Whitelist:         s.UserWhitelistPaths(),
		WhitelistWarnings: s.refreshWhitelist().Warnings,
	}
}

// Telemetry is what the frontend's onNativeRealTelemetry receives.
type Telemetry struct {
	CPU        float64          `json:"cpu"`
	FreeDiskGb float64          `json:"freeDiskGb"`
	MemUsedGb  float64          `json:"memUsedGb"`
	MemTotalGb float64          `json:"memTotalGb"`
	Processes  []map[string]any `json:"processes"`
	Power      PowerPayload     `json:"power"`
}

// PowerPayload is the power part of the telemetry: thermal, memory pressure,
// and the battery when the machine has one.
type PowerPayload struct {
	Thermal     string         `json:"thermal"`
	MemPressure string         `json:"memPressure"`
	Battery     map[string]any `json:"battery,omitempty"`
}

// batteryPayload is the frontend's shape for the battery, which omits every
// field the battery does not report.
func batteryPayload(b *power.Battery) map[string]any {
	out := map[string]any{
		"charging": b.Charging,
		"plugged":  b.Plugged,
		"full":     b.Full,
	}
	if b.Percent != nil {
		out["percent"] = *b.Percent
	}
	if b.HealthPct != nil {
		out["healthPct"] = *b.HealthPct
	}
	if b.Cycles != nil {
		out["cycles"] = *b.Cycles
	}
	if b.TempC != nil {
		out["tempC"] = *b.TempC
	}
	if b.Minutes != nil {
		out["minutes"] = *b.Minutes
	}
	if b.AdapterWatts != nil {
		out["adapterWatts"] = *b.AdapterWatts
	}
	return out
}

// BuildTelemetry samples the machine for the telemetry event.
func (s *Scanner) BuildTelemetry() Telemetry {
	cpu, processes := s.sampleRealProcesses()
	freeGB, _ := s.queryDiskSpace()
	usedGB, totalGB := s.queryMemoryUsage()
	snapshot := power.Take()
	payload := PowerPayload{
		Thermal:     snapshot.Thermal,
		MemPressure: snapshot.MemPressure,
	}
	if snapshot.Battery != nil {
		payload.Battery = batteryPayload(snapshot.Battery)
	}
	return Telemetry{
		CPU:        round1(cpu),
		FreeDiskGb: round2(freeGB),
		MemUsedGb:  round2(usedGB),
		MemTotalGb: round2(totalGB),
		Processes:  processes,
		Power:      payload,
	}
}

// scanHomeTreemap measures the user's top-level folders for the disk analyzer's
// overview.
func (s *Scanner) scanHomeTreemap() []map[string]any {
	names := []string{
		"Library", "Downloads", "Documents", "Desktop", "Movies",
		"Pictures", "Music", "Projects", "Developer",
	}
	type row struct {
		name  string
		path  string
		bytes int64
	}
	var rows []row
	for _, name := range names {
		path := s.home + "/" + name
		if !exists(path) {
			continue
		}
		size := s.fastDirectorySize(path, 1800)
		if size.bytes > 0 {
			rows = append(rows, row{name: name, path: "~/" + name, bytes: size.bytes})
		}
	}
	var total int64
	for _, r := range rows {
		total += r.bytes
	}
	if total <= 0 {
		return nil
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].bytes > rows[j].bytes })
	tiles := []map[string]any{}
	x := 0.0
	for _, r := range rows {
		pct := float64(r.bytes) / float64(total) * 100
		width := pct
		if width < 8 {
			width = 8
		}
		tiles = append(tiles, map[string]any{
			"name":     r.name,
			"path":     r.path,
			"sizeGb":   gb(r.bytes),
			"pct":      round1(pct),
			"color":    "linear-gradient(145deg, #3b6fd4, #254b9e)",
			"files":    0,
			"x":        x,
			"y":        0,
			"w":        width,
			"h":        100,
			"children": []map[string]any{},
		})
		x += width
	}
	return tiles
}

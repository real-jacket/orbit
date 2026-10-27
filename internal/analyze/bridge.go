// Package analyze maps `mo analyze --json` onto the disk analyzer's treemap.
// It runs the upstream `mo` CLI, which the app does not bundle: the user
// installs it with Homebrew.
package analyze

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// analyzeTimeout bounds one `mo analyze` run, so a stuck mount cannot leave the
// page spinning forever.
const analyzeTimeout = 90 * time.Second

// MoBinaryPath is the path of the `mo` CLI. It tries the places the CLI
// installs itself, then falls back to the Homebrew path.
func MoBinaryPath() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/opt/homebrew/bin/mo",
		"/usr/local/bin/mo",
	}
	if home != "" {
		// With an unset HOME filepath.Join would hand back a relative path, and
		// a file of that name in the working directory would get executed.
		candidates = append(candidates,
			filepath.Join(home, ".local/bin/mo"),
			filepath.Join(home, ".mole/mo"),
		)
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.Mode()&0o111 != 0 {
			return path
		}
	}
	return "/opt/homebrew/bin/mo"
}

// Payload is what the frontend's onNativeDiskAnalyze receives.
type Payload struct {
	Treemap2D  []Tile  `json:"treemap2D"`
	Insights   []Tile  `json:"insights"`
	Source     string  `json:"source"`
	Overview   bool    `json:"overview,omitempty"`
	ScanStatus string  `json:"scanStatus,omitempty"`
	TotalGb    float64 `json:"totalGb"`
	Path       string  `json:"path,omitempty"`
	// Error explains a failed run: "mo" missing, directory unreadable, timeout.
	// Without it every failure looked like the same "unavailable" label.
	Error string `json:"error,omitempty"`
}

// Tile is one rectangle of the treemap.
type Tile struct {
	Name       string  `json:"name"`
	Path       string  `json:"path"`
	SizeGb     float64 `json:"sizeGb"`
	Pct        float64 `json:"pct"`
	Color      string  `json:"color"`
	Files      int     `json:"files"`
	IsDir      bool    `json:"isDir"`
	ScanStatus string  `json:"scanStatus"`
	Cleanable  bool    `json:"cleanable"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	W          float64 `json:"w"`
	H          float64 `json:"h"`
	Children   []Tile  `json:"children"`
}

// Overview analyzes the home directory.
func Overview() Payload {
	stdout, source, failure := runAnalyze("")
	payload := buildPayload(stdout, source)
	payload.Overview = true
	payload.Error = failure
	return payload
}

// Children analyzes one directory.
func Children(path string) Payload {
	stdout, source, failure := runAnalyze(path)
	payload := buildPayload(stdout, source)
	payload.Path = absolutize(path)
	payload.Error = failure
	return payload
}

// runAnalyze runs `mo analyze --json` on path, or on the home directory when
// path is empty, and returns its standard output and the command that stands
// for it.
func runAnalyze(path string) (stdout, source, failure string) {
	binary := MoBinaryPath()
	if _, err := os.Stat(binary); err != nil {
		return "", "mo analyze unavailable", "没有找到 mo CLI，请先安装：brew install mole"
	}
	args := []string{binary, "analyze", "--json"}
	source = "mo analyze --json"
	if path != "" {
		args = append(args, path)
		source += " " + path
	}
	// A full-home scan takes about 30s on a laptop; a network volume or a
	// failing disk never finishes, and the page would sit in "扫描中…" forever.
	ctx, cancel := context.WithTimeout(context.Background(), analyzeTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/env", args...)
	command.Env = mergedEnv(os.Environ(),
		"PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"TERM=dumb",
	)
	var stderr strings.Builder
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", "mo analyze unavailable", fmt.Sprintf("分析超时（%s），目录可能太大或不可读", analyzeTimeout)
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		// `mo` can print usable JSON and still exit non-zero on a partial scan.
		if len(out) == 0 {
			return "", "mo analyze unavailable", firstLine(message)
		}
	}
	return string(out), source, ""
}

// mergedEnv returns base with the keys of overrides replaced. The Swift app
// started `mo` with ProcessInfo.processInfo.environment plus a PATH and TERM
// override, and that inheritance is load-bearing: `mo` is a bash script running
// under `set -u`, so with an empty HOME it dies with
// "base.sh: line 168: HOME: unbound variable" before it analyses anything.
func mergedEnv(base []string, overrides ...string) []string {
	replaced := make(map[string]bool, len(overrides))
	for _, entry := range overrides {
		if key, _, ok := strings.Cut(entry, "="); ok {
			replaced[key] = true
		}
	}
	env := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok && replaced[key] {
			continue
		}
		env = append(env, entry)
	}
	return append(env, overrides...)
}

// buildPayload turns mo's JSON into the treemap the frontend draws.
func buildPayload(stdout, fallbackSource string) Payload {
	var root struct {
		Entries    []map[string]any `json:"entries"`
		Overview   bool             `json:"overview"`
		TotalSize  json.Number      `json:"total_size"`
		ScanStatus string           `json:"scan_status"`
	}
	if err := json.Unmarshal([]byte(stdout), &root); err != nil || root.Entries == nil {
		return Payload{
			Treemap2D: []Tile{},
			Insights:  []Tile{},
			Source:    fallbackSource,
			TotalGb:   0,
		}
	}
	var partitions, insights []map[string]any
	for _, entry := range root.Entries {
		if isInsight(entry) {
			insights = append(insights, entry)
		} else {
			partitions = append(partitions, entry)
		}
	}
	total := parseBytes(root.TotalSize)
	var treemap []Tile
	if total > 0 {
		treemap = layout(partitions, total)
	} else {
		treemap = layout(partitions, sumBytes(partitions))
	}
	return Payload{
		Treemap2D:  treemap,
		Insights:   layout(insights, sumBytes(insights)),
		Source:     fallbackSource,
		Overview:   root.Overview,
		ScanStatus: root.ScanStatus,
		TotalGb:    plainGB(total),
	}
}

// isInsight reports whether mo marked an entry as an insight rather than a
// partition of the directory.
func isInsight(entry map[string]any) bool {
	insight, _ := entry["insight"].(bool)
	return insight
}

// layout turns mo's entries into rectangles, left to right in the order mo
// listed them.
func layout(entries []map[string]any, total int64) []Tile {
	home, _ := os.UserHomeDir()
	type parsed struct {
		name      string
		path      string
		bytes     int64
		isDir     bool
		status    string
		cleanable bool
	}
	var items []parsed
	for _, entry := range entries {
		name, _ := entry["name"].(string)
		path, _ := entry["path"].(string)
		if name == "" || path == "" {
			continue
		}
		bytes := toInt64(entry["size"])
		isDir, _ := entry["is_dir"].(bool)
		status, _ := entry["scan_status"].(string)
		// A directory mo could not read reports size 0 with a scan_status that
		// is not "complete". Dropping it hid the gap: the treemap simply did
		// not add up and nothing said why.
		if bytes <= 0 && (status == "" || status == "complete") {
			continue
		}
		cleanable, _ := entry["cleanable"].(bool)
		items = append(items, parsed{
			name: name, path: path, bytes: bytes,
			isDir: isDir, status: status, cleanable: cleanable,
		})
	}
	basis := total
	if basis <= 0 {
		for _, item := range items {
			basis += item.bytes
		}
	}
	if basis <= 0 {
		return []Tile{}
	}
	tiles := make([]Tile, 0, len(items))
	x := 0.0
	for _, item := range items {
		pct := float64(item.bytes) / float64(basis) * 100
		width := pct
		if width < 6 {
			width = 6
		}
		color := "linear-gradient(145deg, #0284c7, #0c4a6e)"
		if item.cleanable {
			color = "linear-gradient(145deg, #059669, #064e3b)"
		} else if item.isDir {
			color = "linear-gradient(145deg, #d97706, #78350f)"
		}
		tiles = append(tiles, Tile{
			Name:       item.name,
			Path:       relativize(item.path, home),
			SizeGb:     plainGB(item.bytes),
			Pct:        round1(pct),
			Color:      color,
			Files:      0,
			IsDir:      item.isDir,
			ScanStatus: item.status,
			Cleanable:  item.cleanable,
			X:          x,
			Y:          0,
			W:          width,
			H:          100,
			Children:   []Tile{},
		})
		x += width
	}
	return tiles
}

// sumBytes adds up the size of every entry.
func sumBytes(entries []map[string]any) int64 {
	var total int64
	for _, entry := range entries {
		total += toInt64(entry["size"])
	}
	return total
}

// absolutize turns the app's "~"-relative paths into absolute ones, because
// `mo analyze` does not expand them and the page has no home directory of its
// own (it also could not expand them on its own before HomePath was sent).
func absolutize(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if path == "~/" || strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, path)
}

// firstLine keeps the first meaningful line of `mo`'s stderr, which is what the
// user needs to see (the rest is a shell trace or a wall of unrelated output).
func firstLine(message string) string {
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Trace/breakpoint trap") {
			continue
		}
		const limit = 200
		if runes := []rune(line); len(runes) > limit {
			return string(runes[:limit]) + "…"
		}
		return line
	}
	return "分析失败"
}

// relativize replaces the home directory at the start of path with ~.
func relativize(path, home string) string {
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// parseBytes reads a byte count, which mo may encode as a number or a string.
func parseBytes(value json.Number) int64 {
	if value == "" {
		return 0
	}
	if n, err := value.Int64(); err == nil {
		return n
	}
	if f, err := value.Float64(); err == nil {
		return int64(f)
	}
	return 0
}

// toInt64 reads an integer from a JSON value.
func toInt64(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}

// plainGB converts bytes to gigabytes, rounded to two decimals.
func plainGB(bytes int64) float64 {
	return round2(float64(bytes) / 1_000_000_000)
}

// round1 rounds v to one decimal place.
func round1(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*10+0.5)) / 10
	}
	return float64(int64(v*10-0.5)) / 10
}

// round2 rounds v to two decimal places.
func round2(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*100+0.5)) / 100
	}
	return float64(int64(v*100-0.5)) / 100
}

// Package analyze maps the in-process directory tree onto the disk analyzer.
package analyze

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Payload is what the frontend's onNativeDiskAnalyze receives.
type Payload struct {
	Treemap2D  []Tile  `json:"treemap2D"`
	Insights   []Tile  `json:"insights"`
	Source     string  `json:"source"`
	Overview   bool    `json:"overview,omitempty"`
	ScanStatus string  `json:"scanStatus,omitempty"`
	TotalGb    float64 `json:"totalGb"`
	Path       string  `json:"path,omitempty"`
	// Error explains a failed run: unreadable directory, timeout, no `mo`.
	// Without it every failure looked like the same "unavailable" label.
	Error     string `json:"error,omitempty"`
	RequestID string `json:"requestId,omitempty"`

	// The rest describes the scan itself rather than the rectangles: which
	// volume the numbers came from, how much of the tree was readable, and
	// what the numbers mean. `tiles` duplicates `treemap2D` for the newer
	// page code, which reads the documented name.
	Tiles          []Tile            `json:"tiles,omitempty"`
	ScanID         string            `json:"scanId,omitempty"`
	Root           string            `json:"root,omitempty"`
	TotalSizeGb    float64           `json:"totalSizeGb,omitempty"`
	TotalLogicalGb float64           `json:"totalLogicalGb,omitempty"`
	FileCount      int64             `json:"fileCount,omitempty"`
	Partial        bool              `json:"partial,omitempty"`
	Restricted     []RestrictionInfo `json:"restricted,omitempty"`
	Excluded       []RestrictionInfo `json:"excluded,omitempty"`
	Aggregated     *Aggregated       `json:"aggregated,omitempty"`
	LargeFiles     []LargeFileTile   `json:"largeFiles,omitempty"`
	Volume         *Volume           `json:"volume,omitempty"`
}

// Volume is the filesystem the scan ran on.
type Volume struct {
	TotalGb float64 `json:"totalGb"`
	UsedGb  float64 `json:"usedGb"`
	FreeGb  float64 `json:"freeGb"`
}

// Aggregated counts the children the scan folded into "…其它".
type Aggregated struct {
	Count  int              `json:"count"`
	SizeGb float64          `json:"sizeGb"`
	Items  []AggregatedItem `json:"items,omitempty"`
}

// AggregatedItem is a bounded sample of the direct children folded into
// "其它". It is meant for inspection and opt-in collection, not automatic
// deletion.
type AggregatedItem struct {
	Name             string  `json:"name"`
	Path             string  `json:"path"`
	SizeGb           float64 `json:"sizeGb"`
	FileCount        int64   `json:"fileCount,omitempty"`
	IsDir            bool    `json:"isDir"`
	Modified         int64   `json:"modified,omitempty"`
	Partial          bool    `json:"partial,omitempty"`
	ProtectionReason string  `json:"protectionReason,omitempty"`
}

// RestrictionInfo is one path the scan could not read or chose not to visit.
type RestrictionInfo struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Kind   string `json:"kind,omitempty"` // permission | system, set by the app once it knows the Full Disk Access state
}

const (
	ReasonDenied   = "权限不足"
	ReasonNotOwner = "只有该目录的所有者（root 或其他用户）能读取，完全磁盘访问权限也无法放行"
)

// LargeFileTile is one entry of the large-file list.
type LargeFileTile struct {
	Name             string  `json:"name"`
	Path             string  `json:"path"`
	SizeGb           float64 `json:"sizeGb"`
	ProtectionReason string  `json:"protectionReason,omitempty"`
}

// Tile is one rectangle of the treemap.
type Tile struct {
	Name       string  `json:"name"`
	Path       string  `json:"path"`
	SizeGb     float64 `json:"sizeGb"`
	DiskGb     float64 `json:"diskGb,omitempty"`
	LogicalGb  float64 `json:"logicalGb,omitempty"`
	Partial    bool    `json:"partial,omitempty"`
	Modified   int64   `json:"modified,omitempty"`
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
	// Preview is a bounded, non-interactive sample of this directory's direct
	// children. The page draws it inside the tile so a large single block still
	// reveals what is taking the space.
	Preview          []Tile `json:"preview,omitempty"`
	ProtectionReason string `json:"protectionReason,omitempty"`
}

// PayloadFor returns the treemap of path, scanning it when the in-memory tree
// does not cover it yet. Drilling into a directory the tree already holds costs
// a map lookup. Failure and cancellation never start a second CLI scan.
func PayloadFor(path string, refresh bool) Payload {
	node, stats, scanID, err := Shared.Tree(path, refresh)
	if err != nil {
		status := "error"
		if errors.Is(err, context.Canceled) {
			status = "canceled"
		}
		return Payload{Path: absolutize(path), Overview: path == "", Source: "orbit 本地目录扫描", ScanID: scanID, ScanStatus: status, Error: err.Error()}
	}
	payload := payloadForNode(node, stats, scanID, Shared.Root(), absolutize(path))
	payload.Overview = path == ""
	return payload
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

// relativize replaces the home directory at the start of path with ~.
func relativize(path, home string) string {
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// plainGB converts bytes to gigabytes, rounded to two decimals.
func plainGB(bytes int64) float64 {
	return float64(bytes) / 1_000_000_000
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

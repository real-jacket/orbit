package analyze

import (
	"os"
	"testing"
)

const sampleAnalyzeJSON = `{
  "scan_status": "partial",
  "path": "/Users/me",
  "overview": true,
  "entries": [
    {
      "scan_status": "complete",
      "name": "Documents",
      "path": "/Users/me/Documents",
      "size": 526151707463,
      "is_dir": true
    },
    {
      "scan_status": "partial",
      "name": "Library",
      "path": "/Users/me/Library",
      "size": 45520041310,
      "is_dir": true,
      "cleanable": true
    },
    {
      "scan_status": "complete",
      "name": "notes.txt",
      "path": "/Users/me/notes.txt",
      "size": 1234567890,
      "is_dir": false
    },
    {
      "scan_status": "complete",
      "name": "tip",
      "path": "/Users/me/tip",
      "size": 999,
      "insight": true
    }
  ],
  "total_size": 709980057685
}`

func TestBuildPayloadSplitsInsightsFromPartitions(t *testing.T) {
	payload := buildPayload(sampleAnalyzeJSON, "mo analyze --json")
	if len(payload.Treemap2D) != 3 {
		t.Errorf("treemap tiles = %d, want 3", len(payload.Treemap2D))
	}
	if len(payload.Insights) != 1 {
		t.Errorf("insight tiles = %d, want 1", len(payload.Insights))
	}
	if !payload.Overview {
		t.Error("overview should be true")
	}
	if payload.ScanStatus != "partial" {
		t.Errorf("scan status = %q", payload.ScanStatus)
	}
	if payload.Source != "mo analyze --json" {
		t.Errorf("source = %q", payload.Source)
	}
}

func TestBuildPayloadSizesAgainstTotalSize(t *testing.T) {
	payload := buildPayload(sampleAnalyzeJSON, "mo analyze --json")
	documents := payload.Treemap2D[0]
	if documents.Name != "Documents" {
		t.Fatalf("first tile = %q", documents.Name)
	}
	if want := 526.15; documents.SizeGb != want {
		t.Errorf("size GB = %v, want %v", documents.SizeGb, want)
	}
	wantPct := 526151707463.0 / 709980057685.0 * 100
	if diff := documents.Pct - round1(wantPct); diff > 0.11 || diff < -0.11 {
		t.Errorf("pct = %v, want %v", documents.Pct, round1(wantPct))
	}
	if documents.W < 6 {
		t.Errorf("width %v is below the 6%% floor", documents.W)
	}
	if documents.H != 100 || documents.Y != 0 {
		t.Errorf("height %v and y %v should fill the strip", documents.H, documents.Y)
	}
}

func TestBuildPayloadColorsByCleanability(t *testing.T) {
	payload := buildPayload(sampleAnalyzeJSON, "")
	if got := payload.Treemap2D[0].Color; got != "linear-gradient(145deg, #d97706, #78350f)" {
		t.Errorf("a directory that is not cleanable should be amber, got %q", got)
	}
	if got := payload.Treemap2D[1].Color; got != "linear-gradient(145deg, #059669, #064e3b)" {
		t.Errorf("a cleanable directory should be green, got %q", got)
	}
	if got := payload.Treemap2D[2].Color; got != "linear-gradient(145deg, #0284c7, #0c4a6e)" {
		t.Errorf("a file should be blue, got %q", got)
	}
}

func TestBuildPayloadRelativizesTheHomeDirectory(t *testing.T) {
	payload := buildPayload(sampleAnalyzeJSON, "")
	home := homeForTest(t)
	want := relativize("/Users/me/Documents", home)
	if payload.Treemap2D[0].Path != want {
		t.Errorf("path = %q, want %q", payload.Treemap2D[0].Path, want)
	}
}

func TestBuildPayloadFallsBackOnBadJSON(t *testing.T) {
	payload := buildPayload("not json", "mo analyze unavailable")
	if payload.Treemap2D == nil || len(payload.Treemap2D) != 0 {
		t.Errorf("treemap = %v, want empty", payload.Treemap2D)
	}
	if payload.Insights == nil || len(payload.Insights) != 0 {
		t.Errorf("insights = %v, want empty", payload.Insights)
	}
	if payload.Source != "mo analyze unavailable" {
		t.Errorf("source = %q", payload.Source)
	}
	if payload.TotalGb != 0 {
		t.Errorf("total GB = %v, want 0", payload.TotalGb)
	}
}

func TestLayoutAddsUpPositions(t *testing.T) {
	payload := buildPayload(sampleAnalyzeJSON, "")
	var width float64
	for _, tile := range payload.Treemap2D {
		if tile.X != width {
			t.Errorf("%s starts at %v, want %v", tile.Name, tile.X, width)
		}
		width += tile.W
	}
}

func TestMoBinaryPathIsAbsolute(t *testing.T) {
	if path := MoBinaryPath(); path == "" || path[0] != '/' {
		t.Errorf("binary path = %q", path)
	}
}

// homeForTest returns the home directory the tests relativize against.
func homeForTest(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home directory: %v", err)
	}
	return home
}

package scanner

import (
	"os"
	"path/filepath"
	"time"
)

const fullMeasureBudget = 120 * time.Second

// fullMeasureQueue lets one on-demand measurement walk at a time, so it holds at
// most one of the two walkSlots and a running scan keeps the other.
var fullMeasureQueue = make(chan struct{}, 1)

// MeasureOutcome is the answer to one on-demand full measurement.
type MeasureOutcome struct {
	Path    string  `json:"path"`
	OK      bool    `json:"ok"` // false: refused, nothing was measured
	Bytes   int64   `json:"bytes"`
	SizeGb  float64 `json:"sizeGb"`
	Files   int     `json:"files"`
	Partial bool    `json:"partial"`
	Reason  string  `json:"reason,omitempty"`
}

// MeasurePathFully measures one path without the scan's file cap. Only a path
// inside home that cleanupReason would currently let through is measured, so the
// button can never be used to probe protected or foreign locations. The move to
// the Trash still re-checks everything with live=true.
func (s *Scanner) MeasurePathFully(raw string) MeasureOutcome {
	return s.measureFully(raw, fullMeasureBudget)
}

func (s *Scanner) measureFully(raw string, budget time.Duration) MeasureOutcome {
	path := filepath.Clean(s.expand(raw))
	out := MeasureOutcome{Path: path}
	if s.home == "" || s.home == "/" || !filepath.IsAbs(path) || path == s.home || !withinPath(path, s.home) {
		out.Reason = "只能测量家目录内的路径"
		return out
	}
	if _, err := os.Lstat(path); err != nil {
		out.Reason = "路径不存在或无法读取：" + err.Error()
		return out
	}
	if reason := s.CleanupReason(path); reason != "" {
		out.Reason = reason
		return out
	}
	fullMeasureQueue <- struct{}{}
	defer func() { <-fullMeasureQueue }()
	size := s.measureDirectory(path, 0, budget)
	out.OK, out.Bytes, out.SizeGb, out.Files, out.Partial = true, size.bytes, plainGB(size.bytes), size.files, size.partial
	if size.partial {
		if size.timedOut {
			out.Reason = "超过 " + itoa64(int64(budget/time.Second)) + " 秒仍未读完，体积只是已读取部分"
		} else {
			out.Reason = "部分目录无法读取（权限或读取错误），体积只是已读取部分"
		}
	}
	return out
}

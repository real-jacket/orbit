package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Ported from tw93/Mole V1.72.0 lib/clean/apps.sh _mole_launch_agent_job_can_run
// Unknown plists, PATH commands and ambiguous interpreter flags keep their claim.
func (s *Scanner) launchAgentCanRun(ctx context.Context, path string, mounts []string) bool {
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() || ctx.Err() != nil {
		return true
	}
	out, err := s.runCmd(ctx, "", "/usr/bin/plutil", "-convert", "json", "-o", "-", path)
	var props map[string]json.RawMessage
	if err != nil || ctx.Err() != nil || json.Unmarshal([]byte(out), &props) != nil {
		return true
	}
	var program string
	var args []string
	if value, present := props["Program"]; present {
		if json.Unmarshal(value, &program) != nil || program == "" {
			return true
		}
	}
	argsKnown := json.Unmarshal(props["ProgramArguments"], &args) == nil
	if program == "" && argsKnown && len(args) > 0 {
		program = args[0]
	}
	if s.launchAgentPathMissing(ctx, program, mounts) {
		return false
	}
	if argsKnown && len(args) > 1 {
		switch filepath.Base(program) {
		case "sh", "bash", "zsh", "fish", "node", "python", "python3", "ruby", "perl", "osascript":
			if s.launchAgentPathMissing(ctx, args[1], mounts) {
				return false
			}
		}
	}
	return true
}

// Ported from tw93/Mole V1.72.0 lib/clean/apps.sh _mole_launch_agent_path_provably_missing
// shortcut: only local install roots and plain script arguments are judged; extend when another launcher needs it.
func (s *Scanner) launchAgentPathMissing(ctx context.Context, path string, mounts []string) bool {
	if ctx.Err() != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.HasPrefix(path, "/.") || withinPath(path, "/Volumes") {
		return false
	}
	local := false
	for _, root := range []string{s.home, "/Applications", "/opt/homebrew", "/usr/local"} {
		local = local || withinPath(path, root)
	}
	if !local {
		return false
	}
	for _, root := range mounts {
		if withinPath(path, root) {
			return false
		}
	}
	for _, name := range []string{"Desktop", "Documents", "Downloads", "Library/Mobile Documents", "Library/CloudStorage", "Library/Containers", "Library/Group Containers"} {
		if withinPath(path, filepath.Join(s.home, name)) {
			return false
		}
	}
	// Probe before filesystem reads so a stale local mount cannot hold up judging.
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := s.runCmd(probeCtx, "", "/usr/bin/env", "LC_ALL=C", "/usr/bin/stat", "-f", "%N", "--", path)
	var exit *exec.ExitError
	if err == nil || probeCtx.Err() != nil || !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.HasSuffix(strings.ToLower(err.Error()), syscall.ENOENT.Error()) {
		return false
	}
	parent := "/"
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if ctx.Err() != nil {
			return false
		}
		current := filepath.Join(parent, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			_, err := readDirEntries(parent)
			return err == nil && ctx.Err() == nil
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false
		}
		parent = current
	}
	return false
}

var launchAgentMountFiles = []struct{ path, format string }{{"/etc/fstab", "fstab"}, {"/etc/auto_master", "master"}}

// Ported from tw93/Mole V1.72.0 lib/clean/apps.sh _mole_launch_agent_load_mount_points
// An unplugged configured volume is not an uninstalled program.
func configuredLaunchAgentMounts() ([]string, error) {
	var mounts []string
	var read func(string, string) error
	read = func(path, format string) error {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("cannot read mount configuration %s", path)
		}
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		if err != nil || len(data) > 1<<20 {
			return fmt.Errorf("cannot read mount configuration %s", path)
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 || strings.HasPrefix(fields[0], "#") || strings.HasPrefix(fields[0], "+") {
				continue
			}
			point := fields[0]
			if format == "fstab" {
				if len(fields) < 2 {
					return fmt.Errorf("invalid mount configuration %s", path)
				}
				point = fields[1]
			}
			if format == "master" && point == "/-" {
				if len(fields) < 2 {
					return fmt.Errorf("unknown direct mount map %s", path)
				}
				if !strings.HasPrefix(fields[1], "-") {
					mapPath := fields[1]
					if !filepath.IsAbs(mapPath) {
						mapPath = filepath.Join(filepath.Dir(path), mapPath)
					}
					if err := read(mapPath, "direct"); err != nil {
						return err
					}
				}
				continue
			}
			point = strings.ReplaceAll(point, `\040`, " ")
			if filepath.IsAbs(point) && point != "/" {
				mounts = append(mounts, filepath.Clean(point))
			}
		}
		return nil
	}
	for _, config := range launchAgentMountFiles {
		if err := read(config.path, config.format); err != nil {
			return nil, err
		}
	}
	return mounts, nil
}

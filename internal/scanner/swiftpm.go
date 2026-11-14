package scanner

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Ported from tw93/Mole V1.72.0 lib/clean/purge_shared.sh MOLE_SWIFTPM_BUILD_OUTPUT_DIRS
var swiftPMOutputDirs = []string{
	"out", "index-build", "arm64-apple-macosx", "x86_64-apple-macosx",
	"aarch64-unknown-linux-gnu", "x86_64-unknown-linux-gnu",
}

// Ported from tw93/Mole V1.72.0 lib/clean/project.sh _is_swiftpm_scratch_root
func isSwiftPMScratchRoot(dir string) bool {
	if !realDir(dir) {
		return false
	}
	name := filepath.Base(dir)
	if name == ".build" {
		return true
	}
	if isArtifactName(name) || !hasCacheDirTag(dir) {
		return false
	}
	file, err := os.Open(filepath.Join(dir, "CACHEDIR.TAG"))
	if err != nil {
		return false
	}
	defer file.Close()
	tag, err := io.ReadAll(io.LimitReader(file, 4096))
	return err == nil && strings.Contains(string(tag), "Swift Package Manager")
}

// Ported from tw93/Mole V1.72.0 lib/clean/project.sh is_swiftpm_packaged_scratch_dir
func swiftPMPackagedScratch(dir string) bool {
	root := dir
	if filepath.Base(dir) == "index-build" {
		root = filepath.Dir(dir)
	}
	if !realDir(dir) || !isSwiftPMScratchRoot(root) {
		return false
	}
	// Binary-only packages need their downloaded artifacts kept too.
	for _, name := range []string{"checkouts", "repositories", "artifacts"} {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return true
		}
		entries, err := readDirEntries(path)
		if err != nil || len(entries) > 0 {
			return true
		}
	}
	return false
}

// Ported from tw93/Mole V1.72.0 lib/clean/project.sh emit_purge_candidate_or_swiftpm_outputs
func swiftPMArtifacts(path string) []string {
	if !swiftPMPackagedScratch(path) {
		return []string{path}
	}
	var paths []string
	for _, name := range swiftPMOutputDirs {
		output := filepath.Join(path, name)
		if realDir(output) {
			paths = append(paths, swiftPMArtifacts(output)...)
		}
	}
	return paths
}

// Ported from tw93/Mole V1.72.0 lib/clean/project.sh is_swiftpm_build_output_artifact
func swiftPMOutputRoot(path string) string {
	parent := filepath.Dir(path)
	if !containsString(swiftPMOutputDirs, filepath.Base(path)) {
		return ""
	}
	if filepath.Base(parent) == "index-build" {
		parent = filepath.Dir(parent)
	}
	if !isSwiftPMScratchRoot(parent) {
		return ""
	}
	return parent
}

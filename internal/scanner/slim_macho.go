package scanner

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// fatSlice is one architecture inside a universal Mach-O file.
type fatSlice struct {
	arch         string
	offset, size uint64
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_fat_slices
// Fat headers are parsed here, so slimming needs no developer tools (lipo).
// A file that is not a plausible fat file reports ok=false.
func readFatSlices(f io.ReaderAt, fileSize int64) ([]fatSlice, bool) {
	var head [8]byte
	if _, err := f.ReadAt(head[:], 0); err != nil {
		return nil, false
	}
	magic := binary.BigEndian.Uint32(head[0:4])
	entrySize := 20
	switch magic {
	case 0xCAFEBABE:
	case 0xCAFEBABF:
		entrySize = 32
	default:
		return nil, false
	}
	count := int(binary.BigEndian.Uint32(head[4:8]))
	if count < 1 || count > 16 {
		return nil, false
	}
	table := make([]byte, count*entrySize)
	if _, err := f.ReadAt(table, 8); err != nil {
		return nil, false
	}
	slices := make([]fatSlice, 0, count)
	for i := 0; i < count; i++ {
		entry := table[i*entrySize:]
		cpu := binary.BigEndian.Uint32(entry[0:4])
		sub := binary.BigEndian.Uint32(entry[4:8]) & 0x00FFFFFF
		var off, size uint64
		if entrySize == 20 {
			off, size = uint64(binary.BigEndian.Uint32(entry[8:12])), uint64(binary.BigEndian.Uint32(entry[12:16]))
		} else {
			off, size = binary.BigEndian.Uint64(entry[8:16]), binary.BigEndian.Uint64(entry[16:24])
		}
		if off == 0 || size == 0 || off+size < off || off+size > uint64(fileSize) {
			return nil, false
		}
		arch := "other"
		switch cpu {
		case 16777228:
			arch = "arm64"
			if sub == 2 {
				arch = "arm64e"
			}
		case 16777223:
			arch = "x86_64"
			if sub == 8 {
				arch = "x86_64h"
			}
		case 7:
			arch = "i386"
		}
		slices = append(slices, fatSlice{arch: arch, offset: off, size: size})
	}
	return slices, true
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_arm_slice
// A file is only thinned when it holds exactly one plain arm64 slice and at
// least one Intel slice, with nothing else (arm64e or unknown slices stay).
func armSlice(slices []fatSlice) (arm fatSlice, intel int64, ok bool) {
	arms := 0
	for _, slice := range slices {
		switch slice.arch {
		case "arm64":
			arm = slice
			arms++
		case "x86_64", "x86_64h", "i386":
			intel += int64(slice.size)
		default:
			return fatSlice{}, 0, false
		}
	}
	if arms != 1 || intel == 0 {
		return fatSlice{}, 0, false
	}
	return arm, intel, true
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_is_thin_arm
func isThinArm(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var head [8]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return false
	}
	return bytes.Equal(head[:], []byte{0xCF, 0xFA, 0xED, 0xFE, 0x0C, 0x00, 0x00, 0x01})
}

// fileArmSlice reads path's fat header; ok is false for any file slimming must leave alone.
func fileArmSlice(path string) (fatSlice, int64, bool) {
	f, err := os.Open(path)
	if err != nil {
		return fatSlice{}, 0, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fatSlice{}, 0, false
	}
	slices, ok := readFatSlices(f, info.Size())
	if !ok {
		return fatSlice{}, 0, false
	}
	return armSlice(slices)
}

// plistValue reads one plist value; dicts become maps, arrays slices and every
// scalar a placeholder, because callers only look at keys.
func plistValue(d *xml.Decoder, start xml.StartElement) any {
	switch start.Name.Local {
	case "dict":
		m := map[string]any{}
		key := ""
		for {
			tok, err := d.Token()
			if err != nil {
				return m
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					key = ""
					_ = d.DecodeElement(&key, &t)
				} else {
					m[key] = plistValue(d, t)
				}
			case xml.EndElement:
				return m
			}
		}
	case "array":
		var list []any
		for {
			tok, err := d.Token()
			if err != nil {
				return list
			}
			switch t := tok.(type) {
			case xml.StartElement:
				list = append(list, plistValue(d, t))
			case xml.EndElement:
				return list
			}
		}
	}
	_ = d.Skip()
	return struct{}{}
}

// plistRoot reads the top-level dict of a plist file, converting a binary one first.
func plistRoot(path string) (map[string]any, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	if isBinaryPlist(data) {
		if data, err = convertPlistToXML(path); err != nil {
			return nil, false
		}
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, false
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "dict" {
			root, _ := plistValue(d, start).(map[string]any)
			return root, root != nil
		}
	}
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_sealed_keys
// Maps every files2 key of a CodeResources seal to true when it is sealed by
// cdhash (nested code with its own signature) and false for a plain file hash.
func sealedKeys(codeResources string) map[string]bool {
	root, ok := plistRoot(codeResources)
	if !ok {
		return nil
	}
	files, _ := root["files2"].(map[string]any)
	keys := make(map[string]bool, len(files))
	for key, value := range files {
		entry, _ := value.(map[string]any)
		_, keys[key] = entry["cdhash"]
	}
	return keys
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_sealing_root
// The directory holding the _CodeSignature that seals this file: Contents for
// app-like bundles, Versions/<v> for frameworks. Empty when none is inside app.
func sealingRoot(file, app string) string {
	for dir := filepath.Dir(file); dir == app || strings.HasPrefix(dir, app+"/"); dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, "_CodeSignature", "CodeResources")); err == nil && info.Mode().IsRegular() {
			return dir
		}
		if dir == app {
			break
		}
	}
	return ""
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_is_main_executable
// A bundle's main executable carries the bundle signature itself, so the seal
// never lists it.
func isMainExecutable(file, root string) bool {
	var plist string
	switch dir := filepath.Dir(file); {
	case dir == filepath.Join(root, "MacOS"):
		plist = filepath.Join(root, "Info.plist")
	case dir == root && strings.Contains(root, ".framework/Versions/"):
		plist = filepath.Join(root, "Resources", "Info.plist")
	default:
		return false
	}
	props, err := parsePlistFile(plist)
	return err == nil && props["CFBundleExecutable"] != "" && props["CFBundleExecutable"] == filepath.Base(file)
}

// slimFile is one file of an app that can lose its Intel code.
type slimFile struct {
	Rel   string // relative to the app
	Intel int64
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_plan_app
// Lists every file that can be thinned without breaking a seal: universal
// Mach-O files that are a bundle's main executable or nested code with its own
// signature, unless an enclosing seal lists the file by plain hash.
func slimPlan(app string) ([]slimFile, int64) {
	app = strings.TrimRight(app, "/")
	seals := map[string]map[string]bool{}
	keysOf := func(root string) map[string]bool {
		keys, ok := seals[root]
		if !ok {
			keys = sealedKeys(filepath.Join(root, "_CodeSignature", "CodeResources"))
			seals[root] = keys
		}
		return keys
	}
	var files []slimFile
	var total int64
	_ = filepath.WalkDir(filepath.Join(app, "Contents"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "Resources" || d.Name() == "_CodeSignature" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if _, intel, ok := fileArmSlice(path); ok {
			root := sealingRoot(path, app)
			if root == "" {
				return nil
			}
			allowed := isMainExecutable(path, root) || keysOf(root)[strings.TrimPrefix(path, root+"/")]
			for outer := root; allowed; {
				if outer = sealingRoot(outer, app); outer == "" {
					break
				}
				if sealed, listed := keysOf(outer)[strings.TrimPrefix(path, outer+"/")]; listed && !sealed {
					allowed = false
				}
			}
			if allowed {
				files = append(files, slimFile{Rel: strings.TrimPrefix(path, app+"/"), Intel: intel})
				total += intel
			}
		}
		return nil
	})
	return files, total
}

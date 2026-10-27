package scanner

import (
	"bytes"
	"encoding/xml"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// parsePlistFile reads the string values of an Info.plist. XML plists are read
// directly; binary plists (which most Apple apps ship) are converted with
// plutil first, because a binary plist is otherwise unreadable and made the
// uninstall list report bundle ids like "local.app.GarageBand" for an app whose
// real identifier is com.apple.garageband10.
func parsePlistFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if props := parsePlist(data); len(props) > 0 {
		return props, nil
	}
	if !isBinaryPlist(data) {
		return nil, errors.New("plist has no string properties")
	}
	converted, err := convertPlistToXML(path)
	if err != nil {
		return nil, err
	}
	props := parsePlist(converted)
	if len(props) == 0 {
		return nil, errors.New("plist has no string properties")
	}
	return props, nil
}

// isBinaryPlist reports whether data starts with the binary plist magic.
func isBinaryPlist(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(data), []byte("bplist00"))
}

// convertPlistToXML asks plutil for an XML copy of a binary plist on stdout.
func convertPlistToXML(path string) ([]byte, error) {
	cmd := exec.Command("plutil", "-convert", "xml1", "-o", "-", "--", path)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

// parsePlist reads the string values of an Info.plist. The app only needs the
// bundle's name, identifier and version, so this reads the XML plist that app
// bundles ship and ignores everything else. A binary plist yields nothing;
// parsePlistFile converts those first.
func parsePlist(data []byte) map[string]string {
	props := map[string]string{}
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("<?xml")) &&
		!bytes.HasPrefix(bytes.TrimSpace(data), []byte("<plist")) {
		return props
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var key string
	var inString bool
	var text strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "key":
				key = ""
			case "string":
				inString = true
				text.Reset()
			}
		case xml.CharData:
			if key == "" {
				key = string(element)
			} else if inString {
				text.Write(element)
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "key":
				key = strings.TrimSpace(key)
			case "string":
				if inString && key != "" {
					props[key] = text.String()
				}
				inString = false
				key = ""
			default:
				key = ""
			}
		}
	}
	return props
}

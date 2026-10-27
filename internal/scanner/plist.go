package scanner

import (
	"bytes"
	"encoding/xml"
	"strings"
)

// parsePlist reads the string values of an Info.plist. The app only needs the
// bundle's name, identifier and version, so this reads the XML plist that app
// bundles ship and ignores everything else. A binary plist yields nothing.
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

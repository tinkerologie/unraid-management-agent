package plugininventory

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const manifestLimit = 1 << 20

type manifest struct {
	Name    string `xml:"name,attr"`
	Author  string `xml:"author,attr"`
	Version string `xml:"version,attr"`
	URL     string `xml:"pluginURL,attr"`
	Support string `xml:"support,attr"`
	Icon    string `xml:"icon,attr"`
	Min     string `xml:"min,attr"`
	Max     string `xml:"max,attr"`
}

func openRegular(path string) (*os.File, os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("expected a regular file")
	}
	f, err := os.Open(path) // #nosec G304 -- paths come from the native plugin directories.
	return f, info, err
}

func parseManifest(path string) (manifest, os.FileInfo, error) {
	f, info, err := openRegular(path)
	if err != nil {
		return manifest{}, nil, err
	}
	defer func() { _ = f.Close() }()
	if info.Size() > manifestLimit {
		return manifest{}, info, fmt.Errorf("manifest exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, manifestLimit+1))
	if err != nil {
		return manifest{}, info, fmt.Errorf("read manifest: %w", err)
	}
	if len(data) > manifestLimit {
		return manifest{}, info, fmt.Errorf("manifest exceeds size limit")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var result manifest
	found := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return manifest{}, info, err
		}
		switch item := token.(type) {
		case xml.Directive:
			if strings.HasPrefix(string(item), "DOCTYPE ") {
				decoder.Entity, err = manifestEntities(string(item))
				if err != nil {
					return manifest{}, info, err
				}
				if exceedsDocumentExpansionLimit(data[decoder.InputOffset():], decoder.Entity) {
					return manifest{}, info, fmt.Errorf("manifest document expansion exceeds size limit")
				}
			}
		case xml.StartElement:
			if found || item.Name.Local != "PLUGIN" {
				return manifest{}, info, fmt.Errorf("expected one PLUGIN root element")
			}
			if err := decoder.DecodeElement(&result, &item); err != nil {
				return manifest{}, info, err
			}
			found = true
		case xml.CharData:
			if strings.TrimSpace(string(item)) != "" {
				return manifest{}, info, fmt.Errorf("unexpected text outside PLUGIN")
			}
		}
	}
	if !found {
		return manifest{}, info, fmt.Errorf("missing PLUGIN root")
	}
	return result, info, nil
}

var (
	entityDecl = regexp.MustCompile(`<!ENTITY[ \t\r\n]+([A-Za-z_:][\w.:-]*)[ \t\r\n]+(?:"([^"]*)"|'([^']*)')[ \t\r\n]*>`)
	entityRef  = regexp.MustCompile(`&([A-Za-z_:][\w.:-]*|#[0-9]+|#x[0-9a-fA-F]+);`)
	dtdComment = regexp.MustCompile(`(?s)<!--.*?-->`)
)

func exceedsDocumentExpansionLimit(data []byte, entities map[string]string) bool {
	budget := manifestLimit
	for _, match := range entityRef.FindAllSubmatchIndex(data, -1) {
		budget -= len(entities[string(data[match[2]:match[3]])])
		if budget < 0 {
			return true
		}
	}
	return false
}

// XML's decoder does not load external entities. Supply only literal
// declarations, resolving nested references with depth and size limits.
func manifestEntities(directive string) (map[string]string, error) {
	literals := make(map[string]string)
	for _, match := range entityDecl.FindAllStringSubmatch(dtdComment.ReplaceAllString(directive, ""), -1) {
		if _, exists := literals[match[1]]; !exists {
			literals[match[1]] = match[2] + match[3]
		}
	}
	expanded := make(map[string]string, len(literals))
	visiting := make(map[string]bool)
	budget := manifestLimit
	var resolve func(string, int) (string, error)
	resolve = func(name string, depth int) (string, error) {
		if literal, ok := xmlEntityLiteral(name); ok {
			return literal, nil
		}
		if value, ok := expanded[name]; ok {
			return value, nil
		}
		value, ok := literals[name]
		if !ok || depth >= 16 || visiting[name] {
			return "", fmt.Errorf("invalid manifest entity reference")
		}
		visiting[name] = true
		var out strings.Builder
		offset := 0
		for _, match := range entityRef.FindAllStringSubmatchIndex(value, -1) {
			replacement, err := resolve(value[match[2]:match[3]], depth+1)
			if err != nil {
				return "", err
			}
			budget -= match[0] - offset + len(replacement)
			if budget < 0 {
				return "", fmt.Errorf("manifest entity expansion exceeds size limit")
			}
			out.WriteString(value[offset:match[0]])
			out.WriteString(replacement)
			offset = match[1]
		}
		budget -= len(value) - offset
		if budget < 0 {
			return "", fmt.Errorf("manifest entity expansion exceeds size limit")
		}
		out.WriteString(value[offset:])
		delete(visiting, name)
		expanded[name] = out.String()
		return expanded[name], nil
	}
	for name := range literals {
		if _, err := resolve(name, 0); err != nil {
			return nil, err
		}
	}
	return expanded, nil
}

func xmlEntityLiteral(name string) (string, bool) {
	switch name {
	case "amp":
		return "&", true
	case "lt":
		return "<", true
	case "gt":
		return ">", true
	case "apos":
		return "'", true
	case "quot":
		return "\"", true
	}
	digits, numeric := strings.CutPrefix(name, "#")
	if !numeric {
		return "", false
	}
	base := 10
	if hex, ok := strings.CutPrefix(digits, "x"); ok {
		base, digits = 16, hex
	}
	value, err := strconv.ParseUint(digits, base, 32)
	if err != nil || value > utf8.MaxRune || !utf8.ValidRune(rune(value)) || (value < 32 && value != 9 && value != 10 && value != 13) {
		return "", false
	}
	return string(rune(value)), true
}

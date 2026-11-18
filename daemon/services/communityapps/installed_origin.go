package communityapps

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
)

const (
	dockerManagedLabel = "net.unraid.docker.managed"
	dockerManagedValue = "dockerman"
)

// InstalledCatalogLink reads only the standard link from the native user
// template of a Docker Manager container. An empty result means unknown origin.
func (s *Service) InstalledCatalogLink(ctx context.Context, name string, labels map[string]string) string {
	if labels[dockerManagedLabel] != dockerManagedValue {
		return ""
	}

	var link string
	matched := false
	entries := 0
	dir := filepath.Join(s.hostRoot, userTemplatesDir)
	err := filepath.WalkDir(dir, func(filename string, entry fs.DirEntry, err error) error {
		entries++
		if err != nil || ctx.Err() != nil || entries > 1024 {
			return fs.ErrInvalid
		}
		if filename != dir && strings.HasPrefix(entry.Name(), ".") && entry.IsDir() {
			return fs.SkipDir
		}
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".xml" || strings.HasPrefix(entry.Name(), ".") {
			return nil
		}

		template, err := readCatalogLinkTemplate(filename)
		if err != nil || len(template.Names) != 1 || template.Names[0] != name {
			return nil
		}
		if matched {
			return fs.ErrInvalid
		}
		matched = true
		if len(template.Links) == 1 {
			link = publicCatalogLink(strings.TrimSpace(template.Links[0]))
		}
		return nil
	})
	if err != nil {
		return ""
	}
	return link
}

type catalogLinkTemplate struct {
	XMLName xml.Name `xml:"Container"`
	Names   []string `xml:"Name"`
	Links   []string `xml:"TemplateURL"`
}

func readCatalogLinkTemplate(filename string) (catalogLinkTemplate, error) {
	var template catalogLinkTemplate
	err := readInstallFile(filename, 1<<20, func(reader io.Reader) error {
		decoder := xml.NewDecoder(reader)
		for {
			token, err := decoder.Token()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			switch value := token.(type) {
			case xml.Directive:
				return fs.ErrInvalid
			case xml.StartElement:
				if template.XMLName.Local != "" || value.Name.Space != "" {
					return fs.ErrInvalid
				}
				if err := decoder.DecodeElement(&template, &value); err != nil {
					return err
				}
			case xml.CharData:
				if strings.TrimSpace(string(value)) != "" {
					return fs.ErrInvalid
				}
			}
		}
	})
	return template, err
}

func publicCatalogLink(value string) string {
	if safeNativeURL(value) == "" || strings.ContainsAny(value, "?#") {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || strings.ContainsAny(parsed.Host, "[]") {
		return ""
	}
	return value
}

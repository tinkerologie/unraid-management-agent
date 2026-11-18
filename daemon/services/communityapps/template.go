package communityapps

import (
	"path"
	"slices"
	"strings"
)

// ConfigKind identifies the meaning of a setting in an Unraid Docker template.
type ConfigKind string

const (
	ConfigPort        ConfigKind = "port"
	ConfigPath        ConfigKind = "path"
	ConfigEnvironment ConfigKind = "environment"
)

// TemplateConfig is a setting declared by the publisher. It can contain
// sensitive defaults and values and belongs to the install service.
type TemplateConfig struct {
	Kind     ConfigKind
	Name     string
	Target   string
	Mode     string
	Default  string `json:"-"`
	Value    string `json:"-"`
	Required bool
	Masked   bool
}

// Template contains the installation settings parsed from the publisher's
// template. It is returned by Resolve for the install service.
type Template struct {
	Name          string
	Image         string
	CatalogLink   string `json:"-"`
	Network       string
	Config        []TemplateConfig
	RuntimeIssues []string
}

// TemplateField is a public view of a setting. Environment fields show
// whether a value or a default is present.
type TemplateField struct {
	Kind       ConfigKind `json:"kind"`
	Name       string     `json:"name,omitempty"`
	Target     string     `json:"target"`
	Mode       string     `json:"mode,omitempty"`
	Default    string     `json:"default,omitempty"`
	Value      string     `json:"value,omitempty"`
	HasDefault bool       `json:"has_default"`
	HasValue   bool       `json:"has_value"`
	Required   bool       `json:"required"`
	Masked     bool       `json:"masked"`
}

func (c TemplateConfig) publicField() TemplateField {
	field := TemplateField{
		Kind: c.Kind, Name: c.Name, Target: c.Target, Mode: c.Mode,
		HasDefault: c.Default != "", HasValue: c.Value != "",
		Required: c.Required, Masked: c.Masked,
	}
	if c.Kind != ConfigEnvironment && !c.Masked {
		field.Default = c.Default
		field.Value = c.Value
	} else if c.Kind == ConfigEnvironment {
		field.Mode = ""
		field.Name = ""
	}
	return field
}

func readTemplate(row phpArray, listing Listing) Template {
	t := Template{Name: listing.Name, Image: listing.Image, CatalogLink: row.text("TemplateURL")}
	t.Network = row.text("Network")
	if t.Network == "" {
		t.Network = row.array("Networking").text("Mode")
	}
	if t.Network == "" {
		t.Network = "bridge"
	}
	if row.flag("Privileged") {
		t.RuntimeIssues = append(t.RuntimeIssues, "privileged")
	}
	if hasTemplateSetting(row, "ExtraParams") || hasTemplateSetting(row, "PostArgs") {
		t.RuntimeIssues = append(t.RuntimeIssues, "raw_docker_arguments")
	}
	if row.flag("TailscaleEnabled") || slices.ContainsFunc(
		[]string{"CPUset", "Memory", "MyIP", "MyMAC", "ExtraNetworks"},
		func(key string) bool { return hasTemplateSetting(row, key) },
	) {
		t.RuntimeIssues = append(t.RuntimeIssues, "advanced_runtime_settings")
	}
	network := strings.ToLower(t.Network)
	switch network {
	case "host", "none", "container", "macvlan", "ipvlan":
		t.RuntimeIssues = append(t.RuntimeIssues, "unsupported_network")
	}
	if strings.HasPrefix(network, "container:") || len(row.array("Network")) != 0 {
		t.RuntimeIssues = append(t.RuntimeIssues, "unsupported_network_settings")
	}
	configs := row.array("Config")
	if configs == nil && hasTemplateSetting(row, "Config") {
		t.RuntimeIssues = append(t.RuntimeIssues, "unsupported_config")
	}
	if configs.get("@attributes") != nil {
		t.addConfig(configs)
	} else {
		for _, config := range configs {
			value, _ := config.value.(phpArray)
			t.addConfig(value)
		}
	}
	// CA adds version 2 when Config is present and the publisher supplied
	// no root attributes. Docker Manager uses legacy mappings otherwise.
	version := row.array("@attributes").text("version")
	if row.get("@attributes") == nil && len(configs) > 0 {
		version = "2"
	}
	if version != "2" && (hasTemplateSetting(row.array("Data"), "Volume") ||
		hasTemplateSetting(row.array("Networking"), "Publish") ||
		hasTemplateSetting(row, "Environment") || hasTemplateSetting(row, "Labels")) {
		t.RuntimeIssues = append(t.RuntimeIssues, "legacy_runtime_settings")
	}
	return t
}

func hasTemplateSetting(row phpArray, key string) bool {
	if a, ok := row.get(key).(phpArray); ok {
		return len(a) != 0
	}
	return strings.TrimSpace(row.text(key)) != "" || row.flag(key)
}

func (t *Template) addConfig(row phpArray) {
	attrs := row.array("@attributes")
	kind := strings.ToLower(attrs.text("Type"))
	config := TemplateConfig{
		Name: attrs.text("Name"), Target: attrs.text("Target"), Mode: attrs.text("Mode"),
		Default: attrs.text("Default"), Value: row.text("value"),
		Required: attrs.flag("Required"), Masked: attrs.flag("Mask"),
	}
	switch kind {
	case "port":
		config.Kind = ConfigPort
	case "path":
		config.Kind = ConfigPath
	case "variable":
		if config.Target == "" || strings.ContainsAny(config.Target, "=\x00\r\n") {
			t.RuntimeIssues = append(t.RuntimeIssues, "invalid_environment_name")
			return
		}
		config.Kind = ConfigEnvironment
	default:
		// Native Docker Manager emits device and named label directives even with empty values.
		if kind == "device" || kind == "label" && config.Target != "" ||
			config.Required || config.Default != "" || config.Value != "" || attrs == nil {
			t.RuntimeIssues = append(t.RuntimeIssues, unsupportedConfigReason(kind))
		}
		return
	}

	t.Config = append(t.Config, config)
}

func unsupportedConfigReason(kind string) string {
	if kind == "device" {
		return "device_passthrough"
	}
	return "unsupported_config"
}

// Inspection reports a socket mount in the published settings. Installation
// planning checks the effective mount again after applying declared overrides.
func (c TemplateConfig) mountsDockerSocket() bool {
	if c.Kind != ConfigPath {
		return false
	}
	source := c.Value
	if source == "" {
		source = c.Default
	}
	return isDockerSocket(source) || source != "" && isDockerSocket(c.Target)
}

func isDockerSocket(value string) bool {
	return path.Base(strings.TrimSpace(value)) == "docker.sock"
}

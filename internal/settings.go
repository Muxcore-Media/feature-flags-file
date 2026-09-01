package internal

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func (m *Module) Settings() []contracts.SettingDef {
	return m.settingsDefs()
}

func (m *Module) UpdateSetting(key, value string) error {
	return m.updateSetting(key, value)
}

func (m *Module) settingsDefs() []contracts.SettingDef {
	m.mu.RLock()
	path := m.filePath
	m.mu.RUnlock()
	yamlText, _ := m.flagsYAML()
	return []contracts.SettingDef{
		{
			Key:         "flags_file",
			Label:       "Flags File",
			Type:        contracts.SettingTypeString,
			Value:       path,
			Default:     "flags.yaml",
			Description: "Path to YAML feature flags (FEATURE_FLAGS_FILE); updates reload immediately",
			Group:       "Flags",
		},
		{
			Key:         "flags_yaml",
			Label:       "Flags YAML",
			Type:        contracts.SettingTypeString,
			Value:       yamlText,
			Default:     "",
			Description: "Inline YAML flag definitions; saved to the flags file on update",
			Group:       "Flags",
		},
	}
}

func (m *Module) updateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	switch key {
	case "flags_file", "FEATURE_FLAGS_FILE":
		if err := validateFlagsPath(value); err != nil {
			return err
		}
		flags, err := m.tryLoadPath(value)
		if err != nil {
			return err
		}
		m.mu.Lock()
		m.filePath = value
		m.flags = flags
		m.lastLoadErr = nil
		m.mu.Unlock()
		slog.Info("feature-flags path updated", "path", value, "count", len(flags))
		return nil
	case "flags_yaml":
		if value == "" {
			return fmt.Errorf("flags_yaml must not be empty")
		}
		return m.writeFlagsFile([]byte(value))
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}

func validateFlagsPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("flags path must not be empty")
	}
	if path == "." || path == ".." {
		return fmt.Errorf("flags path must not be %q", path)
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." {
		return fmt.Errorf("flags path must not resolve to %q", clean)
	}
	return nil
}

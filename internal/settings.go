package internal

import (
	"fmt"
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
	}
}

func (m *Module) updateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	switch key {
	case "flags_file", "FEATURE_FLAGS_FILE":
		if value == "" {
			return fmt.Errorf("flags_file must not be empty")
		}
		m.mu.Lock()
		m.filePath = value
		m.mu.Unlock()
		return m.loadFile()
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}

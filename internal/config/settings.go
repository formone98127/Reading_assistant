package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// UserSettings is persisted under the save directory.
type UserSettings struct {
	OllamaModel string `json:"ollamaModel,omitempty"`
}

func SettingsPath(saveDir string) string {
	return filepath.Join(saveDir, "settings.json")
}

func LoadUserSettings(saveDir string) UserSettings {
	path := SettingsPath(saveDir)
	raw, err := os.ReadFile(path)
	if err != nil {
		return UserSettings{}
	}
	var s UserSettings
	if json.Unmarshal(raw, &s) != nil {
		return UserSettings{}
	}
	return s
}

func SaveUserSettings(saveDir string, s UserSettings) error {
	if saveDir == "" {
		return nil
	}
	path := SettingsPath(saveDir)
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

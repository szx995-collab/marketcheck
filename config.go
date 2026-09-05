package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Settings struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	DeepseekKey string `json:"deepseek_key"`
	OpenAIKey   string `json:"openai_key"`
	FredKey     string `json:"fred_key"`
	Remember    bool   `json:"remember"`
}

func (s Settings) Key() string {
	if s.Provider == "openai" {
		return s.OpenAIKey
	}
	return s.DeepseekKey
}
func (s Settings) Public() map[string]any {
	return map[string]any{"provider": s.Provider, "model": s.Model, "deepseek_configured": s.DeepseekKey != "", "openai_configured": s.OpenAIKey != "", "fred_configured": s.FredKey != "", "remember": s.Remember}
}
func loadSettings(dir string) Settings {
	s := Settings{Provider: "deepseek", Model: "deepseek-v4-flash"}
	if b, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if k := os.Getenv("DEEPSEEK_API_KEY"); k != "" {
		s.DeepseekKey = k
	}
	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		s.OpenAIKey = k
	}
	if k := os.Getenv("FRED_API_KEY"); k != "" {
		s.FredKey = k
	}
	return s
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(path+".tmp", b, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

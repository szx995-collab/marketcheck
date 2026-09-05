package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Settings struct {
	Provider           string `json:"provider"`
	Model              string `json:"model"`
	ReasoningEffort    string `json:"reasoning_effort"`
	DeepseekKey        string `json:"deepseek_key"`
	OpenAIKey          string `json:"openai_key"`
	GLMKey             string `json:"glm_key"`
	KimiKey            string `json:"kimi_key"`
	ClaudeKey          string `json:"claude_key"`
	GrokKey            string `json:"grok_key"`
	CompatibleKey      string `json:"compatible_key"`
	CompatibleBaseURL  string `json:"compatible_base_url"`
	CompatibleJSONMode bool   `json:"compatible_json_mode"`
	FredKey            string `json:"fred_key"`
	Remember           bool   `json:"remember"`
}

func (s Settings) Key() string {
	if key := s.keyFields()[s.Provider]; key != nil {
		return *key
	}
	return ""
}
func (s *Settings) keyFields() map[string]*string {
	return map[string]*string{"deepseek": &s.DeepseekKey, "openai": &s.OpenAIKey, "glm": &s.GLMKey, "kimi": &s.KimiKey, "claude": &s.ClaudeKey, "grok": &s.GrokKey, "compatible": &s.CompatibleKey, "fred": &s.FredKey}
}
func (s Settings) Public() map[string]any {
	out := map[string]any{"provider": s.Provider, "model": s.Model, "reasoning_effort": effectiveEffort(s), "remember": s.Remember, "compatible_base_url": s.CompatibleBaseURL, "compatible_json_mode": s.CompatibleJSONMode}
	for id, key := range s.keyFields() {
		out[id+"_configured"] = *key != ""
	}
	return out
}
func loadSettings(dir string) Settings {
	s := Settings{Provider: "deepseek", Model: "deepseek-v4-flash"}
	if b, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if base := os.Getenv("COMPATIBLE_BASE_URL"); base != "" && base != s.CompatibleBaseURL {
		s.CompatibleBaseURL, s.CompatibleKey = base, ""
	}
	for id, names := range map[string][]string{
		"deepseek": {"DEEPSEEK_API_KEY"}, "openai": {"OPENAI_API_KEY"}, "fred": {"FRED_API_KEY"},
		"glm": {"GLM_API_KEY", "ZHIPUAI_API_KEY"}, "kimi": {"MOONSHOT_API_KEY", "KIMI_API_KEY"},
		"claude": {"ANTHROPIC_API_KEY", "CLAUDE_API_KEY"}, "grok": {"XAI_API_KEY", "GROK_API_KEY"}, "compatible": {"COMPATIBLE_API_KEY"},
	} {
		for _, name := range names {
			if key := os.Getenv(name); key != "" {
				*s.keyFields()[id] = key
				break
			}
		}
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

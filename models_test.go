package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReasoningRequestParameters(t *testing.T) {
	old := llmHTTP
	t.Cleanup(func() { llmHTTP = old })
	cases := []struct {
		provider, model, effort, native, thinking string
		budget                                    int
	}{
		{"openai", "gpt-6-astra", "xhigh", "xhigh", "", 0},
		{"openai", "gpt-6-astra", "max", "max", "", 0},
		{"openai", "gpt-5.6-luna", "none", "none", "", 0},
		{"deepseek", "deepseek-v4-flash", "off", "", "disabled", 0},
		{"deepseek", "deepseek-v4-flash", "low", "low", "enabled", 0},
		{"deepseek", "deepseek-v4-pro", "max", "max", "enabled", 0},
		{"glm", "glm-5.2", "high", "high", "enabled", 0},
		{"glm", "glm-5.2", "max", "max", "enabled", 0},
		{"glm", "glm-5.2", "off", "", "disabled", 0},
		{"glm", "glm-4.7-flash", "on", "", "enabled", 0},
		{"kimi", "kimi-k2.6", "on", "", "enabled", 0},
		{"kimi", "kimi-k2.6", "off", "", "disabled", 0},
		{"kimi", "kimi-k3", "low", "low", "", 0},
		{"kimi", "kimi-k2.7-code", "auto", "", "enabled", 0},
		{"claude", "claude-opus-5", "xhigh", "xhigh", "adaptive", 0},
		{"claude", "claude-sonnet-5", "max", "max", "adaptive", 0},
		{"claude", "claude-sonnet-4-6", "low", "low", "adaptive", 0},
		{"claude", "claude-haiku-4-5-20251001", "low", "", "enabled", 1024},
		{"claude", "claude-haiku-4-5-20251001", "medium", "", "enabled", 4096},
		{"claude", "claude-haiku-4-5-20251001", "high", "", "enabled", 8192},
		{"claude", "claude-haiku-4-5-20251001", "off", "", "disabled", 0},
		{"grok", "grok-4.6", "xhigh", "xhigh", "", 0},
		{"grok", "grok-4.5", "medium", "medium", "", 0},
		{"compatible", "gpt-6-astra", "xhigh", "xhigh", "", 0},
		{"compatible", "deepseek-v4-flash", "high", "high", "enabled", 0},
	}
	for _, c := range cases {
		t.Run(c.provider+"/"+c.model+"/"+c.effort, func(t *testing.T) {
			s := allProviderSettings()
			s.Provider, s.Model, s.ReasoningEffort = c.provider, c.model, c.effort
			llmHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				var p map[string]any
				if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
					t.Fatal(err)
				}
				native := p["reasoning_effort"]
				if c.provider == "claude" {
					config := p["output_config"].(map[string]any)
					native = config["effort"]
					if config["format"].(map[string]any)["type"] != "json_schema" {
						t.Fatal("effort overwrote output schema")
					}
					if p["reasoning_effort"] != nil {
						t.Fatal("wrong Claude effort location")
					}
				}
				if (c.native == "" && native != nil) || (c.native != "" && native != c.native) {
					t.Fatalf("effort got %v, want %q", native, c.native)
				}
				if c.thinking == "" {
					if p["thinking"] != nil {
						t.Fatal("unsupported thinking parameter")
					}
				} else {
					thinking := p["thinking"].(map[string]any)
					if thinking["type"] != c.thinking {
						t.Fatal("wrong thinking switch")
					}
					if c.budget > 0 && (thinking["budget_tokens"] != float64(c.budget) || p["max_tokens"].(float64) <= float64(c.budget)) {
						t.Fatal("invalid thinking budget or no room for answer")
					}
					if c.model == "kimi-k2.7-code" && thinking["keep"] != "all" {
						t.Fatal("missing preserved thinking")
					}
				}
				tokens := p["max_tokens"]
				if c.provider == "openai" || c.model == "gpt-6-astra" || c.model == "kimi-k3" {
					tokens = p["max_completion_tokens"]
					if p["max_tokens"] != nil {
						t.Fatal("wrong token parameter")
					}
				}
				if !oneOf(c.effort, "off", "none") && tokens.(float64) <= 5000 {
					t.Fatal("reasoning still limited to old tiny budget")
				}
				body := `{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`
				if c.provider == "claude" {
					body = `{"content":[{"type":"text","text":"{\"ok\":true}"}],"stop_reason":"end_turn"}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			var out struct {
				OK bool `json:"ok"`
			}
			if err := callLLM(context.Background(), s, "JSON", nil, &out); err != nil || !out.OK {
				t.Fatalf("call failed: %v", err)
			}
		})
	}
}

func TestRejectUnsupportedEffortBeforeSavingOrSending(t *testing.T) {
	cases := []Settings{
		{Provider: "openai", Model: "gpt-4.1-mini", ReasoningEffort: "high"},
		{Provider: "openai", Model: "gpt-6-astra", ReasoningEffort: "none"},
		{Provider: "codex", Model: "", ReasoningEffort: "xhigh"},
		{Provider: "codex", Model: "gpt-5.5", ReasoningEffort: "max"},
		{Provider: "deepseek", Model: "deepseek-v4-flash", ReasoningEffort: "xhigh"},
		{Provider: "glm", Model: "glm-4.7-flash", ReasoningEffort: "high"},
		{Provider: "kimi", Model: "kimi-k3", ReasoningEffort: "off"},
		{Provider: "claude", Model: "claude-sonnet-4-6", ReasoningEffort: "xhigh"},
		{Provider: "grok", Model: "grok-4.5", ReasoningEffort: "xhigh"},
		{Provider: "compatible", Model: "unknown", ReasoningEffort: "high"},
	}
	a, _ := newApp(t.TempDir())
	before := a.config()
	for _, s := range cases {
		w := requestTest(t, a, "POST", "/api/settings", s)
		if w.Code != 400 || a.config() != before {
			t.Fatalf("invalid effort saved: %s", s.Model)
		}
		var out struct {
			OK bool `json:"ok"`
		}
		if err := callLLM(context.Background(), s, "JSON", nil, &out); err == nil || !strings.Contains(err.Error(), "档位") {
			t.Fatalf("effort not validated first: %v", err)
		}
	}
}

func TestDefaultEffortDoesNotOverrideVendorAndCatalogIsConsistent(t *testing.T) {
	var out struct {
		OK bool `json:"ok"`
	}
	for provider, models := range llmModels {
		for _, m := range models {
			s := Settings{Provider: provider, Model: m.ID, ReasoningEffort: m.DefaultEffort}
			if err := validateEffort(s); err != nil {
				t.Fatalf("invalid catalog default for %s: %v", m.ID, err)
			}
			if provider == "codex" {
				continue
			}
			s.ReasoningEffort = "auto"
			p, err := llmPayload(s, "JSON", "{}", &out)
			if err != nil {
				t.Fatal(err)
			}
			if p["reasoning_effort"] != nil {
				t.Fatal("auto overrides effort")
			}
			if m.style != "always" && p["thinking"] != nil {
				t.Fatal("auto overrides thinking")
			}
			if provider == "claude" && p["output_config"].(map[string]any)["effort"] != nil {
				t.Fatal("auto overrides Claude effort")
			}
		}
	}
	unknown := findLLMModel("compatible", "brand-new-model")
	if len(unknown.Efforts) != 1 || unknown.Efforts[0].Value != "auto" {
		t.Fatal("guessed unknown model capability")
	}
}

func TestModelEffortPersistsAndReachesCodex(t *testing.T) {
	a, _ := newApp(t.TempDir())
	a.settings.DeepseekKey, a.settings.FredKey = "preserve-secret", "preserve-fred"
	s := Settings{Provider: "codex", Model: "gpt-6-astra", ReasoningEffort: "xhigh", Remember: true}
	w := requestTest(t, a, "POST", "/api/settings", s)
	if w.Code != 200 || strings.Contains(w.Body.String(), "preserve-") {
		t.Fatal("save failed or leaked credentials")
	}
	a, _ = newApp(a.root)
	if a.config().ReasoningEffort != "xhigh" || a.config().FredKey != "preserve-fred" {
		t.Fatal("restart lost effort or key")
	}
	mockCodex(t, func(ctx context.Context, _, _ string, args []string, _ string) ([]byte, []byte, error) {
		if args[0] == "login" {
			return nil, []byte("Logged in using ChatGPT"), nil
		}
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--model gpt-6-astra") || !strings.Contains(joined, `model_reasoning_effort="xhigh"`) || strings.Contains(joined, `model_reasoning_effort="low"`) {
			t.Fatal("Codex ignored chosen effort")
		}
		return []byte("{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"{\\\"ok\\\":true}\"}}\n{\"type\":\"turn.completed\"}\n"), nil, nil
	})
	w = requestTest(t, a, "POST", "/api/test-model", map[string]any{})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if strings.Contains(strings.Join(codexArgs("", "auto", "instructions", "schema"), " "), "model_reasoning_effort=") {
		t.Fatal("auto still hardcoded")
	}
}

func TestCustomModelListUsesOnlyMatchingCredential(t *testing.T) {
	old := llmHTTP
	t.Cleanup(func() { llmHTTP = old })
	a, _ := newApp(t.TempDir())
	a.settings = allProviderSettings()
	before := a.config()
	calls := 0
	llmHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.String() != "https://model.example/v1/models" || r.Header.Get("Authorization") != "Bearer "+before.CompatibleKey {
			t.Fatal("wrong list endpoint or credential")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-6-astra"},{"id":"new-model"},{"id":"new-model"},{"id":""}]}`))}, nil
	})}
	w := requestTest(t, a, "POST", "/api/models", map[string]any{"base_url": "https://model.example/v1/"})
	var models []LLMModel
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &models) != nil || len(models) != 2 || len(models[1].Efforts) != 1 || a.config() != before || strings.Contains(w.Body.String(), "secret-") {
		t.Fatal("bad list or settings mutated")
	}
	for _, body := range []map[string]any{{"base_url": "https://new.example/v1"}, {"base_url": "https://model.example/v1", "clear_keys": true}} {
		w = requestTest(t, a, "POST", "/api/models", body)
		if w.Code != 400 || calls != 1 {
			t.Fatal("old key reused for another endpoint or after clear")
		}
	}
}

func TestCustomModelListErrorsAreSanitizedAndRedirectsBlocked(t *testing.T) {
	old := llmHTTP
	t.Cleanup(func() { llmHTTP = old })
	a, _ := newApp(t.TempDir())
	a.settings = allProviderSettings()
	for _, status := range []int{302, 401, 404, 500} {
		llmHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "model.example" {
				t.Fatal("followed credential redirect")
			}
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://redirect.example/models"}}, Body: io.NopCloser(strings.NewReader("secret-private-body"))}, nil
		})}
		w := requestTest(t, a, "POST", "/api/models", map[string]any{"base_url": "https://model.example/v1"})
		if w.Code != 400 || strings.Contains(w.Body.String(), "secret-") {
			t.Fatal("unsafe list error")
		}
	}
}

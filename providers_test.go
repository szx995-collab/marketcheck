package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func allProviderSettings() Settings {
	return Settings{DeepseekKey: "secret-deepseek", OpenAIKey: "secret-openai", GLMKey: "secret-glm", KimiKey: "secret-kimi", ClaudeKey: "secret-claude", GrokKey: "secret-grok", CompatibleKey: "secret-compatible", FredKey: "secret-fred", CompatibleBaseURL: "https://model.example/v1"}
}

func TestProviderRequestsUseOwnEndpointKeyAndProtocol(t *testing.T) {
	old := llmHTTP
	t.Cleanup(func() { llmHTTP = old })
	for _, provider := range modelProviders {
		if provider.ID == "codex" {
			continue
		}
		t.Run(provider.ID, func(t *testing.T) {
			s := allProviderSettings()
			s.Provider, s.Model = provider.ID, provider.DefaultModel
			if s.Model == "" {
				s.Model = "my-model"
			}
			calls := 0
			llmHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				expected := provider.Endpoint
				if provider.ID == "compatible" {
					expected = "https://model.example/v1/chat/completions"
				}
				if r.URL.String() != expected || r.Method != "POST" {
					t.Fatalf("wrong destination: %s %s", r.Method, r.URL)
				}
				body, _ := io.ReadAll(r.Body)
				if strings.Contains(string(body), "secret-") {
					t.Fatal("credential included in prompt")
				}
				var payload map[string]any
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				if payload["model"] != s.Model {
					t.Fatal("model changed")
				}
				response := `{"choices":[{"message":{"content":"{\"ok\":true}","reasoning_content":"ignore this"},"finish_reason":"stop"}]}`
				if provider.ID == "claude" {
					if r.Header.Get("x-api-key") != s.ClaudeKey || r.Header.Get("Authorization") != "" || r.Header.Get("anthropic-version") != "2023-06-01" {
						t.Fatal("wrong Anthropic auth headers")
					}
					if payload["system"] == nil || payload["response_format"] != nil || payload["max_tokens"] != float64(5000) {
						t.Fatal("wrong Messages payload")
					}
					messages := payload["messages"].([]any)
					if len(messages) != 1 || messages[0].(map[string]any)["role"] != "user" {
						t.Fatal("system role sent to Messages API")
					}
					format := payload["output_config"].(map[string]any)["format"].(map[string]any)
					if format["type"] != "json_schema" || format["schema"].(map[string]any)["properties"].(map[string]any)["ok"] == nil {
						t.Fatal("missing output schema")
					}
					response = `{"content":[{"type":"thinking","thinking":"ignore this"},{"type":"text","text":"{\"ok\":"},{"type":"text","text":"true}"}],"stop_reason":"end_turn"}`
				} else {
					if r.Header.Get("Authorization") != "Bearer "+s.Key() || r.Header.Get("x-api-key") != "" {
						t.Fatal("wrong provider key")
					}
					if provider.ID == "openai" {
						if payload["max_tokens"] != nil || payload["max_completion_tokens"] != float64(5000) {
							t.Fatal("wrong OpenAI token parameter")
						}
					} else if payload["max_tokens"] == nil || payload["max_completion_tokens"] != nil {
						t.Fatal("wrong compatible token parameter")
					}
					if provider.ID == "compatible" && payload["response_format"] != nil {
						t.Fatal("unsupported JSON mode forced on custom endpoint")
					}
					if provider.ID != "compatible" && payload["response_format"].(map[string]any)["type"] != "json_object" {
						t.Fatal("missing JSON mode")
					}
					if oneOf(provider.ID, "glm", "kimi", "deepseek") && payload["thinking"].(map[string]any)["type"] != "disabled" {
						t.Fatal("default model thinking not disabled")
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
			})}
			var out struct {
				OK bool `json:"ok"`
			}
			if err := callLLM(context.Background(), s, "Return JSON", "test", &out); err != nil || !out.OK || calls != 1 {
				t.Fatalf("provider call failed: %v (%d calls)", err, calls)
			}
		})
	}
}

func TestProviderSettingsPreserveAndClearAllKeys(t *testing.T) {
	a, _ := newApp(t.TempDir())
	s := allProviderSettings()
	s.Provider, s.Model, s.Remember = "claude", "claude-haiku-4-5-20251001", true
	w := requestTest(t, a, "POST", "/api/settings", s)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, id := range []string{"glm", "kimi", "claude", "grok", "compatible", "codex"} {
		p, _ := providerByID(id)
		model := p.DefaultModel
		if id == "compatible" {
			model = "custom-model"
		}
		w = requestTest(t, a, "POST", "/api/settings", Settings{Provider: id, Model: model, Remember: true, CompatibleBaseURL: s.CompatibleBaseURL})
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		w = requestTest(t, a, "GET", "/api/bootstrap", nil)
		if strings.Contains(w.Body.String(), "secret-") {
			t.Fatal("bootstrap returned a credential")
		}
		current := a.config()
		for keyID, key := range current.keyFields() {
			if *key != *s.keyFields()[keyID] {
				t.Fatalf("switching %s lost %s key", id, keyID)
			}
		}
	}
	a, _ = newApp(a.root)
	if a.config().ClaudeKey != s.ClaudeKey || a.config().GrokKey != s.GrokKey {
		t.Fatal("new keys not persisted")
	}
	w = requestTest(t, a, "POST", "/api/settings", map[string]any{"provider": "codex", "model": "", "remember": true, "clear_keys": true})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	a, _ = newApp(a.root)
	current := a.config()
	for id, key := range current.keyFields() {
		if *key != "" {
			t.Fatalf("clear did not remove %s", id)
		}
	}
}

func TestNewProviderEnvironmentVariables(t *testing.T) {
	for name, value := range map[string]string{"GLM_API_KEY": "glm-test", "MOONSHOT_API_KEY": "kimi-test", "ANTHROPIC_API_KEY": "claude-test", "XAI_API_KEY": "grok-test", "COMPATIBLE_API_KEY": "custom-test", "COMPATIBLE_BASE_URL": "https://api.example/v1"} {
		t.Setenv(name, value)
	}
	s := loadSettings(t.TempDir())
	if s.GLMKey != "glm-test" || s.KimiKey != "kimi-test" || s.ClaudeKey != "claude-test" || s.GrokKey != "grok-test" || s.CompatibleKey != "custom-test" {
		t.Fatal("new environment keys not loaded")
	}
}

func TestCompatibleEndpointValidation(t *testing.T) {
	for base, want := range map[string]string{
		"https://api.example/v1/":                     "https://api.example/v1/chat/completions",
		"https://api.example/custom/chat/completions": "https://api.example/custom/chat/completions",
		"http://127.0.0.1:1234/v1":                    "http://127.0.0.1:1234/v1/chat/completions",
		"http://[::1]:1234/v1":                        "http://[::1]:1234/v1/chat/completions",
	} {
		got, err := compatibleEndpoint(base)
		if err != nil || got != want {
			t.Fatalf("%s: got %s, %v", base, got, err)
		}
	}
	for _, base := range []string{"", "api.example/v1", "http://api.example/v1", "https://user:password@api.example/v1", "https://api.example/v1?key=secret", "https://api.example/v1#secret", "file:///settings.json", "https://api.example/v1?"} {
		if _, err := compatibleEndpoint(base); err == nil {
			t.Errorf("invalid endpoint accepted: %s", base)
		}
	}
}

func TestCustomEndpointChangeDoesNotReusePreviousKey(t *testing.T) {
	a, _ := newApp(t.TempDir())
	a.settings = allProviderSettings()
	w := requestTest(t, a, "POST", "/api/settings", Settings{Provider: "compatible", Model: "test", CompatibleBaseURL: "https://model.example/v1/", CompatibleJSONMode: true})
	if w.Code != 200 || a.config().CompatibleKey == "" || !a.config().CompatibleJSONMode {
		t.Fatal("equivalent endpoint lost key or JSON preference")
	}
	w = requestTest(t, a, "POST", "/api/settings", Settings{Provider: "compatible", Model: "test", CompatibleBaseURL: "https://another.example/v1"})
	if w.Code != 200 || a.config().CompatibleKey != "" {
		t.Fatal("old custom key reused at new destination")
	}
	w = requestTest(t, a, "POST", "/api/settings", Settings{Provider: "compatible", Model: "test", CompatibleBaseURL: "http://remote.example/v1", CompatibleKey: "new-secret"})
	if w.Code != 400 || a.config().CompatibleKey != "" || a.config().CompatibleBaseURL != "https://another.example/v1" {
		t.Fatal("invalid settings partially saved")
	}
}

func TestClaudeTruncationAndRefusalNotTreatedAsSuccess(t *testing.T) {
	for _, stop := range []string{"max_tokens", "refusal", "tool_use", "pause_turn", ""} {
		data, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": `{"ok":true}`}}, "stop_reason": stop})
		if _, ok := llmResponseText("claude", data); ok {
			t.Fatalf("unfinished/invalid stop accepted: %q", stop)
		}
	}
	for _, stop := range []string{"length", "content_filter", "tool_calls"} {
		data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"ok":true}`}, "finish_reason": stop}}})
		if _, ok := llmResponseText("grok", data); ok {
			t.Fatalf("invalid completion accepted: %s", stop)
		}
	}
}

func TestLLMRetryDoesNotKeepPartiallyDecodedFields(t *testing.T) {
	old := llmHTTP
	t.Cleanup(func() { llmHTTP = old })
	calls := 0
	llmHTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		content := `{"method":"regression","confidence":"bad"}`
		if calls == 2 {
			content = "```json\n{\"confidence\":0.95}\n```"
		}
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	s := allProviderSettings()
	s.Provider, s.Model = "glm", "glm-4.7-flash"
	var out ModelSpec
	if err := callLLM(context.Background(), s, "JSON", nil, &out); err != nil || calls != 2 || out.Method != "" || out.Confidence != .95 {
		t.Fatalf("bad retry state: %+v, %v", out, err)
	}
}

func TestModelRedirectNeverForwardsCredential(t *testing.T) {
	calls := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	s := allProviderSettings()
	s.Provider, s.Model, s.CompatibleBaseURL = "compatible", "test", origin.URL
	var out struct {
		OK bool `json:"ok"`
	}
	err := callLLM(context.Background(), s, "JSON", nil, &out)
	if err == nil || calls != 0 || !strings.Contains(err.Error(), "重定向") {
		t.Fatalf("credential redirect permitted: %v, calls=%d", err, calls)
	}
}

func TestNewProviderErrorsAreActionableAndDoNotEchoSecrets(t *testing.T) {
	old := llmHTTP
	t.Cleanup(func() { llmHTTP = old })
	for _, status := range []int{400, 401, 403, 404, 422, 429, 500, 529} {
		llmHTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"secret-claude private-body"}}`)), Header: make(http.Header)}, nil
		})}
		s := allProviderSettings()
		s.Provider, s.Model = "claude", "claude-haiku-4-5-20251001"
		var out struct {
			OK bool `json:"ok"`
		}
		err := callLLM(context.Background(), s, "JSON", nil, &out)
		if err == nil || strings.Contains(err.Error(), "secret-") || strings.Contains(err.Error(), "private-body") {
			t.Fatalf("unsafe error: %d %v", status, err)
		}
	}
}

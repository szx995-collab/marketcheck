package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// The catalog is shared by the settings UI and request validation. Only expose
// distinct, documented levels; a model name alone never enables unknown options.
type EffortOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type LLMModel struct {
	ID            string         `json:"id"`
	Label         string         `json:"label"`
	Efforts       []EffortOption `json:"efforts"`
	DefaultEffort string         `json:"default_effort"`
	Hint          string         `json:"hint"`
	style         string
}

func modelChoice(id, label, style, initial, hint string, levels ...string) LLMModel {
	labels := map[string]string{"auto": "模型默认", "off": "关闭思考", "none": "关闭思考", "on": "开启思考", "low": "低", "medium": "中", "high": "高", "xhigh": "极高", "max": "最高"}
	m := LLMModel{ID: id, Label: label, style: style, DefaultEffort: initial, Hint: hint, Efforts: []EffortOption{{"auto", "模型默认"}}}
	for _, level := range levels {
		m.Efforts = append(m.Efforts, EffortOption{level, labels[level]})
	}
	return m
}

var llmModels = map[string][]LLMModel{
	"deepseek": {
		modelChoice("deepseek-v4-flash", "DeepSeek V4 Flash", "thinking-effort", "off", "思考开启时支持低、高、最高三档。", "off", "low", "high", "max"),
		modelChoice("deepseek-v4-pro", "DeepSeek V4 Pro", "thinking-effort", "off", "思考开启时支持低、高、最高三档。", "off", "low", "high", "max"),
	},
	"openai": {
		modelChoice("gpt-6-astra", "GPT-6 Astra", "openai", "medium", "支持低到最高；极高对应 xhigh。", "low", "medium", "high", "xhigh", "max"),
		modelChoice("gpt-5.6-luna", "GPT-5.6 Luna", "openai", "medium", "可关闭思考，或选择推理强度。", "none", "low", "medium", "high", "xhigh", "max"),
		modelChoice("gpt-4.1-mini", "GPT-4.1 mini", "plain", "auto", "此模型不支持调节推理强度。"),
	},
	"codex": {
		modelChoice("", "自动选择（Codex 默认）", "codex", "auto", "由本机 Codex 决定模型与档位；指定强度请先选择具体模型。"),
		modelChoice("gpt-6-astra", "GPT-6 Astra", "codex", "medium", "极高对应 xhigh；使用 ChatGPT 账号的 Codex 额度。", "low", "medium", "high", "xhigh", "max"),
		modelChoice("gpt-5.6-sol", "GPT-5.6 Sol", "codex", "medium", "账号是否可用，以保存并测试结果为准。", "low", "medium", "high", "xhigh", "max"),
		modelChoice("gpt-5.6-terra", "GPT-5.6 Terra", "codex", "medium", "账号是否可用，以保存并测试结果为准。", "low", "medium", "high", "xhigh", "max"),
		modelChoice("gpt-5.6-luna", "GPT-5.6 Luna", "codex", "medium", "账号是否可用，以保存并测试结果为准。", "low", "medium", "high", "xhigh", "max"),
		modelChoice("gpt-5.5", "GPT-5.5", "codex", "medium", "支持低、中、高、极高。", "low", "medium", "high", "xhigh"),
		modelChoice("gpt-5.4-mini", "GPT-5.4 mini", "codex", "medium", "支持低、中、高、极高。", "low", "medium", "high", "xhigh"),
	},
	"glm": {
		modelChoice("glm-5.2", "GLM-5.2", "thinking-effort", "high", "支持关闭思考、高、最高；服务会合并其他同义档位。", "off", "high", "max"),
		modelChoice("glm-5.1", "GLM-5.1", "toggle", "off", "此模型支持思考开关，不支持细分强度。", "off", "on"),
		modelChoice("glm-4.7", "GLM-4.7", "toggle", "off", "此模型支持思考开关，不支持细分强度。", "off", "on"),
		modelChoice("glm-4.7-flash", "GLM-4.7 Flash", "toggle", "off", "此模型支持思考开关，不支持细分强度。", "off", "on"),
	},
	"kimi": {
		modelChoice("kimi-k3", "Kimi K3", "kimi-effort", "high", "始终推理，可选择低、高、最高。", "low", "high", "max"),
		modelChoice("kimi-k2.6", "Kimi K2.6", "toggle", "off", "此模型支持思考开关，不支持细分强度。", "off", "on"),
		modelChoice("kimi-k2.7-code", "Kimi K2.7 Code", "always", "auto", "此模型始终思考，不支持关闭或细分强度。"),
	},
	"claude": {
		modelChoice("claude-sonnet-5", "Claude Sonnet 5", "adaptive", "medium", "自适应思考；支持低到最高。", "low", "medium", "high", "xhigh", "max"),
		modelChoice("claude-opus-5", "Claude Opus 5", "adaptive", "medium", "自适应思考；支持低到最高。", "low", "medium", "high", "xhigh", "max"),
		modelChoice("claude-sonnet-4-6", "Claude Sonnet 4.6", "adaptive", "medium", "自适应思考；此版本没有独立的极高档。", "low", "medium", "high", "max"),
		modelChoice("claude-opus-4-6", "Claude Opus 4.6", "adaptive", "medium", "自适应思考；此版本没有独立的极高档。", "low", "medium", "high", "max"),
		modelChoice("claude-haiku-4-5-20251001", "Claude Haiku 4.5", "budget", "off", "低 / 中 / 高分别设置 1,024 / 4,096 / 8,192 个思考 token 的预算，不等同于其他 Claude 的原生 effort。", "off", "low", "medium", "high"),
	},
	"grok": {
		modelChoice("grok-4.6", "Grok 4.6", "effort", "high", "始终推理，支持低、中、高、极高。", "low", "medium", "high", "xhigh"),
		modelChoice("grok-4.5", "Grok 4.5", "effort", "high", "始终推理，此版本没有独立的极高档。", "low", "medium", "high"),
	},
}

func findLLMModel(provider, id string) LLMModel {
	for _, m := range llmModels[provider] {
		if m.ID == id {
			return m
		}
	}
	if provider == "compatible" {
		for _, p := range []string{"openai", "deepseek", "glm", "kimi", "grok"} {
			for _, m := range llmModels[p] {
				if m.ID == id {
					return m
				}
			}
		}
	}
	return modelChoice(id, id, "unknown", "auto", "尚无此模型的推理参数说明，使用服务默认值。")
}

func modelCatalog(s Settings) map[string][]LLMModel {
	out := make(map[string][]LLMModel)
	for id, models := range llmModels {
		out[id] = append([]LLMModel(nil), models...)
	}
	out["compatible"] = []LLMModel{}
	if s.Model != "" {
		found := false
		for _, m := range out[s.Provider] {
			if m.ID == s.Model {
				found = true
			}
		}
		if !found {
			out[s.Provider] = append(out[s.Provider], findLLMModel(s.Provider, s.Model))
		}
	}
	return out
}

func effectiveEffort(s Settings) string {
	if s.ReasoningEffort != "" {
		return s.ReasoningEffort
	}
	// Older saved settings did not contain this field. Keep their fast defaults.
	if oneOf(s.Provider, "deepseek", "glm", "kimi", "claude") && oneOf(findLLMModel(s.Provider, s.Model).style, "thinking-effort", "toggle", "budget") {
		return "off"
	}
	return "auto"
}

func validateEffort(s Settings) error {
	effort := effectiveEffort(s)
	for _, e := range findLLMModel(s.Provider, s.Model).Efforts {
		if e.Value == effort {
			return nil
		}
	}
	return errors.New("所选模型不支持这个推理档位，请重新选择模型与推理强度")
}

func reasoningTokens(s Settings) int {
	effort := effectiveEffort(s)
	if oneOf(effort, "off", "none") {
		return 5000
	}
	if oneOf(findLLMModel(s.Provider, s.Model).style, "plain", "unknown", "budget") {
		return 5000
	}
	if oneOf(s.Model, "kimi-k2.6", "kimi-k2.7-code") {
		return 32768
	}
	if effort == "low" {
		return 16384
	}
	if oneOf(effort, "high", "xhigh", "max", "auto") {
		return 65536
	}
	return 32768
}

func modelTimeout(s Settings) time.Duration {
	if reasoningTokens(s) > 5000 || s.Provider == "codex" || oneOf(effectiveEffort(s), "low", "medium", "high", "xhigh", "max", "on") {
		return 10 * time.Minute
	}
	return 110 * time.Second
}

func applyReasoning(payload map[string]any, s Settings) error {
	if err := validateEffort(s); err != nil {
		return err
	}
	effort, style := effectiveEffort(s), findLLMModel(s.Provider, s.Model).style
	if effort == "auto" {
		return nil
	}
	switch style {
	case "thinking-effort", "toggle":
		typeName := "enabled"
		if effort == "off" {
			typeName = "disabled"
		}
		payload["thinking"] = map[string]string{"type": typeName}
		if style == "thinking-effort" && effort != "off" {
			payload["reasoning_effort"] = effort
		}
	case "openai", "effort", "kimi-effort":
		payload["reasoning_effort"] = effort
	case "adaptive":
		payload["thinking"] = map[string]string{"type": "adaptive"}
		payload["output_config"].(map[string]any)["effort"] = effort
	case "budget":
		if effort == "off" {
			payload["thinking"] = map[string]string{"type": "disabled"}
			break
		}
		budget := map[string]int{"low": 1024, "medium": 4096, "high": 8192}[effort]
		payload["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
		payload["max_tokens"] = budget + 5000
	}
	return nil
}

// Custom services supply their own model IDs. Listing never saves settings or
// reuses a saved credential after its destination changes.
func (a *App) listCustomModels(w http.ResponseWriter, r *http.Request) {
	var input struct {
		BaseURL string `json:"base_url"`
		Key     string `json:"key"`
		Clear   bool   `json:"clear_keys"`
	}
	if !decode(w, r, &input) {
		return
	}
	endpoint, err := compatibleEndpoint(input.BaseURL)
	if err != nil {
		fail(w, err)
		return
	}
	saved := a.config()
	key := strings.TrimSpace(input.Key)
	previous, _ := compatibleEndpoint(saved.CompatibleBaseURL)
	if key == "" && !input.Clear && previous == endpoint {
		key = saved.CompatibleKey
	}
	if key == "" {
		fail(w, errors.New("请先填写此接口的 API Key，再读取模型列表"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(endpoint, "/chat/completions")+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	client := *llmHTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		fail(w, errors.New("无法读取模型列表，请检查接口地址和网络"))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fail(w, errors.New("读取模型列表失败，请确认 Key 有效且接口支持 GET /models；不会跟随重定向"))
		return
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err != nil || len(b) > 1<<20 || json.Unmarshal(b, &result) != nil {
		fail(w, errors.New("模型列表格式无效或过大"))
		return
	}
	models, seen := []LLMModel{}, map[string]bool{}
	for _, item := range result.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" || len(id) > 100 || strings.ContainsAny(id, "\r\n\t") || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, findLLMModel("compatible", id))
		if len(models) >= 500 {
			break
		}
	}
	if len(models) == 0 {
		fail(w, errors.New("接口没有返回可选模型"))
		return
	}
	respond(w, models)
}

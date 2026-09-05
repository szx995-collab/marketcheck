package main

import (
	"errors"
	"net/url"
	"strings"
)

type ModelProvider struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	DefaultModel string `json:"default_model"`
	Hint         string `json:"hint"`
	Endpoint     string `json:"-"`
}

var modelProviders = []ModelProvider{
	{"deepseek", "DeepSeek", "deepseek-v4-flash", "使用 DeepSeek 开放平台 API Key。", "https://api.deepseek.com/chat/completions"},
	{"openai", "OpenAI GPT（API 计费）", "gpt-4.1-mini", "使用 OpenAI API Key；ChatGPT 套餐登录请选择 Codex。", "https://api.openai.com/v1/chat/completions"},
	{"codex", "Codex（ChatGPT 登录）", "", "复用本机 ChatGPT 登录。", ""},
	{"glm", "GLM（智谱）", "glm-4.7-flash", "使用智谱国内开放平台 API Key（open.bigmodel.cn）。Z.ai 或其他兼容地址可选择自定义接口。", "https://open.bigmodel.cn/api/paas/v4/chat/completions"},
	{"kimi", "Kimi（月之暗面）", "kimi-k2.6", "使用 Moonshot 国内 API Key（api.moonshot.cn）。其他地区的 Key 请配合对应的自定义接口地址。", "https://api.moonshot.cn/v1/chat/completions"},
	{"claude", "Claude（Anthropic）", "claude-haiku-4-5-20251001", "使用 Anthropic 官方 API Key，接入 Messages API；模型需支持结构化输出。", "https://api.anthropic.com/v1/messages"},
	{"grok", "Grok（xAI）", "grok-4.6", "使用 xAI 开放平台 API Key；模型名称以账号实际可用列表为准。", "https://api.x.ai/v1/chat/completions"},
	{"compatible", "自定义 OpenAI 兼容接口", "", "填写服务的 Base URL、模型 ID 和对应 Key。仅支持 Chat Completions 兼容协议，Key 会发送到你填写的地址。", ""},
}

func providerByID(id string) (ModelProvider, bool) {
	for _, p := range modelProviders {
		if p.ID == id {
			return p, true
		}
	}
	return ModelProvider{}, false
}

// Accept the documented base URL or full chat endpoint. Keep credentials and
// query parameters out of URLs; auth belongs in the request header.
func compatibleEndpoint(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.ForceQuery {
		return "", errors.New("请填写有效的 Base URL，不要在地址中包含 Key、查询参数或片段")
	}
	local := oneOf(u.Hostname(), "localhost", "127.0.0.1", "::1")
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", errors.New("远程接口请使用 HTTPS；仅本机 localhost / 127.0.0.1 / ::1 支持 HTTP")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	if !strings.HasSuffix(u.Path, "/chat/completions") {
		u.Path += "/chat/completions"
	}
	return u.String(), nil
}

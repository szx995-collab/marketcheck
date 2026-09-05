package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testHypothesis() Hypothesis {
	return Hypothesis{Kind: "event", Target: SeriesSpec{"tencent", "sh000300", "沪深300", "value"}, Signal: SeriesSpec{"tencent", "sh000300", "沪深300", "value"}, Start: "2020-01-01", End: "2024-12-31", Frequency: "daily", XTransform: "return", YTransform: "return", Lookback: 1, Horizon: 1, Lag: 0, Operator: "le", Threshold: -1, Direction: "positive"}
}

func requestTest(t *testing.T, a *App, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, "http://127.0.0.1:8765"+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, req)
	return w
}

func TestHypothesisValidation(t *testing.T) {
	h := testHypothesis()
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	h.Signal.Symbol = "sh000300; whoami"
	if h.Validate() == nil {
		t.Fatal("invalid symbol accepted")
	}
	h = testHypothesis()
	h.End = h.Start
	if h.Validate() == nil {
		t.Fatal("empty date range accepted")
	}
	h = testHypothesis()
	h.Controls = []SeriesSpec{h.Target}
	if h.Validate() == nil {
		t.Fatal("event control silently ignored")
	}
	for _, band := range []float64{-1, math.NaN(), math.Inf(1)} {
		h = testHypothesis()
		h.FlatBand = band
		if h.Validate() == nil {
			t.Fatal("invalid flat band accepted")
		}
	}
	h = testHypothesis()
	h.FlatBand = .1
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.Summary(), "其余为平稳") {
		t.Fatal("comparison definition not confirmed")
	}
}

func TestConfirmationAndSecretBoundaries(t *testing.T) {
	a, err := newApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a.settings.DeepseekKey = "private-key-do-not-return"
	w := requestTest(t, a, "GET", "/api/bootstrap", nil)
	if strings.Contains(w.Body.String(), "private-key-do-not-return") {
		t.Fatal("key leaked")
	}
	w = requestTest(t, a, "POST", "/api/runs", map[string]any{"hypothesis": testHypothesis(), "confirmed": false})
	if w.Code != 400 {
		t.Fatal("unconfirmed hypothesis accepted")
	}
	w = requestTest(t, a, "POST", "/api/runs", map[string]any{"hypothesis": testHypothesis(), "confirmed": true})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var run Run
	if err := json.Unmarshal(w.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	a.runs[run.ID].Data = json.RawMessage(`{"series":{}}`)
	w = requestTest(t, a, "POST", "/api/runs/"+run.ID+"/analyze", map[string]any{"model": ModelSpec{Method: "event", Confidence: .95, HACLags: 5}, "confirmed": false})
	if w.Code != 400 {
		t.Fatal("unconfirmed model accepted")
	}
	if a.runs[run.ID].Confirmed {
		t.Fatal("model marked confirmed")
	}
	if !a.beginJob(run.ID, "data_running", "fetch", true) {
		t.Fatal("could not start job")
	}
	if a.beginJob(run.ID, "analysis_running", "analyze", false) {
		t.Fatal("concurrent job accepted")
	}
}

func TestOriginAndJSONProtection(t *testing.T) {
	a, _ := newApp(t.TempDir())
	for _, origin := range []string{"https://evil.example", "null", "http://localhost:9999"} {
		req := httptest.NewRequest("POST", "http://127.0.0.1:8765/api/settings", strings.NewReader(`{}`))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatalf("origin %s accepted", origin)
		}
	}
	req := httptest.NewRequest("POST", "http://127.0.0.1:8765/api/settings", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal("simple cross-site request accepted")
	}
}

func TestSettingsRememberIsExplicit(t *testing.T) {
	a, _ := newApp(t.TempDir())
	s := Settings{Provider: "deepseek", Model: "deepseek-v4-flash", DeepseekKey: "test-key", Remember: true}
	w := requestTest(t, a, "POST", "/api/settings", s)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(a.local, "settings.json")); err != nil {
		t.Fatal(err)
	}
	s.DeepseekKey = ""
	s.Remember = false
	w = requestTest(t, a, "POST", "/api/settings", s)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if a.config().DeepseekKey != "test-key" {
		t.Fatal("blank key should preserve memory setting")
	}
	if _, err := os.Stat(filepath.Join(a.local, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("key persisted without consent")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLLMRequestAndRetry(t *testing.T) {
	old := llmHTTP
	defer func() { llmHTTP = old }()
	calls := 0
	llmHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer local-test-key" {
			t.Error("missing auth")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "deepseek-v4-flash" {
			t.Error("model changed")
		}
		content := `{"choices":[{"message":{"content":"{bad"},"finish_reason":"stop"}]}`
		if calls == 2 {
			content = `{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(content)), Header: make(http.Header)}, nil
	})}
	var out map[string]any
	err := callLLM(context.Background(), Settings{Provider: "deepseek", Model: "deepseek-v4-flash", DeepseekKey: "local-test-key"}, "JSON", nil, &out)
	if err != nil || out["ok"] != true || calls != 2 {
		t.Fatalf("retry failed: %v %+v calls=%d", err, out, calls)
	}
}

func TestModelConstraints(t *testing.T) {
	h := testHypothesis()
	m := ModelSpec{Method: "event", Confidence: .95, HACLags: 5}
	if err := m.Validate(h); err != nil {
		t.Fatal(err)
	}
	m.Method = "pearson"
	if m.Validate(h) == nil {
		t.Fatal("event changed to unrelated method")
	}
	m.Method = "event"
	m.Confidence = .904
	if m.Validate(h) == nil {
		t.Fatal("unsupported confidence accepted")
	}
	m.Confidence = .95
	h.Horizon = 10
	if m.Validate(h) == nil {
		t.Fatal("insufficient dependence horizon accepted")
	}
}

func TestExportsEscapeUserTextAndContainData(t *testing.T) {
	a, _ := newApp(t.TempDir())
	a.runs["export-test"] = &Run{ID: "export-test", Summary: `<script>alert(1)</script>`, Result: json.RawMessage(`{"verdict":"ok","explanation":"test","n":40,"rows":[{"date":"2020-01-01","x":1,"y":2}],"series":[],"sources":[]}`)}
	w := requestTest(t, a, "GET", "/api/runs/export-test/export/report", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("report not downloadable")
	}
	if strings.Contains(w.Body.String(), "<script>") {
		t.Fatal("unsafe HTML in export")
	}
	w = requestTest(t, a, "GET", "/api/runs/export-test/export/analysis.csv", nil)
	if !strings.Contains(w.Body.String(), "2020-01-01,1,2") {
		t.Fatal(w.Body.String())
	}
}

func TestReportIncludesComparisonAndStatisticsHelp(t *testing.T) {
	a, _ := newApp(t.TempDir())
	a.runs["comparison"] = &Run{ID: "comparison", Summary: "oil", Result: json.RawMessage(`{"verdict":"证据不足","y_unit":"%","takeaways":["只是相对更弱<script>"],"comparison":{"available":true,"note":"Bonferroni 校正","groups":[{"label":"平稳","rule":"-0.1 ≤ X ≤ 0.1","count":0,"mean":null,"negative_rate":null}],"pairs":[{"label":"上涨 − 平稳","effect":null,"ci":null,"p_adjusted":null,"status":"样本不足"}]}}`)}
	w := requestTest(t, a, "GET", "/api/runs/comparison/export/report", nil)
	for _, expected := range []string{"相对更弱&lt;script&gt;", "上涨 − 平稳", "样本不足", "Bonferroni 校正", "Pearson", "不是当前假设有 95% 概率为真", "检验 ID：comparison", "</body></html>"} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Fatalf("report missing %q", expected)
		}
	}
	if strings.Contains(w.Body.String(), "<script>") || strings.Contains(w.Body.String(), "<no value>") {
		t.Fatal("unsafe or incomplete report")
	}
	w = requestTest(t, a, "GET", "/api/bootstrap", nil)
	if !strings.Contains(w.Body.String(), "\"glossary\"") || !strings.Contains(w.Body.String(), "HAC") {
		t.Fatal("statistics help unavailable")
	}
}

func TestInterpretationKeepsComputedFactsAndIgnoresModelClaims(t *testing.T) {
	old := llmHTTP
	defer func() { llmHTTP = old }()
	llmHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"term_ids\":[\"ci\",\"comparison\"],\"text\":\"下跌组平均收益为正 99%\"}"},"finish_reason":"stop"}]}`)), Header: make(http.Header)}, nil
	})}
	a, _ := newApp(t.TempDir())
	a.settings.DeepseekKey = "local-test-key"
	run := &Run{ID: "grounded", Status: "complete", Result: json.RawMessage(`{"verdict":"证据不足","explanation":"均值差区间包含零","takeaways":["下跌组平均收益 -0.0088%"],"warnings":["FRED 当前历史版本"],"comparison":{"available":true,"pairs":[{"label":"上涨 − 下跌","status":"证据不足"}]}}`)}
	a.runs[run.ID] = run
	if err := a.saveLocked(run); err != nil {
		t.Fatal(err)
	}
	a, _ = newApp(a.root)
	a.settings.DeepseekKey = "local-test-key"
	run = a.runs["grounded"]
	w := requestTest(t, a, "POST", "/api/runs/grounded/interpret", map[string]any{})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, expected := range []string{"-0.0088%", "均值差区间包含零", "上涨 − 下跌", "FRED 当前历史版本", "置信水平与置信区间"} {
		if !strings.Contains(run.Narrative, expected) {
			t.Fatalf("missing computed fact %q", expected)
		}
	}
	if strings.Contains(run.Narrative, "99%") || run.NarrativeVersion != 2 {
		t.Fatal("model claim accepted or result not versioned")
	}
	if _, err := groundedInterpretation(run, []string{"unknown"}); err == nil {
		t.Fatal("unknown explanation accepted")
	}
	reloaded, _ := newApp(a.root)
	if reloaded.runs["grounded"].NarrativeVersion != 2 || reloaded.runs["grounded"].Narrative != run.Narrative {
		t.Fatal("interpretation not persisted after restart")
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

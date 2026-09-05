package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var webFiles embed.FS

type App struct {
	mu          sync.Mutex
	root, local string
	settings    Settings
	runs        map[string]*Run
}

func newApp(root string) (*App, error) {
	local := filepath.Join(root, ".local")
	if err := os.MkdirAll(filepath.Join(local, "runs"), 0700); err != nil {
		return nil, err
	}
	a := &App{root: root, local: local, settings: loadSettings(local), runs: map[string]*Run{}}
	paths, _ := filepath.Glob(filepath.Join(local, "runs", "*.json"))
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var r Run
		if json.Unmarshal(b, &r) != nil || r.ID == "" {
			continue
		}
		if strings.HasSuffix(r.Status, "running") {
			r.Status = "failed"
			r.Message = "上次运行被中断，可以重试"
		}
		a.runs[r.ID] = &r
	}
	return a, nil
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "analysis", "engine.py")); err != nil {
		log.Fatal("请在 marketcheck 项目目录中运行")
	}
	a, err := newApp(root)
	if err != nil {
		log.Fatal(err)
	}
	port := os.Getenv("MARKETCHECK_PORT")
	if port == "" {
		port = "8765"
	}
	srv := &http.Server{Addr: net.JoinHostPort("127.0.0.1", port), Handler: a.handler(), ReadHeaderTimeout: 10 * time.Second}
	fmt.Printf("MarketCheck 已启动：http://127.0.0.1:%s\n按 Ctrl+C 停止。密钥可在页面设置中填写。\n", port)
	log.Fatal(srv.ListenAndServe())
}

func (a *App) handler() http.Handler {
	assets, _ := fs.Sub(webFiles, "web")
	static := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "Local access only", 403)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'")
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || u.Scheme != "http" {
				http.Error(w, "Origin rejected", 403)
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			if r.Method != "GET" && r.Method != "POST" {
				http.Error(w, "Method not allowed", 405)
				return
			}
			if r.Method == "POST" && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				fail(w, errors.New("请使用 JSON 请求"))
				return
			}
			a.api(w, r)
			return
		}
		static.ServeHTTP(w, r)
	})
}

func respond(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(400)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	d := json.NewDecoder(r.Body)
	if err := d.Decode(v); err != nil {
		fail(w, errors.New("输入格式无效或超过 8MB"))
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		fail(w, errors.New("只能提交一个 JSON 对象"))
		return false
	}
	return true
}
func (a *App) config() Settings { a.mu.Lock(); defer a.mu.Unlock(); return a.settings }
func (a *App) snapshot(id string) *Run {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r := a.runs[id]; r != nil {
		b, _ := json.Marshal(r)
		var copy Run
		_ = json.Unmarshal(b, &copy)
		// Preserve raw formatting for the result identity check, including runs loaded from disk.
		copy.Result = append(json.RawMessage(nil), r.Result...)
		return &copy
	}
	return nil
}
func (a *App) saveLocked(r *Run) error {
	return writeJSON(filepath.Join(a.local, "runs", r.ID+".json"), r)
}
func (a *App) history() []map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	rs := make([]*Run, 0, len(a.runs))
	for _, r := range a.runs {
		rs = append(rs, r)
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].Created > rs[j].Created })
	out := []map[string]any{}
	for i, r := range rs {
		if i >= 50 {
			break
		}
		out = append(out, map[string]any{"id": r.ID, "created": r.Created, "status": r.Status, "summary": r.Summary})
	}
	return out
}

func (a *App) api(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	if r.Method == "GET" {
		switch path {
		case "bootstrap":
			s := a.config()
			respond(w, map[string]any{"settings": s.Public(), "providers": modelProviders, "models": modelCatalog(s), "catalog": catalog, "history": a.history(), "glossary": statisticsTerms})
			return
		case "history":
			respond(w, a.history())
			return
		case "codex/status":
			respond(w, readCodexStatus(r.Context()))
			return
		case "health":
			data, err := a.worker(r.Context(), map[string]string{"op": "health"}, "")
			if err != nil {
				fail(w, err)
			} else {
				respond(w, data)
			}
			return
		}
		if strings.HasPrefix(path, "runs/") {
			parts := strings.Split(path, "/")
			if len(parts) == 4 && parts[2] == "export" {
				a.exportRun(w, r, parts[1], parts[3])
				return
			}
			id := strings.TrimPrefix(path, "runs/")
			run := a.snapshot(id)
			if run == nil {
				http.NotFound(w, r)
			} else {
				respond(w, run)
			}
			return
		}
		http.NotFound(w, r)
		return
	}
	switch path {
	case "models":
		a.listCustomModels(w, r)
	case "settings":
		var input struct {
			Settings
			Clear bool `json:"clear_keys"`
		}
		if !decode(w, r, &input) {
			return
		}
		input.Model = strings.TrimSpace(input.Model)
		_, validProvider := providerByID(input.Provider)
		if !validProvider || (input.Model == "" && input.Provider != "codex") || len(input.Model) > 100 {
			fail(w, errors.New("请选择服务和模型"))
			return
		}
		if err := validateEffort(input.Settings); err != nil {
			fail(w, err)
			return
		}
		a.mu.Lock()
		s := a.settings
		if input.CompatibleBaseURL != "" || input.Provider == "compatible" {
			endpoint, err := compatibleEndpoint(input.CompatibleBaseURL)
			if err != nil {
				a.mu.Unlock()
				fail(w, err)
				return
			}
			oldEndpoint, _ := compatibleEndpoint(s.CompatibleBaseURL)
			if endpoint != oldEndpoint {
				s.CompatibleKey = ""
			}
			s.CompatibleBaseURL = strings.TrimSpace(input.CompatibleBaseURL)
			s.CompatibleJSONMode = input.CompatibleJSONMode
		}
		for id, key := range s.keyFields() {
			if input.Clear {
				*key = ""
			}
			if value := strings.TrimSpace(*input.Settings.keyFields()[id]); value != "" {
				*key = value
			}
		}
		s.Provider = input.Provider
		s.Model = strings.TrimSpace(input.Model)
		s.ReasoningEffort = effectiveEffort(input.Settings)
		s.Remember = input.Remember
		var err error
		if s.Remember {
			err = writeJSON(filepath.Join(a.local, "settings.json"), s)
		} else {
			err = os.Remove(filepath.Join(a.local, "settings.json"))
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err == nil {
			a.settings = s
		}
		a.mu.Unlock()
		if err != nil {
			fail(w, errors.New("保存设置失败，请检查 .local 目录权限"))
			return
		}
		respond(w, s.Public())
	case "test-model":
		var out struct {
			OK bool `json:"ok"`
		}
		err := callLLM(r.Context(), a.config(), `仅返回 {"ok":true}`, "连接测试", &out)
		if err != nil {
			fail(w, err)
			return
		}
		if !out.OK {
			fail(w, errors.New("模型未正确完成连接测试，请重试"))
			return
		}
		respond(w, map[string]bool{"ok": true})
	case "clarify":
		var input struct {
			Original string            `json:"original"`
			Answers  map[string]string `json:"answers"`
		}
		if !decode(w, r, &input) {
			return
		}
		if len(strings.TrimSpace(input.Original)) < 3 || len(input.Original) > 6000 || len(input.Answers) > 60 {
			fail(w, errors.New("请输入简短的市场假设"))
			return
		}
		var out Clarification
		err := callLLM(r.Context(), a.config(), clarifyPrompt, map[string]any{"original": input.Original, "answers": input.Answers, "today": time.Now().UTC().Format("2006-01-02"), "catalog": catalog}, &out)
		if err != nil {
			fail(w, err)
			return
		}
		if len(out.Questions) > 6 || out.Draft.Kind == "" {
			fail(w, errors.New("模型未返回可编辑草稿，请重试或手动填写"))
			return
		}
		out.Draft.Original = input.Original
		respond(w, out)
	case "runs":
		var input struct {
			Hypothesis Hypothesis `json:"hypothesis"`
			Confirmed  bool       `json:"confirmed"`
		}
		if !decode(w, r, &input) {
			return
		}
		if !input.Confirmed {
			fail(w, errors.New("请先确认假设"))
			return
		}
		if err := input.Hypothesis.Validate(); err != nil {
			fail(w, err)
			return
		}
		run := &Run{ID: fmt.Sprintf("%d", time.Now().UnixNano()), Created: time.Now().UTC().Format(time.RFC3339Nano), Status: "confirmed", Message: "假设已确认，请获取数据", Hypothesis: input.Hypothesis, Summary: input.Hypothesis.Summary()}
		a.mu.Lock()
		err := a.saveLocked(run)
		if err == nil {
			a.runs[run.ID] = run
		}
		a.mu.Unlock()
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, run)
	default:
		parts := strings.Split(path, "/")
		if len(parts) != 3 || parts[0] != "runs" {
			http.NotFound(w, r)
			return
		}
		a.runAction(w, r, parts[1], parts[2])
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func mockCodex(t *testing.T, run func(context.Context, string, string, []string, string) ([]byte, []byte, error)) {
	t.Helper()
	oldFind, oldRun := findCodex, runCodex
	findCodex = func() (string, error) { return "codex-test", nil }
	runCodex = run
	t.Cleanup(func() { findCodex, runCodex = oldFind, oldRun })
}

func TestCodexRejectsAPIKeyLoginWithoutFallback(t *testing.T) {
	calls := 0
	mockCodex(t, func(_ context.Context, _, _ string, args []string, _ string) ([]byte, []byte, error) {
		calls++
		if strings.Join(args, " ") != "login status" {
			t.Fatal("model invoked under API auth")
		}
		return nil, []byte("Logged in using an API key - sk-private-test-value"), nil
	})
	s := Settings{Provider: "codex", DeepseekKey: "deepseek-secret", OpenAIKey: "openai-secret"}
	if s.Key() != "" {
		t.Fatal("Codex exposed an API credential")
	}
	var out struct {
		OK bool `json:"ok"`
	}
	err := callLLM(context.Background(), s, "JSON", nil, &out)
	if err == nil || calls != 1 || strings.Contains(err.Error(), "sk-private") || !strings.Contains(err.Error(), "ChatGPT") {
		t.Fatalf("wrong auth behavior: %v", err)
	}
}

func TestCodexStatusMissingCLIAndLoggedOut(t *testing.T) {
	mockCodex(t, func(context.Context, string, string, []string, string) ([]byte, []byte, error) {
		return nil, []byte("Not logged in"), errors.New("exit 1")
	})
	s := readCodexStatus(context.Background())
	if !s.Installed || s.LoggedIn {
		t.Fatalf("wrong status: %+v", s)
	}
	findCodex = func() (string, error) { return "", exec.ErrNotFound }
	s = readCodexStatus(context.Background())
	if s.Installed || s.LoggedIn || !strings.Contains(s.Message, "未找到") {
		t.Fatalf("wrong missing status: %+v", s)
	}
}

func TestCodexInvocationAndTypedJSON(t *testing.T) {
	var workdir string
	mockCodex(t, func(_ context.Context, _, dir string, args []string, input string) ([]byte, []byte, error) {
		if args[0] == "login" {
			return nil, []byte("Logged in using ChatGPT"), nil
		}
		workdir = dir
		joined := strings.Join(args, " ")
		for _, required := range []string{"--ignore-user-config", "--ignore-rules", "--ephemeral", "--sandbox read-only", `forced_login_method="chatgpt"`, "features.shell_tool=false", "features.plugins=false", "features.hooks=false", "--model model-test"} {
			if !strings.Contains(joined, required) {
				t.Errorf("missing constraint %s", required)
			}
		}
		if input != `{"original":"油价上涨时股票如何变化？"}` {
			t.Errorf("wrong input: %s", input)
		}
		schema, err := os.ReadFile(filepath.Join(dir, "schema.json"))
		if err != nil || !strings.Contains(string(schema), `"additionalProperties": false`) {
			t.Fatal("missing strict schema")
		}
		return []byte("{\"type\":\"thread.started\"}\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"{\\\"ok\\\":true}\"}}\n{\"type\":\"turn.completed\"}\n"), nil, nil
	})
	var out struct {
		OK bool `json:"ok"`
	}
	if err := callLLM(context.Background(), Settings{Provider: "codex", Model: "model-test"}, "JSON", map[string]string{"original": "油价上涨时股票如何变化？"}, &out); err != nil || !out.OK {
		t.Fatalf("call failed: %v", err)
	}
	if _, err := os.Stat(workdir); !os.IsNotExist(err) {
		t.Fatal("temporary task files retained")
	}
}

func TestCodexResultRejectsPartialAndFailedTurns(t *testing.T) {
	for _, data := range []string{
		`{"type":"item.completed","item":{"type":"agent_message","text":"{\"ok\":true}"}}`,
		"not json",
		`{"type":"turn.completed"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"not JSON"}}` + "\n" + `{"type":"turn.completed"}`,
		`{"type":"turn.failed","error":{"message":"usage_limit_reached private-token-value"}}`,
	} {
		var out struct {
			OK bool `json:"ok"`
		}
		err := decodeCodexResult([]byte(data), &out)
		if err == nil || strings.Contains(err.Error(), "private-token-value") {
			t.Fatalf("unsafe result: %v", err)
		}
	}
	if !strings.Contains(codexFailure("usage_limit_reached").Error(), "额度") {
		t.Fatal("limit error not actionable")
	}
}

func TestCodexEnvironmentAndProcessCancellation(t *testing.T) {
	env := codexEnvironment([]string{"PATH=test-path", "CODEX_HOME=test-home", "OPENAI_API_KEY=secret", "CODEX_API_KEY=secret", "FRED_API_KEY=secret", "DEEPSEEK_API_KEY=secret", "CODEX_THREAD_ID=parent", "GITHUB_TOKEN=secret"})
	if !reflect.DeepEqual(env, []string{"PATH=test-path", "CODEX_HOME=test-home"}) {
		t.Fatalf("unexpected child environment: %v", env)
	}
	exe, _ := os.Executable()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := executeCodex(ctx, exe, t.TempDir(), []string{"-test.run=^$"}, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
	var out limitedOutput
	_, _ = out.Write(make([]byte, 600*1024))
	if !out.overflow || out.Len() != 512*1024 {
		t.Fatal("output unbounded")
	}
}

func TestCodexSchemaCoversClarificationAndSettingsKeepKeys(t *testing.T) {
	schema, err := codexSchema(reflect.TypeOf(Clarification{}))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(schema)
	for _, field := range []string{"questions", "draft", "flat_band", "controls", "options"} {
		if !strings.Contains(string(b), `"`+field+`"`) {
			t.Fatalf("missing %s", field)
		}
	}
	a, _ := newApp(t.TempDir())
	a.settings.DeepseekKey, a.settings.FredKey = "preserve-deepseek", "preserve-fred"
	w := requestTest(t, a, "POST", "/api/settings", Settings{Provider: "codex", Remember: true})
	if w.Code != 200 || a.config().Model != "" || a.config().FredKey != "preserve-fred" || a.config().DeepseekKey != "preserve-deepseek" {
		t.Fatalf("bad settings switch: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "preserve-") {
		t.Fatal("settings exposed secrets")
	}
	a, _ = newApp(a.root)
	if a.config().Provider != "codex" || a.config().Model != "" {
		t.Fatal("Codex choice lost after restart")
	}
}

// Opt-in only: consumes the local ChatGPT account's Codex allowance.
func TestCodexLiveConnection(t *testing.T) {
	if os.Getenv("MARKETCHECK_TEST_CODEX") != "1" {
		t.Skip("set MARKETCHECK_TEST_CODEX=1 for a real subscription call")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()
	var out struct {
		OK bool `json:"ok"`
	}
	if err := callCodex(ctx, Settings{Model: os.Getenv("MARKETCHECK_TEST_CODEX_MODEL")}, `仅返回 {"ok":true}`, "连接测试", &out); err != nil || !out.OK {
		t.Fatalf("real Codex call: %v", err)
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func findPython(root string) string {
	if p := os.Getenv("MARKETCHECK_PYTHON"); p != "" {
		return p
	}
	for _, p := range []string{filepath.Join(root, ".venv", "Scripts", "python.exe"), filepath.Join(root, ".venv", "bin", "python")} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, ".tools", "python-path.txt")); err == nil {
		return strings.TrimSpace(string(b))
	}
	if p, err := exec.LookPath("python3"); err == nil && !strings.Contains(p, "WindowsApps") {
		return p
	}
	if p, err := exec.LookPath("python"); err == nil && !strings.Contains(p, "WindowsApps") {
		return p
	}
	return "python"
}

func (a *App) worker(ctx context.Context, input any, key string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, findPython(a.root), filepath.Join(a.root, "analysis", "engine.py"))
	cmd.Dir = a.root
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1", "FRED_API_KEY="+key)
	cmd.Stdin = bytes.NewReader(body)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("处理超时，可缩短日期范围后重试")
		}
		// Python import/launch failures are actionable; provider responses are never logged here.
		if strings.Contains(stderr.String(), "ModuleNotFoundError") {
			return nil, errors.New("Python 分析依赖未安装，请先运行 setup.ps1")
		}
		return nil, errors.New("分析进程启动失败，请检查 Python 环境或运行 setup.ps1")
	}
	var response struct {
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
		Data  json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(out.Bytes(), &response); err != nil {
		return nil, errors.New("分析进程返回格式异常")
	}
	if !response.OK {
		return nil, errors.New(cleanError(errors.New(response.Error), key))
	}
	return response.Data, nil
}

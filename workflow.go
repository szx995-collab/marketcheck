package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (a *App) runAction(w http.ResponseWriter, r *http.Request, id, action string) {
	run := a.snapshot(id)
	if run == nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(run.Status, "running") {
		fail(w, errors.New("本次任务正在运行，请等待完成"))
		return
	}
	s := a.config()
	switch action {
	case "data":
		var input struct {
			Uploads map[string]string `json:"uploads"`
			Demo    bool              `json:"demo"`
		}
		if !decode(w, r, &input) {
			return
		}
		if !a.beginJob(id, "data_running", "正在获取数据、检查覆盖范围…", true) {
			fail(w, errors.New("任务正在运行"))
			return
		}
		go func() {
			data, err := a.worker(context.Background(), map[string]any{"op": "fetch", "hypothesis": run.Hypothesis, "uploads": input.Uploads, "demo": input.Demo}, s.FredKey)
			a.mu.Lock()
			defer a.mu.Unlock()
			current := a.runs[id]
			if err != nil {
				current.Status = "failed"
				current.Message = err.Error()
			} else {
				current.Data = data
				current.Status = "data_ready"
				current.Message = "数据已就绪。检查数据摘要，然后选择并确认模型。"
			}
			if err := a.saveLocked(current); err != nil {
				current.Message += "（本地保存失败，当前结果仍在内存中）"
			}
		}()
		respond(w, map[string]bool{"started": true})
	case "recommend":
		if len(run.Data) == 0 {
			fail(w, errors.New("请先获取数据"))
			return
		}
		var dataset struct {
			Summary  any  `json:"summary"`
			Warnings any  `json:"warnings"`
			Demo     bool `json:"demo"`
		}
		_ = json.Unmarshal(run.Data, &dataset)
		var model ModelSpec
		err := callLLM(r.Context(), s, modelPrompt, map[string]any{"hypothesis": run.Hypothesis, "data": dataset}, &model)
		if err != nil {
			fail(w, err)
			return
		}
		if err = model.Validate(run.Hypothesis); err != nil {
			fail(w, err)
			return
		}
		a.mu.Lock()
		current := a.runs[id]
		if !strings.HasSuffix(current.Status, "running") {
			current.Recommendation = &model
			_ = a.saveLocked(current)
		}
		a.mu.Unlock()
		respond(w, model)
	case "analyze":
		var input struct {
			Model     ModelSpec `json:"model"`
			Confirmed bool      `json:"confirmed"`
		}
		if !decode(w, r, &input) {
			return
		}
		if !input.Confirmed {
			fail(w, errors.New("必须先确认模型与参数"))
			return
		}
		if len(run.Data) == 0 {
			fail(w, errors.New("请先获取数据"))
			return
		}
		if err := input.Model.Validate(run.Hypothesis); err != nil {
			fail(w, err)
			return
		}
		if !a.beginJob(id, "analysis_running", "内置分析工具正在计算…", false) {
			fail(w, errors.New("任务正在运行"))
			return
		}
		a.mu.Lock()
		current := a.runs[id]
		current.Model = &input.Model
		current.Confirmed = true
		_ = a.saveLocked(current)
		a.mu.Unlock()
		go func() {
			result, err := a.worker(context.Background(), map[string]any{"op": "analyze", "hypothesis": run.Hypothesis, "model": input.Model, "dataset": run.Data}, "")
			a.mu.Lock()
			defer a.mu.Unlock()
			current := a.runs[id]
			if err != nil {
				current.Status = "failed"
				current.Message = err.Error()
			} else {
				current.Result = result
				current.Status = "complete"
				current.Message = "计算完成，可查看结果或让 AI 解读。"
			}
			if err := a.saveLocked(current); err != nil {
				current.Message += "（本地保存失败，当前结果仍在内存中）"
			}
		}()
		respond(w, map[string]bool{"started": true})
	case "interpret":
		if len(run.Result) == 0 {
			fail(w, errors.New("请先完成检验"))
			return
		}
		var result map[string]any
		_ = json.Unmarshal(run.Result, &result)
		delete(result, "series")
		delete(result, "points")
		delete(result, "rows")
		var out struct {
			TermIDs []string `json:"term_ids"`
		}
		err := callLLM(r.Context(), s, interpretPrompt, map[string]any{"hypothesis": run.Summary, "model": run.Model, "result": result, "glossary": statisticsTerms}, &out)
		if err != nil {
			fail(w, err)
			return
		}
		text, err := groundedInterpretation(run, out.TermIDs)
		if err != nil {
			fail(w, err)
			return
		}
		a.mu.Lock()
		current := a.runs[id]
		if current.Status == "complete" && string(current.Result) == string(run.Result) {
			current.Narrative = text
			current.NarrativeVersion = 2
			_ = a.saveLocked(current)
		}
		a.mu.Unlock()
		respond(w, map[string]string{"text": text})
	default:
		http.NotFound(w, r)
	}
}

func (a *App) beginJob(id, status, message string, clearData bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.runs[id]
	if strings.HasSuffix(r.Status, "running") {
		return false
	}
	r.Status = status
	r.Message = message
	r.Result = nil
	r.Narrative = ""
	r.NarrativeVersion = 0
	r.Confirmed = false
	r.Model = nil
	if clearData {
		r.Data = nil
		r.Recommendation = nil
	}
	_ = a.saveLocked(r)
	return true
}

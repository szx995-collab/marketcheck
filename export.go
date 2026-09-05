package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strconv"
	"strings"
)

type Point struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}
type ReportSeries struct {
	Label  string  `json:"label"`
	Points []Point `json:"points"`
}
type ReportSource struct {
	Label      string `json:"label"`
	Provider   string `json:"provider"`
	Units      string `json:"units"`
	Adjustment string `json:"adjustment"`
	Start      string `json:"start"`
	End        string `json:"end"`
}
type ReportResult struct {
	Verdict     string           `json:"verdict"`
	Explanation string           `json:"explanation"`
	ModelDetail string           `json:"model_detail"`
	N           int              `json:"n"`
	Start       string           `json:"start"`
	End         string           `json:"end"`
	Demo        bool             `json:"demo"`
	Series      []ReportSeries   `json:"series"`
	Sources     []ReportSource   `json:"sources"`
	Warnings    []string         `json:"warnings"`
	Rows        []map[string]any `json:"rows"`
	Takeaways   []string         `json:"takeaways"`
	YUnit       string           `json:"y_unit"`
	Group       map[string]any   `json:"group"`
	Comparison  map[string]any   `json:"comparison"`
}

func reportNumber(value any) string {
	if value == nil {
		return "—"
	}
	if number, ok := value.(float64); ok {
		return strconv.FormatFloat(number, 'g', 4, 64)
	}
	return fmt.Sprint(value)
}

// Only numeric coordinates are interpolated into this trusted SVG. Labels are escaped by html/template.
func reportLine(points []Point) template.HTML {
	if len(points) < 2 {
		return ""
	}
	low, high := points[0].Value, points[0].Value
	for _, p := range points {
		if p.Value < low {
			low = p.Value
		}
		if p.Value > high {
			high = p.Value
		}
	}
	if high == low {
		high = low + 1
	}
	var b strings.Builder
	b.WriteString(`<svg viewBox="0 0 800 190" role="img" aria-label="原始序列走势图"><line x1="50" x2="780" y1="160" y2="160" stroke="#dce5d4"/><polyline fill="none" stroke="#34764b" stroke-width="1.6" points="`)
	for i, p := range points {
		x := 50 + 730*float64(i)/float64(len(points)-1)
		y := 15 + 145*(high-p.Value)/(high-low)
		fmt.Fprintf(&b, "%.2f,%.2f ", x, y)
	}
	fmt.Fprintf(&b, `"/><text x="2" y="20">%.3g</text><text x="2" y="160">%.3g</text></svg>`, high, low)
	return template.HTML(b.String())
}

var reportTemplate = template.Must(template.New("report").Funcs(template.FuncMap{"chart": reportLine, "num": reportNumber}).Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>MarketCheck 检验报告</title>
<style>body{font-family:system-ui,"Microsoft YaHei";max-width:1040px;margin:35px auto;padding:25px;color:#284631;line-height:1.9}h1{font-size:27px}h2{font-size:19px}table{border-collapse:collapse;width:100%;font-size:12px}td,th{padding:8px;text-align:left;border-bottom:1px solid #ddd}.table-wrap{overflow-x:auto}.warning{background:#fff5db;padding:12px;margin:12px 0}svg{width:100%;height:auto}svg text{font:11px system-ui;fill:#7c8e6a}pre{white-space:pre-wrap;font:inherit}.summary{padding:18px;background:#eef4e8}dt{font-weight:600;margin-top:15px}dd{margin:5px 0;color:#526a4c}summary{cursor:pointer}</style></head><body>
<h1>MarketCheck · 市场假设检验</h1><div class="summary">{{.Run.Summary}}</div>
{{if .Result.Demo}}<div class="warning">合成演示数据，不代表真实市场。</div>{{end}}
<h2>主检验 · {{.Result.Verdict}}</h2><p>{{.Result.Explanation}}</p><ul>{{range .Result.Takeaways}}<li>{{.}}</li>{{end}}</ul>
<p>{{.Result.ModelDetail}}；有效样本 {{.Result.N}}；{{.Result.Start}} 至 {{.Result.End}}</p>
{{with .Result.Group}}<h2>主检验：条件组与对照组</h2><p>平均值和中位数单位：{{$.Result.YUnit}}。比例为样本描述，未单独检验比例差。</p>
<div class="table-wrap"><table><tr><th>组别</th><th>样本</th><th>平均结果</th><th>中位数</th><th>上涨比例（%）</th><th>下跌比例（%）</th></tr>
<tr><td>满足条件</td><td>{{num .event_count}}</td><td>{{num .event_mean}}</td><td>{{num .event_median}}</td><td>{{num .event_positive_rate}}</td><td>{{num .event_negative_rate}}</td></tr>
<tr><td>不满足条件</td><td>{{num .control_count}}</td><td>{{num .control_mean}}</td><td>{{num .control_median}}</td><td>{{num .control_positive_rate}}</td><td>{{num .control_negative_rate}}</td></tr></table></div>{{end}}
{{with .Result.Comparison}}<h2>补充对照：上涨、平稳、下跌</h2>{{if .available}}<p>Y 平均值和中位数单位：{{$.Result.YUnit}}。不足 15 个的组只作描述。</p>
<div class="table-wrap"><table><tr><th>情形</th><th>划分</th><th>样本</th><th>平均结果</th><th>中位数</th><th>上涨比例（%）</th><th>下跌比例（%）</th><th>不变比例（%）</th></tr>
{{range .groups}}<tr><td>{{.label}}</td><td>{{.rule}}</td><td>{{num .count}}</td><td>{{num .mean}}</td><td>{{num .median}}</td><td>{{num .positive_rate}}</td><td>{{num .negative_rate}}</td><td>{{num .zero_rate}}</td></tr>{{end}}
{{with .overall}}<tr><td>全部样本</td><td>前三组合计，不是独立对照</td><td>{{num .count}}</td><td>{{num .mean}}</td><td>{{num .median}}</td><td>{{num .positive_rate}}</td><td>{{num .negative_rate}}</td><td>{{num .zero_rate}}</td></tr>{{end}}</table></div>
<h3>三组之间，差多少？</h3><p>均值差为前组减后组，单位：{{if eq $.Result.YUnit "%"}}百分点{{else}}{{$.Result.YUnit}}{{end}}。</p>
<div class="table-wrap"><table><tr><th>比较</th><th>均值差</th><th>校正后区间</th><th>校正后 p 值</th><th>判断</th></tr>
{{range .pairs}}<tr><td>{{.label}}</td><td>{{num .effect}}</td><td>{{if .ci}}[{{num (index .ci 0)}}, {{num (index .ci 1)}}]{{else}}—{{end}}</td><td>{{num .p_adjusted}}</td><td>{{.status}}</td></tr>{{end}}</table></div>{{end}}<p>{{.note}}</p>
{{else}}<p>旧版结果未保存三组对照；重新确认假设并检验后可生成。</p>{{end}}
<details><summary>统计术语与例子 · 点开查看完整说明</summary><dl>{{range .Glossary}}<dt>{{.Term}}</dt><dd>{{.Explanation}}<p>例：{{.Example}}</p></dd>{{end}}</dl></details>
{{range .Result.Series}}<h2>{{.Label}}</h2>{{chart .Points}}{{end}}
<h2>AI 辅助解读</h2>{{if .Run.Narrative}}{{if ne .Run.NarrativeVersion 2}}<div class="warning">此解读来自旧版，请重新解读并导出，以计算表格为准。</div>{{end}}{{end}}<pre>{{if .Run.Narrative}}{{.Run.Narrative}}{{else}}本次未请求 AI 解读。{{end}}</pre>
<h2>数据来源与口径</h2><div class="table-wrap"><table><tr><th>标的</th><th>来源</th><th>单位</th><th>口径</th><th>覆盖范围</th></tr>{{range .Result.Sources}}<tr><td>{{.Label}}</td><td>{{.Provider}}</td><td>{{.Units}}</td><td>{{.Adjustment}}</td><td>{{.Start}} — {{.End}}</td></tr>{{end}}</table></div>
<h2>局限</h2>{{range .Result.Warnings}}<div class="warning">{{.}}</div>{{end}}<p>检验 ID：{{.Run.ID}}；创建时间：{{.Run.Created}}</p></body></html>`))

func (a *App) exportRun(w http.ResponseWriter, r *http.Request, id, kind string) {
	run := a.snapshot(id)
	if run == nil {
		http.NotFound(w, r)
		return
	}
	if kind == "report" {
		if len(run.Result) == 0 {
			http.Error(w, "请先完成检验", 400)
			return
		}
		var result ReportResult
		if json.Unmarshal(run.Result, &result) != nil {
			http.Error(w, "结果读取失败", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="marketcheck-%s-report.html"`, run.ID))
		_ = reportTemplate.Execute(w, map[string]any{"Run": run, "Result": result, "Glossary": statisticsTerms})
		return
	}
	var columns []string
	rows := [][]string{}
	switch kind {
	case "raw.csv":
		if len(run.Data) == 0 {
			http.Error(w, "请先获取数据", 400)
			return
		}
		var data struct {
			Series map[string]struct {
				Spec SeriesSpec `json:"spec"`
				Rows []Point    `json:"rows"`
			} `json:"series"`
		}
		if json.Unmarshal(run.Data, &data) != nil {
			http.Error(w, "数据读取失败", 500)
			return
		}
		columns = []string{"role", "source", "symbol", "date", "value"}
		for _, role := range []string{"target", "signal", "control1", "control2"} {
			s, ok := data.Series[role]
			if !ok {
				continue
			}
			for _, p := range s.Rows {
				rows = append(rows, []string{role, s.Spec.Source, s.Spec.Symbol, p.Date, strconv.FormatFloat(p.Value, 'g', -1, 64)})
			}
		}
	case "analysis.csv":
		if len(run.Result) == 0 {
			http.Error(w, "请先完成检验", 400)
			return
		}
		var data ReportResult
		if json.Unmarshal(run.Result, &data) != nil {
			http.Error(w, "结果读取失败", 500)
			return
		}
		columns = []string{"date", "x", "y"}
		if len(data.Rows) > 0 {
			for _, key := range []string{"control1", "control2"} {
				if _, ok := data.Rows[0][key]; ok {
					columns = append(columns, key)
				}
			}
		}
		for _, row := range data.Rows {
			line := []string{}
			for _, col := range columns {
				line = append(line, fmt.Sprint(row[col]))
			}
			rows = append(rows, line)
		}
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="marketcheck-%s-%s"`, run.ID, kind))
	_, _ = w.Write([]byte{0xef, 0xbb, 0xbf})
	writer := csv.NewWriter(w)
	writer.UseCRLF = true
	_ = writer.Write(columns)
	for _, row := range rows {
		for i, v := range row {
			if len(v) > 0 && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
				n, err := strconv.ParseFloat(v, 64)
				if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
					row[i] = "'" + v
				}
			}
		}
		_ = writer.Write(row)
	}
	writer.Flush()
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

type SeriesSpec struct {
	Source string `json:"source"`
	Symbol string `json:"symbol"`
	Label  string `json:"label"`
	Field  string `json:"field"`
}

type Hypothesis struct {
	Original   string       `json:"original"`
	Kind       string       `json:"kind"`
	Target     SeriesSpec   `json:"target"`
	Signal     SeriesSpec   `json:"signal"`
	Controls   []SeriesSpec `json:"controls"`
	Start      string       `json:"start"`
	End        string       `json:"end"`
	Frequency  string       `json:"frequency"`
	Timing     string       `json:"timing"`
	XTransform string       `json:"x_transform"`
	YTransform string       `json:"y_transform"`
	Lookback   int          `json:"lookback"`
	Horizon    int          `json:"horizon"`
	Lag        int          `json:"lag"`
	Operator   string       `json:"operator"`
	Threshold  float64      `json:"threshold"`
	FlatBand   float64      `json:"flat_band"`
	Direction  string       `json:"direction"`
}

func validSeries(s SeriesSpec) bool {
	if s.Field != "value" && s.Field != "volume" {
		return false
	}
	if len(s.Label) > 100 {
		return false
	}
	switch s.Source {
	case "tencent":
		return regexp.MustCompile(`^(sh|sz)[0-9]{6}$`).MatchString(s.Symbol)
	case "yahoo":
		return regexp.MustCompile(`^[A-Za-z0-9^=._-]{1,30}$`).MatchString(s.Symbol)
	case "fred":
		return s.Field == "value" && regexp.MustCompile(`^[A-Z0-9_]{1,50}$`).MatchString(s.Symbol)
	case "csv":
		return regexp.MustCompile(`^[A-Za-z0-9_-]{1,50}$`).MatchString(s.Symbol)
	}
	return false
}

func (h Hypothesis) Validate() error {
	if h.Kind != "event" && h.Kind != "relation" {
		return errors.New("请选择条件比较或变量关系")
	}
	if !validSeries(h.Target) || !validSeries(h.Signal) {
		return errors.New("数据源、代码或字段无效；A 股使用 sh600519 / sz000001 这样的代码")
	}
	if len(h.Controls) > 2 {
		return errors.New("首版最多两个控制变量")
	}
	for _, s := range h.Controls {
		if !validSeries(s) {
			return errors.New("控制变量的数据源或代码无效")
		}
	}
	start, err := time.Parse("2006-01-02", h.Start)
	if err != nil {
		return errors.New("开始日期无效")
	}
	end, err := time.Parse("2006-01-02", h.End)
	if err != nil || !end.After(start) {
		return errors.New("结束日期必须晚于开始日期")
	}
	if start.Year() < 1970 || end.Year()-start.Year() > 40 {
		return errors.New("请使用 1970 年之后、跨度不超过 40 年的区间")
	}
	if !end.Before(time.Now().UTC().Truncate(24 * time.Hour)) {
		return errors.New("为避免使用未收盘数据，结束日期最晚选昨天")
	}
	if !oneOf(h.Frequency, "daily", "weekly", "monthly") || !oneOf(h.XTransform, "return", "change", "level") || !oneOf(h.YTransform, "return", "change") {
		return errors.New("频率或变量变换无效")
	}
	if h.Lookback < 1 || h.Lookback > 60 || h.Horizon < 1 || h.Horizon > 20 || h.Lag < 0 || h.Lag > 20 {
		return errors.New("观察窗口、持有窗口或滞后期超出支持范围")
	}
	if !oneOf(h.Timing, "", "forward", "concurrent") {
		return errors.New("请选择同期关系或后续关系")
	}
	if h.Timing == "concurrent" && (h.Lag != 0 || h.Lookback != h.Horizon) {
		return errors.New("同期检验要求 X、Y 使用相同窗口，且额外滞后为 0")
	}
	if !oneOf(h.Operator, "ge", "le") || !oneOf(h.Direction, "positive", "negative", "two_sided") {
		return errors.New("条件或方向无效")
	}
	if h.Kind == "event" && len(h.Controls) > 0 {
		return errors.New("条件比较暂不支持控制变量；请切换变量关系 / 回归")
	}
	if math.IsNaN(h.FlatBand) || math.IsInf(h.FlatBand, 0) || h.FlatBand < 0 {
		return errors.New("平稳区间的半宽必须是大于或等于 0 的有限数值")
	}
	if len(h.Original) > 6000 {
		return errors.New("假设过长，请控制在 6000 字以内")
	}
	return nil
}

func oneOf(s string, opts ...string) bool {
	for _, o := range opts {
		if s == o {
			return true
		}
	}
	return false
}

func (h Hypothesis) Summary() string {
	freq := map[string]string{"daily": "观测日", "weekly": "周", "monthly": "月"}[h.Frequency]
	x := map[string]string{"return": "涨跌幅(%)", "change": "数值变化", "level": "数值水平"}[h.XTransform]
	y := map[string]string{"return": "收益率(%)", "change": "数值变化"}[h.YTransform]
	signal := h.Signal.Label
	if signal == "" {
		signal = h.Signal.Symbol
	}
	target := h.Target.Label
	if target == "" {
		target = h.Target.Symbol
	}
	signal += " [" + h.Signal.Source + ":" + h.Signal.Symbol + " / " + h.Signal.Field + "]"
	target += " [" + h.Target.Source + ":" + h.Target.Symbol + " / " + h.Target.Field + "]"
	base := fmt.Sprintf("%s 至 %s，%s的 %d %s%s，滞后 %d %s，与%s随后 %d %s%s", h.Start, h.End, signal, h.Lookback, freq, x, h.Lag, freq, target, h.Horizon, freq, y)
	if h.Timing == "concurrent" {
		base = fmt.Sprintf("%s 至 %s，同期检验：%s的 %d %s%s，与%s截至同一期的 %d %s%s", h.Start, h.End, signal, h.Lookback, freq, x, target, h.Horizon, freq, y)
	}
	if h.Kind == "event" {
		op := ">="
		if h.Operator == "le" {
			op = "<="
		}
		base += fmt.Sprintf("：比较信号 %s %.4g 与不满足条件的样本。", op, h.Threshold)
	} else {
		base += "的关系。"
	}
	if h.XTransform != "level" {
		base += fmt.Sprintf("补充对照：X > %.4g 为上涨，X < %.4g 为下跌，其余为平稳（含边界，单位与 X 相同）。", h.FlatBand, -h.FlatBand)
	}
	return base + "预期方向：" + map[string]string{"positive": "正向 / 高于对照", "negative": "负向 / 低于对照", "two_sided": "双向差异"}[h.Direction]
}

type ModelSpec struct {
	Method     string  `json:"method"`
	Confidence float64 `json:"confidence"`
	HACLags    int     `json:"hac_lags"`
	Reason     string  `json:"reason"`
}

func (m ModelSpec) Validate(h Hypothesis) error {
	if h.Kind == "event" && m.Method != "event" {
		return errors.New("条件比较需要条件均值差模型")
	}
	if h.Kind == "relation" && !oneOf(m.Method, "pearson", "spearman", "regression") {
		return errors.New("变量关系请选择相关或回归模型")
	}
	if len(h.Controls) > 0 && m.Method != "regression" {
		return errors.New("加入控制变量后需要使用回归")
	}
	if m.Confidence != .90 && m.Confidence != .95 && m.Confidence != .99 {
		return errors.New("置信水平请选择 90%、95% 或 99%")
	}
	if m.HACLags < h.Horizon || m.HACLags > 60 {
		return errors.New("HAC 滞后阶数不能小于结果窗口，且不能超过 60")
	}
	return nil
}

type Question struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Options []string `json:"options"`
}
type Clarification struct {
	Message   string     `json:"message"`
	Questions []Question `json:"questions"`
	Draft     Hypothesis `json:"draft"`
}
type Run struct {
	ID               string          `json:"id"`
	Created          string          `json:"created"`
	Status           string          `json:"status"`
	Message          string          `json:"message"`
	Hypothesis       Hypothesis      `json:"hypothesis"`
	Summary          string          `json:"summary"`
	Data             json.RawMessage `json:"data,omitempty"`
	Recommendation   *ModelSpec      `json:"recommendation,omitempty"`
	Model            *ModelSpec      `json:"model,omitempty"`
	Confirmed        bool            `json:"model_confirmed"`
	Result           json.RawMessage `json:"result,omitempty"`
	Narrative        string          `json:"narrative,omitempty"`
	NarrativeVersion int             `json:"narrative_version,omitempty"`
}

func cleanError(err error, secrets ...string) string {
	s := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "[已隐藏]")
		}
	}
	if len(s) > 500 {
		s = s[:500]
	}
	return strings.ToValidUTF8(s, "")
}

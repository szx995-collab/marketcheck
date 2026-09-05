package main

import (
	"encoding/json"
	"errors"
	"strings"
)

type StatisticsTerm struct {
	ID          string `json:"id"`
	Term        string `json:"term"`
	Explanation string `json:"explanation"`
	Example     string `json:"example"`
}

var statisticsTerms = func() []StatisticsTerm {
	data, err := webFiles.ReadFile("web/statistics.json")
	if err != nil {
		panic(err)
	}
	var terms []StatisticsTerm
	if err := json.Unmarshal(data, &terms); err != nil {
		panic(err)
	}
	return terms
}()

// The model chooses explanatory topics; all claims and numbers come from the fixed analysis.
func groundedInterpretation(run *Run, termIDs []string) (string, error) {
	if len(termIDs) == 0 || len(termIDs) > 3 {
		return "", errors.New("AI 未返回有效的解释重点，请重试；上方统计结论不受影响")
	}
	var result ReportResult
	if err := json.Unmarshal(run.Result, &result); err != nil {
		return "", err
	}
	parts := []string{}
	if result.Demo {
		parts = append(parts, "合成演示数据，不代表真实市场。")
	}
	parts = append(parts, result.Verdict+"。"+result.Explanation)
	parts = append(parts, result.Takeaways...)
	if result.Comparison != nil {
		if pairs, ok := result.Comparison["pairs"].([]any); ok {
			for _, pair := range pairs {
				if p, ok := pair.(map[string]any); ok {
					label, _ := p["label"].(string)
					status, _ := p["status"].(string)
					parts = append(parts, label+"："+status+"（补充比较，使用校正后的区间）。")
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, id := range termIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		found := false
		for _, term := range statisticsTerms {
			if term.ID == id {
				parts = append(parts, "理解这次结果 · "+term.Term+"："+term.Explanation)
				found = true
				break
			}
		}
		if !found {
			return "", errors.New("AI 返回了未知的解释主题，请重试；上方统计结论不受影响")
		}
	}
	parts = append(parts, result.Warnings...)
	return strings.Join(parts, "\n\n"), nil
}

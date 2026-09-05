package main

type CatalogItem struct {
	SeriesSpec
	Group string `json:"group"`
	Note  string `json:"note"`
}

var catalog = []CatalogItem{
	{SeriesSpec{"tencent", "sh000300", "沪深300", "value"}, "A 股", "指数点位；腾讯公开日线"},
	{SeriesSpec{"tencent", "sh000001", "上证指数", "value"}, "A 股", "指数点位；腾讯公开日线"},
	{SeriesSpec{"tencent", "sh600519", "贵州茅台", "value"}, "A 股", "前复权收盘价；腾讯公开日线"},
	{SeriesSpec{"tencent", "sz000001", "平安银行", "value"}, "A 股", "前复权收盘价；腾讯公开日线"},
	{SeriesSpec{"yahoo", "SPY", "标普500 ETF · SPY", "value"}, "美股", "ETF 复权收盘价，美元；不是指数点位"},
	{SeriesSpec{"yahoo", "^GSPC", "标普500指数", "value"}, "美股", "指数收盘点位；不是 ETF"},
	{SeriesSpec{"yahoo", "^IXIC", "纳斯达克综合指数", "value"}, "美股", "指数收盘点位；不同于纳斯达克100指数或 QQQ"},
	{SeriesSpec{"yahoo", "QQQ", "纳斯达克100 ETF · QQQ", "value"}, "美股", "ETF 复权收盘价，美元"},
	{SeriesSpec{"yahoo", "AAPL", "苹果 · AAPL", "value"}, "美股", "股票复权收盘价，美元"},
	{SeriesSpec{"yahoo", "GC=F", "黄金期货 · GC=F", "value"}, "黄金 / 原油", "黄金期货连续报价，美元/盎司；换月会影响收益，不是现货"},
	{SeriesSpec{"yahoo", "GLD", "黄金 ETF · GLD", "value"}, "黄金 / 原油", "黄金 ETF 复权价格，美元/份；不是现货金价"},
	{SeriesSpec{"fred", "DCOILWTICO", "WTI 原油现货", "value"}, "黄金 / 原油", "FRED / EIA；美元/桶；需 FRED Key"},
	{SeriesSpec{"fred", "DCOILBRENTEU", "布伦特原油现货", "value"}, "黄金 / 原油", "FRED / EIA；美元/桶；需 FRED Key"},
	{SeriesSpec{"fred", "DGS10", "美国10年国债收益率", "value"}, "FRED 宏观", "百分数水平；变化使用百分点差，不使用价格收益率"},
	{SeriesSpec{"fred", "DFF", "联邦基金有效利率", "value"}, "FRED 宏观", "日频，百分数水平；需 FRED Key"},
	{SeriesSpec{"fred", "DTWEXBGS", "广义美元指数", "value"}, "FRED 宏观", "日频指数；需 FRED Key"},
	{SeriesSpec{"fred", "CPIAUCSL", "美国 CPI", "value"}, "FRED 宏观", "月频；历史修订值，不能当成当时已发布的信息"},
	{SeriesSpec{"fred", "UNRATE", "美国失业率", "value"}, "FRED 宏观", "月频；百分数水平，使用月频检验"},
}

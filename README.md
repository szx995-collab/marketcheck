# MarketCheck · 市场假设检验器

本地个人工具：自然语言假设 → AI 澄清 → 确认假设 → 自动取数 → 确认数学模型 → 内置统计检验 → 图表和解读。

主后端使用 Go，统计模块为仓库内固定的 Python 程序，界面使用原生 HTML / JavaScript / SVG。大模型不会生成或执行分析代码。

## 开始使用

Windows x64，需要 Python 3.11 或更高版本。第一次使用双击 `setup.bat`；它安装项目依赖，缺少 Go 时会下载到项目内的 `.tools`。

然后双击 `start.bat`，或运行：

```powershell
powershell -ExecutionPolicy Bypass -File .\start.ps1
```

已经启动时会打开现有页面。正常启动后可在窗口按 Ctrl+C 停止；后台运行的实例可用 `powershell -ExecutionPolicy Bypass -File .\stop.ps1` 停止。

打开 **http://127.0.0.1:8765**，在“模型与数据源设置”填写 DeepSeek / OpenAI Key 和 FRED Key。默认仅在本次进程内保存；勾选“记住”才会明文保存到本机 `.local/settings.json`。

界面无需修改配置文件。可以先点击“用合成数据体验完整计算”，检查数据、确认模型后生成结果。演示数据始终明确标注，不能用于真实市场判断。

也支持环境变量 `DEEPSEEK_API_KEY`、`OPENAI_API_KEY`、`FRED_API_KEY`、`MARKETCHECK_PYTHON`、`MARKETCHECK_PORT`。`.env.example` 仅示例，程序不会自动读取 `.env`。非 Windows 系统可创建 `.venv`、安装 `requirements.txt` 后在项目目录执行 `go run .`。

## 支持的数据

| 市场 | 来源与示例 | Key |
|---|---|---|
| 沪深 A 股和指数 | 腾讯公开日线：`sh600519`、`sz000001`、`sh000300` | 不需要 |
| 美股和 ETF | Yahoo / yfinance：`AAPL`、`SPY`、`QQQ` | 不需要 |
| 黄金 | `GC=F` 黄金期货连续报价；`GLD` 黄金 ETF | 不需要 |
| 原油现货 | FRED：`DCOILWTICO` WTI、`DCOILBRENTEU` 布伦特 | FRED |
| 利率、美元、宏观 | FRED：`DGS10`、`DFF`、`DTWEXBGS`、`CPIAUCSL`、`UNRATE`，也可填其他系列 ID | FRED |
| 自有数据、现货黄金 | UTF-8 CSV：`date,value`，使用成交量时增加 `volume` 列 | 不需要 |

黄金期货、黄金 ETF 和现货黄金是不同口径。首版没有把替代品冒充现货金价的自动回退。精确现货金价可以通过 CSV 导入。公开行情接口出现网络错误或限流时会报告失败，保留假设，并允许重试或换用 CSV。

股票使用复权收盘价。CSV 的单位、复权和日期口径由你确认；日期必须为 `YYYY-MM-DD`，值不带百分号。相同日期的冲突数值会报错。一个 CSV ID 可供多个变量引用。下载的原始数据缓存 12 小时。

## 使用示例

输入“美债收益率上升后，黄金是否下跌”，澄清时选择：黄金 ETF GLD、FRED DGS10、周频、利率使用原单位变化、GLD 使用未来一期收益率、预期负向。确认后程序下载数据，再让 AI 推荐相关或回归方法。只有点击“确认模型，运行检验”才开始计算。

阈值 `-3` 在“涨跌幅”下代表 `-3%`；利率原单位为百分数水平时，变化 `0.1` 代表 `0.1` 个百分点。宏观月频数据需选择月频，不会通过前向填充伪造日频样本。

## 内置检验与口径

- 条件比较：条件指示变量的 OLS 系数，即条件组均值减其余样本均值；HAC 稳健标准误。
- Pearson / Spearman：相关系数，1500 次循环时间分块 Bootstrap 百分位区间；不显示独立样本假设下的 p 值。
- 回归：OLS + HAC；最多两个控制变量，统一使用 X 的变换、回看窗口和滞后期。
- 有效样本至少 40 个；条件组和对照组各至少 15 个；时间依赖阶数需要与窗口和样本量匹配。

每个序列先按自己的观测日计算信号、滞后及未来结果，再按日期取交集。日频窗口指各序列自身的有效观测日；周/月频取完整周期的末值，不补缺失值。未来结果不能越过选定结束日期；当日未收盘数据不进入检验。

FRED 返回当前可获得的历史版本，不是逐日复原的“当时已知”数据；跨市场收盘与发布时间也不同。这个版本用于**历史关联检验**，不进行因果推断或可交易策略回测。连续期货包含换月影响。置信区间依赖模型假设，不是“假设为真的概率”；反复调参数再挑显著结果会夸大证据。

结果包括效应、置信区间、适用的双侧 p 值、样本量、原序列图、散点图和分布图。支持原始数据 CSV、有效样本 CSV、独立 HTML 报告和本地历史记录。AI 解读失败不会影响已计算的数字与图表。

## 检查与代码

```powershell
go test ./...
go vet ./...
python -m unittest discover -s tests -v
```

主要代码：`main.go` / `workflow.go` 管理流程；`llm.go` 接入 DeepSeek 和 GPT；`analysis/engine.py` 内置取数与统计；`web/` 提供界面。测试检查确认步骤、密钥边界、日期窗口、低频数据、已知效应和失败路径。

`.local/`、`.tools/`、`.venv/` 和密钥文件已加入 `.gitignore`。提交 GitHub 前保留源码与配置示例即可，个人数据和 Key 留在本机。

参考接口：[FRED API](https://fred.stlouisfed.org/docs/api/fred/series_observations.html)、[腾讯接口说明（AKShare）](https://akshare.akfamily.xyz/data/stock/stock.html)、[yfinance](https://ranaroussi.github.io/yfinance/)、[DeepSeek JSON](https://api-docs.deepseek.com/guides/json_mode/)、[OpenAI Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs)。

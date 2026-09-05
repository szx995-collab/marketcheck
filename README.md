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

打开 **http://127.0.0.1:8765**，在“模型与数据源设置”选择服务，点击模型和推理强度选项卡，填写对应 API Key，再点击“保存并测试模型”。模型名称与强度无需手填。也可以选 Codex 复用本机 ChatGPT 登录；需要 FRED 数据时仍填写 FRED Key。默认仅在本次进程内保存；勾选“记住”才会明文保存到本机 `.local/settings.json`。

界面无需修改配置文件。可以先点击“用合成数据体验完整计算”，检查数据、确认模型后生成结果。演示数据始终明确标注，不能用于真实市场判断。

也支持下面列出的模型 Key 环境变量，以及 `FRED_API_KEY`、`MARKETCHECK_PYTHON`、`MARKETCHECK_PORT`、`MARKETCHECK_CODEX`。`.env.example` 仅示例，程序不会自动读取 `.env`。非 Windows 系统可创建 `.venv`、安装 `requirements.txt` 后在项目目录执行 `go run .`。

## 模型 API 选择

| 设置中的服务 | 初始模型（可在选项卡切换） | Key 环境变量 | 接口 |
|---|---|---|---|
| DeepSeek | `deepseek-v4-flash` | `DEEPSEEK_API_KEY` | DeepSeek Chat Completions |
| OpenAI GPT | `gpt-4.1-mini` | `OPENAI_API_KEY` | OpenAI Chat Completions |
| GLM（智谱） | `glm-4.7-flash` | `GLM_API_KEY`（也支持 `ZHIPUAI_API_KEY`） | [智谱国内 API](https://docs.bigmodel.cn/api-reference/模型-api/对话补全) |
| Kimi（月之暗面） | `kimi-k2.6` | `MOONSHOT_API_KEY`（也支持 `KIMI_API_KEY`） | [Moonshot 国内 API](https://platform.kimi.com/docs/api/chat) |
| Claude（Anthropic） | `claude-haiku-4-5-20251001` | `ANTHROPIC_API_KEY`（也支持 `CLAUDE_API_KEY`） | [Messages API](https://platform.claude.com/docs/en/api/overview) + [结构化输出](https://platform.claude.com/docs/en/build-with-claude/structured-outputs) |
| Grok（xAI） | `grok-4.6` | `XAI_API_KEY`（也支持 `GROK_API_KEY`） | [xAI Chat Completions](https://docs.x.ai/developers/model-capabilities/legacy/chat-completions) |
| 自定义 OpenAI 兼容接口 | 从接口读取列表后选择 | `COMPATIBLE_API_KEY` | 自填 Base URL，支持 Chat Completions 协议 |
| Codex（ChatGPT 登录） | 自动选择（CLI 默认值） | 无需模型 API Key | 见下节 |

界面只显示当前模型服务的 Key 输入框，切换不会丢掉其他服务的已保存 Key。勾选“清除已保存的全部 Key”会清除所有模型和 FRED Key。预置模型不代表账号已获得权限，实际可用性以“保存并测试模型”为准。通过 API Key 调用按服务商规则计费；通过 Codex 登录调用使用 ChatGPT 账号的 Codex 额度。Claude 预置模型均按官方结构化输出协议接入。

其他服务可选择“自定义 OpenAI 兼容接口”，填写服务商的 Base URL（例如 `https://服务商域名/v1`）和 Key，点击“读取模型列表”后直接选模型。接口需支持 `GET /models`；已保存的旧模型仍可选择。支持填写完整 `/chat/completions` 地址，程序不会重复拼接。已识别模型沿用对应厂商的推理参数；未知模型只显示“模型默认”，不猜测接口能力。若服务不支持 `response_format`，保持“JSON 模式”不勾选即可。远程地址使用 HTTPS，本机 `http://localhost:端口/v1` 也支持；URL 中不要放 Key。更换自定义地址后必须重新填对应 Key，避免把旧 Key 发到另一个服务。

GLM 默认连接 `open.bigmodel.cn`，Kimi 默认连接 `api.moonshot.cn`；不同平台、地区的 Key 不保证通用，其他地址应配合“自定义接口”使用。`COMPATIBLE_BASE_URL` 可作为自定义地址的环境变量。接口返回格式不符或输出被截断时最多重试一次，鉴权、限流等错误会明确显示，已完成的统计结果保留。

新增服务已覆盖请求格式、Key 隔离、Claude 内容解析、截断响应、重试和失败路径的模拟测试；没有对应有效 Key 的服务不声称已通过真实账号调用。可以填入自己的 Key 后，用“保存并测试模型”确认账号实际可用性。

## 模型与推理强度

模型与强度是两组独立选项卡。切换模型后只显示它支持的档位；“模型默认”表示不覆盖服务的推理参数。选择 **Codex → GPT-6 Astra → 极高** 即会传入模型 `gpt-6-astra` 和强度 `xhigh`，无需拼接模型名称。

| 服务 / 模型 | 可选推理设置 | 实际参数 |
|---|---|---|
| OpenAI GPT-6 Astra、Codex 中的 Astra | 低、中、高、极高、最高 | API `reasoning_effort` / CLI `model_reasoning_effort` |
| OpenAI GPT-5.6 Luna | 关闭、低、中、高、极高、最高 | `reasoning_effort` |
| GPT-4.1 mini | 模型默认 | 不支持推理调节 |
| DeepSeek V4 Flash / Pro | 关闭、低、高、最高 | `thinking.type` + `reasoning_effort` |
| GLM-5.2 | 关闭、高、最高 | `thinking.type` + `reasoning_effort`；不重复展示服务会合并的同义档位 |
| GLM-5.1 / 4.7 / 4.7 Flash、Kimi K2.6 | 关闭、开启 | `thinking.type` |
| Kimi K3 | 低、高、最高 | `reasoning_effort`；始终推理 |
| Kimi K2.7 Code | 模型默认 | 始终思考，无可调档位 |
| Claude Sonnet 5 / Opus 5 | 低、中、高、极高、最高 | 自适应思考 + `output_config.effort` |
| Claude Sonnet 4.6 / Opus 4.6 | 低、中、高、最高 | 自适应思考 + `output_config.effort` |
| Claude Haiku 4.5 | 关闭、低、中、高 | 思考预算分别为 1,024 / 4,096 / 8,192 tokens，非原生 effort 等级 |
| Grok 4.6 / 4.5 | 低、中、高；4.6 另有极高 | `reasoning_effort`；始终推理 |

档位来自各家公开协议：[OpenAI Astra](https://developers.openai.com/api/docs/models/gpt-6-astra)、[DeepSeek](https://api-docs.deepseek.com/guides/thinking_mode/)、[智谱](https://docs.bigmodel.cn/cn/guide/capabilities/thinking)、[Kimi](https://platform.kimi.com/docs/api/chat)、[Claude effort](https://platform.claude.com/docs/en/build-with-claude/effort) / [思考预算](https://platform.claude.com/docs/en/build-with-claude/extended-thinking)、[Grok](https://docs.x.ai/developers/model-capabilities/text/reasoning)。模型名单为预置目录，并非账号权限列表。

更高强度通常更慢、消耗更多额度，不等于统计结论更可靠或更显著。程序会给推理留出更大的输出预算，推理请求最多等待约 10 分钟；输出被截断仍可能失败，可降档重试。所有统计检验依旧由内置工具完成。

## 使用 ChatGPT 订阅中的 Codex 额度

1. 安装官方 [Codex](https://developers.openai.com/codex/quickstart)，用 ChatGPT 账号登录。已在本机 Codex 登录过的用户可直接下一步；也可在终端运行 `codex login`。
2. 打开 MarketCheck 的“模型与数据源设置”，选择 **Codex（ChatGPT 登录）**。程序会检测登录，显示“已检测到 ChatGPT 登录”。无需 OpenAI API Key。
3. 点击“自动选择（Codex 默认）”，或者选择具体模型与推理强度，例如 **GPT-6 Astra → 极高**。点击 **保存并测试模型**，成功后即可正常澄清假设、推荐方法和生成辅助解读。

这会使用 ChatGPT 账号的 Codex 额度，与 Codex 中的其他任务共享。是否包含 Codex、可用模型与使用限额取决于账号当前订阅及服务规则；订阅额度不等同于 OpenAI API 余额。额度不足或登录失效会显示错误，可以等恢复或手动切换 DeepSeek，程序不会自动走付费 API。具体额度和恢复时间在 Codex 中查看。说明依据：[Codex 认证](https://developers.openai.com/codex/auth)、[套餐与额度](https://developers.openai.com/codex/pricing)。

本功能调用官方 `codex exec`（已验证 CLI **0.153.4**），不读取、复制或保存 ChatGPT 登录令牌。登录由 Codex 自己管理，不受 MarketCheck“记住 Key / 清除 Key”影响。每次使用独立临时工作目录、只读沙箱、严格 JSON 输出，并关闭 shell、浏览器、插件、连接器和用户 hooks；不加载个人 Codex 配置或项目指令。数据源 Key 不传给该子进程，取数与统计仍由固定程序执行。参考：[官方非交互调用](https://developers.openai.com/codex/noninteractive)。

找不到 CLI 时，先重启 MarketCheck；Windows 会自动查找 PATH、Codex 桌面安装目录和常见 npm 安装目录。特殊安装可在启动前设置 `$env:MARKETCHECK_CODEX='C:\实际路径\codex.exe'`。旧版本不支持所需参数时请更新 Codex。单次 Codex 调用最多等待约 10 分钟，失败时已填假设与已完成结果保留。

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

**同日联动**：检验“纳指上涨的日子，标普是否也趋向上涨”，选择纳斯达克综合指数（`^IXIC`）作为 X、标普500指数（`^GSPC`）作为 Y，时间关系选“同期关系”，日频、窗口 1、两者均用涨跌幅。可用相关分析检验同日涨跌幅关系，并查看纳指上涨、平稳、下跌时，标普的平均收益和上涨比例。上涨比例目前是描述性结果，不使用均值检验的 p 值判断概率差异显著。

同期模式的 X、Y 使用相同长度的窗口，均截至当期，额外滞后固定为 0。变化类变量仅保留窗口起止日期一致的样本，避免缺失行情导致不同时间跨度被混在一起。周频、月频也可做同期比较；同一日期不意味着不同市场收盘时刻一致。同期关系用于描述联动，不能据此宣称预测或因果。旧版历史检验保持原来的后续关系。

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

## 看懂方法，也看反面

假设确认、模型选择和结果页都提供“统计术语与例子”，解释相关、回归、HAC、Bootstrap、置信区间、p 值、效应大小、控制变量和多重比较等 17 个主题。导出的 HTML 报告同样包含这些说明，不依赖 AI 才能阅读。

例如检验“油价上涨时，股票表现更弱”，先确认 X 的平稳区间。若 X 为油价涨跌幅，半宽 `0.1` 表示 `−0.1% ≤ X ≤ 0.1%` 算平稳，高于区间算上涨，低于区间算下跌。这个值可以修改，不是通用市场标准；切换 X 的单位时会重置为 0，需按新单位确认。0 表示只有恰好零变化算平稳。X 为原数值水平时，不把高价误称上涨，页面会提示改为变化指标。

结果同时展示三组的样本量、平均结果、中位数、上涨 / 下跌 / 不变比例以及总体参考，避免只看条件成立的一面。三组覆盖同一批有效样本；主检验有控制变量时，补充表仍展示未调整的组均值，会注明与主回归的区别。

上涨−平稳、上涨−下跌、平稳−下跌三项均值差使用完整样本顺序的 OLS + HAC，并按 Bonferroni 校正区间和 p 值。每个参与比较的组至少 15 个样本；缺组或不足时明确标为无法判断，不填成零。校正区间的整体置信水平沿用已确认值，仍依赖 HAC 的近似推断假设。此校正仅覆盖三项预设的补充比较，不能消除反复改参数挑结果的影响。方法依据见 [statsmodels HAC 文档](https://www.statsmodels.org/stable/generated/statsmodels.regression.linear_model.RegressionResults.get_robustcov_results.html) 和 [NIST Bonferroni 说明](https://www.itl.nist.gov/div898/handbook/prc/section4/prc463.htm)。

主结论明确区分“支持预期方向”“与预期相反”“证据不足”，并给出实际差多少、反面对照和适用范围。条件组收益 1%、对照组 2%，应说“相对更弱”，不能说“股价下跌”。涨跌比例仅作历史样本描述，当前没有单独检验概率差；相关系数也不等于涨跌概率。旧历史结果保留原样，重新确认平稳区间并检验后才能生成新版对照。

AI 辅助解读从内置术语中选择本次最值得解释的 1–3 个主题。报告中的数字、涨跌判断、支持程度和局限直接引用固定分析结果，避免大模型在转述时把负数说成正数。旧版 AI 解读不会自动覆盖，可点击“重新解读”更新。

## 检查与代码

```powershell
go test ./...
go vet ./...
python -m unittest discover -s tests -v
```

主要代码：`main.go` / `workflow.go` 管理流程；`providers.go` / `llm.go` 接入各家模型 API，`codex.go` 接入本机 Codex；`analysis/engine.py` 内置取数与统计；`web/` 提供界面。测试检查确认步骤、密钥边界、日期窗口、低频数据、已知效应和失败路径。

`.local/`、`.tools/`、`.venv/` 和密钥文件已加入 `.gitignore`。提交 GitHub 前保留源码与配置示例即可，个人数据和 Key 留在本机。

参考接口：[FRED API](https://fred.stlouisfed.org/docs/api/fred/series_observations.html)、[腾讯接口说明（AKShare）](https://akshare.akfamily.xyz/data/stock/stock.html)、[yfinance](https://ranaroussi.github.io/yfinance/)、[DeepSeek JSON](https://api-docs.deepseek.com/guides/json_mode/)、[OpenAI Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs)。

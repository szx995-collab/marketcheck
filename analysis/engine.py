"""Fixed market-data adapters and statistical routines. No generated code is executed."""
from __future__ import annotations

import contextlib
from datetime import date, datetime, timedelta
import hashlib
import io
import json
import math
import os
from pathlib import Path
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
LOCAL = ROOT / ".local"
PACKAGES = ROOT / ".tools" / "python-packages"
if PACKAGES.exists():
    sys.path.insert(0, str(PACKAGES))
os.environ.setdefault("OPENBLAS_NUM_THREADS", "1")

import numpy as np
import pandas as pd


class UserError(Exception):
    pass


def get_json(url, params=None):
    if params:
        url += "?" + urllib.parse.urlencode(params)
    request = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0 MarketCheck/1.0"})
    for attempt in range(2):
        try:
            with urllib.request.urlopen(request, timeout=25) as response:
                return json.loads(response.read(12_000_000))
        except urllib.error.HTTPError as exc:
            if exc.code in (429, 502, 503, 504) and attempt == 0:
                time.sleep(1)
                continue
            raise UserError(f"数据接口返回 HTTP {exc.code}，请检查 Key 权限、代码或稍后重试") from None
        except (TimeoutError, OSError, ValueError):
            if attempt == 0:
                continue
            raise UserError("数据接口连接超时或返回异常，请检查网络；可使用 CSV 导入") from None


def fred(spec, start, end):
    key = os.environ.get("FRED_API_KEY", "")
    if not key:
        raise UserError("此假设需要 FRED 数据，请先在设置中填写 FRED_API_KEY")
    base = {"series_id": spec["symbol"], "api_key": key, "file_type": "json"}
    metadata = get_json("https://api.stlouisfed.org/fred/series", base)
    series = metadata.get("seriess", [])
    if not series:
        raise UserError("FRED 未找到该序列，请检查系列 ID")
    metadata = series[0]
    payload = get_json("https://api.stlouisfed.org/fred/series/observations", {
        **base, "observation_start": start, "observation_end": end, "limit": 100000,
    })
    if payload.get("count", 0) > len(payload.get("observations", [])):
        raise UserError("FRED 返回了不完整数据，请缩短日期范围")
    rows = [{"date": item["date"], "value": float(item["value"])}
            for item in payload.get("observations", []) if item["value"] not in (".", "")]
    return rows, {"provider": "FRED", "title": metadata.get("title"), "units": metadata.get("units"),
                  "native_frequency": metadata.get("frequency_short"), "adjustment": "FRED 当前历史版本",
                  "url": "https://fred.stlouisfed.org/series/" + spec["symbol"],
                  "warnings": ["FRED 使用当前可获得的历史版本，观测日期不等于发布日期；本结果仅描述历史关联，不是实时预测回测。"]}


def tencent(spec, start, end):
    symbol = spec["symbol"]
    if not re.fullmatch(r"(sh|sz)\d{6}", symbol):
        raise UserError("A 股代码格式为 sh600519、sz000001 等")
    rows = []
    adjusted = False
    for year in range(int(start[:4]), int(end[:4]) + 1, 2):
        payload = get_json("https://proxy.finance.qq.com/ifzqgtimg/appstock/app/newfqkline/get", {
            "param": f"{symbol},day,{year}-01-01,{min(year + 1, int(end[:4]))}-12-31,640,qfq",
        })
        data = payload.get("data", {}).get(symbol, {})
        block = data.get("qfqday", [])
        if block:
            adjusted = True
        elif symbol.startswith(("sh000", "sz399")):
            block = data.get("day", [])
        elif data.get("day"):
            raise UserError("腾讯只返回未复权股票数据，不能直接计算收益；请导入复权 CSV")
        for row in block:
            if start <= row[0] <= end:
                value = float(row[5] if spec.get("field") == "volume" else row[2])
                rows.append({"date": row[0], "value": value})
    # The two-year requests are non-overlapping; conflicting dates still fail in normalize().
    return rows, {"provider": "腾讯证券公开接口", "units": "源接口成交量单位" if spec.get("field") == "volume" else ("指数点" if not adjusted else "人民币"),
                  "native_frequency": "D", "adjustment": "前复权" if adjusted else "指数原始点位",
                  "url": "https://gu.qq.com/" + symbol,
                  "warnings": ["公开行情接口可能限流；复权价格不是含交易费用的可执行收益。"]}


def yahoo(spec, start, end):
    import yfinance as yf
    cache = LOCAL / "yfinance"
    cache.mkdir(parents=True, exist_ok=True)
    yf.set_tz_cache_location(str(cache))
    ticker = yf.Ticker(spec["symbol"])
    try:
        history = ticker.history(start=start, end=(date.fromisoformat(end) + timedelta(days=1)).isoformat(),
                                 interval="1d", auto_adjust=True, actions=False, raise_errors=True, timeout=25)
    except Exception:
        raise UserError("Yahoo 行情下载失败，可能是代码、网络或限流问题；请重试或导入 CSV") from None
    column = "Volume" if spec.get("field") == "volume" else "Close"
    if history.empty or column not in history:
        raise UserError("Yahoo 在此区间没有返回行情，请检查代码与上市时间")
    rows = [{"date": index.strftime("%Y-%m-%d"), "value": float(value)}
            for index, value in history[column].items() if pd.notna(value)]
    warnings = ["Yahoo 公开行情由 yfinance 读取；网络或限流时可使用 CSV。"]
    if spec["symbol"].endswith("=F"):
        warnings.append("该序列是期货连续报价，包含换月影响；不是现货价格或可直接交易的连续收益。")
    if spec["symbol"] == "GLD":
        warnings.append("GLD 是黄金 ETF，单位为每份基金的美元价格，不是每盎司现货金价。")
    return rows, {"provider": "Yahoo Finance / yfinance", "units": "成交量" if column == "Volume" else "报价币种（美国标的为美元）",
                  "native_frequency": "D", "adjustment": "auto_adjust=True 的复权收盘价",
                  "url": "https://finance.yahoo.com/quote/" + urllib.parse.quote(spec["symbol"], safe=""), "warnings": warnings}


def csv_data(spec, text):
    try:
        frame = pd.read_csv(io.StringIO(text.lstrip("\ufeff")), dtype={"date": str})
    except Exception:
        raise UserError("无法读取 CSV，请使用 UTF-8 编码和逗号分隔") from None
    column = "volume" if spec.get("field") == "volume" else "value"
    if not {"date", column}.issubset(frame.columns):
        raise UserError(f"CSV 需要 date,{column} 列；日期使用 YYYY-MM-DD，数值不带百分号")
    try:
        rows = [{"date": str(r["date"]), "value": float(r[column])} for r in frame.to_dict("records")]
    except (TypeError, ValueError):
        raise UserError("CSV 数值列存在无法转换的内容") from None
    return rows, {"provider": "用户 CSV", "units": "用户提供的原单位", "native_frequency": "auto",
                  "adjustment": "由用户确认", "warnings": ["CSV 的价格复权、单位和发布时间口径由用户确认。"]}


def normalize(rows, start, end):
    unique = {}
    duplicate = 0
    for row in rows:
        try:
            day = date.fromisoformat(row["date"])
            value = float(row["value"])
        except (ValueError, TypeError, KeyError):
            raise UserError("数据中存在无效日期或数值") from None
        if not math.isfinite(value):
            raise UserError("数据中存在空值或无穷值，请清理后重新导入")
        if not start <= day.isoformat() <= end:
            continue
        if row["date"] in unique:
            if unique[row["date"]] != value:
                raise UserError("同一天存在冲突数值，无法确定应该使用哪一条")
            duplicate += 1
        unique[row["date"]] = value
    clean = [{"date": k, "value": v} for k, v in sorted(unique.items())]
    if len(clean) < 10:
        raise UserError("可用观测少于 10 条，请扩大区间或检查数据源")
    return clean, duplicate


def fetch(request):
    h = request["hypothesis"]
    units = {"daily": 2, "weekly": 8, "monthly": 32}[h["frequency"]]
    warmup = (h["lookback"] + h["lag"] + 3) * units
    start = (date.fromisoformat(h["start"]) - timedelta(days=warmup)).isoformat()
    end = h["end"]
    specs = [("target", h["target"]), ("signal", h["signal"])] + [(f"control{i+1}", s) for i, s in enumerate(h.get("controls") or [])]
    demo = bool(request.get("demo", False))
    if demo:
        dates = pd.bdate_range(start, end)
        rng = np.random.default_rng(20260905)
        x = rng.normal(0, 1.1, len(dates))
        target = 100 * np.exp(np.cumsum(0.002 * np.roll(x, 1) + rng.normal(.00015, .009, len(dates))))
        signal = 80 * np.exp(np.cumsum(x / 100))
    series, summaries, warnings = {}, [], []
    cache_dir = LOCAL / "cache"
    cache_dir.mkdir(parents=True, exist_ok=True)
    loaded = {}
    for role, spec in specs:
        fingerprint = json.dumps([spec["source"], spec["symbol"], spec.get("field", "value"), start, end])
        cached = False
        if demo:
            values = target if role == "target" else signal
            rows = [{"date": d.strftime("%Y-%m-%d"), "value": float(v)} for d, v in zip(dates, values)]
            meta = {"provider": "合成演示数据", "units": "演示数值", "native_frequency": "D", "adjustment": "不适用", "warnings": ["本次使用合成数据，仅演示流程；不是任何真实市场结论。"]}
        elif fingerprint in loaded:
            rows, meta = loaded[fingerprint]
        elif spec["source"] == "csv":
            text = request.get("uploads", {}).get(spec["symbol"], "")
            if not text:
                raise UserError(f"请为 {spec['symbol']} 上传 CSV")
            rows, meta = csv_data(spec, text)
        else:
            cache_file = cache_dir / (hashlib.sha256(fingerprint.encode()).hexdigest() + ".json")
            if cache_file.exists() and time.time() - cache_file.stat().st_mtime < 43200:
                try:
                    rows, meta = json.loads(cache_file.read_text(encoding="utf-8"))
                    cached = True
                except (ValueError, OSError):
                    rows = None
            else:
                rows = None
            if rows is None:
                adapter = {"fred": fred, "tencent": tencent, "yahoo": yahoo}.get(spec["source"])
                if adapter is None:
                    raise UserError("不支持的数据源")
                rows, meta = adapter(spec, start, end)
                rows, _ = normalize(rows, start, end)
                meta["retrieved_at"] = datetime.now().astimezone().isoformat(timespec="seconds")
                cache_file.write_text(json.dumps([rows, meta], ensure_ascii=False), encoding="utf-8")
        rows, duplicates = normalize(rows, start, end)
        loaded[fingerprint] = (rows, meta)
        in_range = [r for r in rows if h["start"] <= r["date"] <= end]
        if len(in_range) < 10:
            raise UserError(f"{spec.get('label') or spec['symbol']} 在指定区间内不足 10 条观测")
        row_summary = {"role": role, "label": spec.get("label") or spec["symbol"], "symbol": spec["symbol"],
                       "count": len(in_range), "start": in_range[0]["date"], "end": in_range[-1]["date"],
                       "warmup_count": len(rows) - len(in_range), "duplicates_removed": duplicates, "cached": cached, **meta}
        series[role] = {"spec": spec, "rows": rows, "meta": meta}
        summaries.append(row_summary)
        warnings.extend(meta.get("warnings", []))
        tolerance = {"daily": 10, "weekly": 16, "monthly": 45}[h["frequency"]]
        if (date.fromisoformat(end) - date.fromisoformat(in_range[-1]["date"])).days > tolerance:
            warnings.append(f"{spec['symbol']} 最新可用观测仅到 {in_range[-1]['date']}，没有覆盖请求的结束日期。")
        if (date.fromisoformat(in_range[0]["date"]) - date.fromisoformat(h["start"])).days > tolerance:
            warnings.append(f"{spec['symbol']} 实际覆盖从 {in_range[0]['date']} 开始，早于此日期的数据不可用。")
    return {"series": series, "summary": summaries, "warnings": list(dict.fromkeys(warnings)), "demo": demo}


def frequency_series(item, frequency, end):
    frame = pd.DataFrame(item["rows"])
    s = pd.Series(frame["value"].to_numpy(dtype=float), index=pd.to_datetime(frame["date"])).sort_index()
    native = item.get("meta", {}).get("native_frequency", "auto")
    median_days = s.index.to_series().diff().dt.days.median()
    if (frequency == "daily" and (native in ("W", "BW", "M", "Q", "A", "SA") or median_days > 5)) or (frequency == "weekly" and (native in ("M", "Q", "A", "SA") or median_days > 10)):
        raise UserError("包含低频序列，不能填充成高频样本；请把假设频率改为月频或匹配原始频率")
    if frequency == "monthly" and (native in ("Q", "A", "SA") or median_days > 45):
        raise UserError("首版不支持把季度或年度序列展开成月频")
    if frequency != "daily":
        # Label an observation only at the completed period end, never before it is observed.
        rule = "W-FRI" if frequency == "weekly" else "ME"
        s = s.resample(rule).last()
        s = s[s.index <= pd.Timestamp(end)].dropna()
    return s


def transform(s, kind, periods):
    if kind == "level":
        return s
    if kind == "change":
        return s - s.shift(periods)
    if (s <= 0).any():
        raise UserError("百分比收益要求原始值为正；数据含零或负值（例如部分油价），请改用数值变化")
    return (s / s.shift(periods) - 1) * 100


def prepare(h, dataset):
    raw = {role: frequency_series(item, h["frequency"], h["end"]) for role, item in dataset["series"].items()}
    target = raw["target"]
    if h["y_transform"] == "return":
        if (target <= 0).any():
            raise UserError("结果序列含零或负值，不能计算百分比收益；请改为数值变化")
        y = (target.shift(-h["horizon"]) / target - 1) * 100
    else:
        y = target.shift(-h["horizon"]) - target
    # Calculate horizon and lag BEFORE joining calendars. Missing days never stretch a window.
    features = {"y": y, "x": transform(raw["signal"], h["x_transform"], h["lookback"]).shift(h["lag"])}
    for role, values in raw.items():
        if role.startswith("control"):
            features[role] = transform(values, h["x_transform"], h["lookback"]).shift(h["lag"])
    frame = pd.concat(features, axis=1, join="inner")
    candidate_count = len(frame.loc[h["start"]:h["end"]])
    frame = frame.loc[h["start"]:h["end"]].replace([np.inf, -np.inf], np.nan).dropna()
    if len(frame) < 40:
        raise UserError(f"对齐并计算窗口后只有 {len(frame)} 个有效样本，至少需要 40 个；请扩大区间或缩短窗口")
    if frame["x"].std() < 1e-12 or frame["y"].std() < 1e-12:
        raise UserError("变量几乎没有变化，无法进行有效统计检验")
    return frame, raw, candidate_count


def block_indices(n, size, rng):
    starts = rng.integers(0, n, math.ceil(n / size))
    return np.concatenate([(np.arange(size) + start) % n for start in starts])[:n]


def analyze(request):
    from scipy import stats
    import statsmodels.api as sm

    h, model, dataset = request["hypothesis"], request["model"], request["dataset"]
    frame, raw, candidates = prepare(h, dataset)
    n = len(frame)
    method, confidence, lags = model["method"], model["confidence"], model["hac_lags"]
    if lags < h["horizon"] or lags > 60 or lags >= n // 3:
        raise UserError("HAC 阶数需不小于观察窗口，且小于有效样本量的三分之一；请扩大区间或减少阶数")
    alpha = 1 - confidence
    warnings = list(dataset.get("warnings", []))
    warnings.append("仅检验本次确认的一个主假设；反复改阈值或模型后挑选显著结果，会夸大证据。")
    warnings.append("不同市场的收盘/发布时间不同；按日期和所选滞后对齐只能描述历史关联，不能证明因果或可交易性。")
    warnings.append("周/月频使用完整周期的最后一个有效观测；不填补缺失值，也不将未完成窗口计入样本。")
    controls = [c for c in frame if c.startswith("control")]
    group_info = None
    p_value = None
    bootstrap_info = None
    if method in ("event", "regression"):
        if method == "event":
            condition = frame["x"] >= h["threshold"] if h["operator"] == "ge" else frame["x"] <= h["threshold"]
            count = int(condition.sum())
            if min(count, n - count) < 15:
                raise UserError(f"条件组 {count} 个、对照组 {n-count} 个样本；每组至少需要 15 个，请扩大区间或调整条件")
            design = pd.DataFrame({"event": condition.astype(float)}, index=frame.index)
            group_info = {"event_count": count, "control_count": n-count,
                          "event_mean": float(frame.loc[condition, "y"].mean()), "control_mean": float(frame.loc[~condition, "y"].mean()),
                          "event_positive_rate": float((frame.loc[condition, "y"] > 0).mean()) * 100,
                          "control_positive_rate": float((frame.loc[~condition, "y"] > 0).mean()) * 100}
            effect_name = "条件组减去对照组的平均结果差"
        else:
            design = frame[["x"] + controls]
            effect_name = "信号每增加 1 单位对应的结果变化"
        design = sm.add_constant(design, has_constant="add")
        if np.linalg.matrix_rank(design.to_numpy()) < design.shape[1]:
            raise UserError("变量之间完全共线，请删除重复或恒定的控制变量")
        fit = sm.OLS(frame["y"], design).fit(cov_type="HAC", cov_kwds={"maxlags": lags, "use_correction": True}, use_t=True)
        effect = float(fit.params.iloc[1])
        ci = [float(v) for v in fit.conf_int(alpha=alpha).iloc[1]]
        p_value = float(fit.pvalues.iloc[1])
        model_detail = f"OLS + HAC({lags}) 稳健标准误；双侧 t 检验"
    elif method in ("pearson", "spearman"):
        if controls:
            raise UserError("相关分析不支持控制变量，请选回归")
        x, y = frame["x"].to_numpy(), frame["y"].to_numpy()
        def correlation(a, b):
            if method == "spearman":
                a, b = stats.rankdata(a), stats.rankdata(b)
            return float(np.corrcoef(a, b)[0, 1])
        effect = correlation(x, y)
        block = max(lags + 1, h["horizon"] + 1, int(round(n ** (1/3))))
        if block > n // 4:
            raise UserError("样本量不足以支持当前时间分块长度，请扩大区间或缩短窗口")
        rng = np.random.default_rng(20260905)
        replicates = []
        for _ in range(1500):
            ix = block_indices(n, block, rng)
            value = correlation(x[ix], y[ix])
            if math.isfinite(value):
                replicates.append(value)
        if len(replicates) < 1400:
            raise UserError("重采样退化，变量的有效变化不足")
        ci = [float(v) for v in np.quantile(replicates, [alpha / 2, 1 - alpha / 2])]
        effect_name = "Pearson 相关系数" if method == "pearson" else "Spearman 秩相关系数"
        model_detail = f"循环时间分块 Bootstrap，块长度 {block}，1500 次，百分位区间"
        bootstrap_info = {"block_length": block, "replicates": 1500, "seed": 20260905}
        warnings.append("相关模型使用分块重采样置信区间，不显示独立样本假设下的 p 值；区间仍依赖时间序列在区间内相对稳定。")
    else:
        raise UserError("不支持的内置模型")
    if not all(math.isfinite(v) for v in [effect, *ci]) or (p_value is not None and not math.isfinite(p_value)):
        raise UserError("数值计算退化，无法给出可靠置信区间")
    excludes_zero = ci[0] > 0 or ci[1] < 0
    expected = h["direction"] == "two_sided" or (effect > 0 if h["direction"] == "positive" else effect < 0)
    verdict = "当前样本支持假设方向" if excludes_zero and expected else ("当前样本显示相反方向" if excludes_zero else "证据不足")
    explanation = f"有效样本 {n} 个，{effect_name}为 {effect:.5g}，{confidence:.0%} 置信区间 [{ci[0]:.5g}, {ci[1]:.5g}]。"
    if p_value is not None:
        explanation += f" 双侧 p 值 {p_value:.4g}。"
    explanation += "该区间" + ("未跨过零。" if excludes_zero else "跨过零，不能据此确认存在差异或关系。")
    rows = [{"date": index.strftime("%Y-%m-%d"), **{k: float(v) for k, v in row.items()}} for index, row in frame.iterrows()]
    chart_series = []
    for role in ("target", "signal"):
        s = raw[role].loc[h["start"]:h["end"]]
        stride = max(1, math.ceil(len(s) / 500))
        chart_series.append({"role": role, "label": dataset["series"][role]["spec"].get("label") or role,
                             "points": [{"date": i.strftime("%Y-%m-%d"), "value": float(v)} for i, v in s.iloc[::stride].items()]})
    hist_counts, hist_edges = np.histogram(frame["y"], bins=min(30, max(8, int(math.sqrt(n)))))
    return {"verdict": verdict, "explanation": explanation, "effect": effect, "effect_name": effect_name,
            "confidence": confidence, "ci": ci, "p_value": p_value, "n": n, "candidate_count": candidates,
            "start": frame.index[0].strftime("%Y-%m-%d"), "end": frame.index[-1].strftime("%Y-%m-%d"),
            "model_detail": model_detail, "group": group_info, "bootstrap": bootstrap_info,
            "warnings": list(dict.fromkeys(warnings)), "demo": dataset.get("demo", False),
            "series": chart_series, "points": rows[::max(1, math.ceil(n/700))], "rows": rows,
            "histogram": {"counts": hist_counts.tolist(), "edges": hist_edges.tolist()}, "sources": dataset["summary"]}


def dispatch(request):
    if request.get("op") == "health":
        import scipy
        import statsmodels
        import yfinance
        return {"ok": True, "python": sys.version.split()[0], "numpy": np.__version__, "pandas": pd.__version__, "scipy": scipy.__version__, "statsmodels": statsmodels.__version__, "yfinance": yfinance.__version__}
    if request.get("op") == "fetch":
        return fetch(request)
    if request.get("op") == "analyze":
        return analyze(request)
    raise UserError("未知的内置工具")


if __name__ == "__main__":
    try:
        request = json.load(sys.stdin)
        with contextlib.redirect_stdout(sys.stderr):
            data = dispatch(request)
        response = {"ok": True, "data": data}
    except UserError as exc:
        response = {"ok": False, "error": str(exc)}
    except Exception as exc:
        # Do not emit provider payloads, request URLs, or credentials.
        response = {"ok": False, "error": f"内置工具无法处理本次输入（{type(exc).__name__}），请检查字段、日期与依赖版本"}
    json.dump(response, sys.stdout, ensure_ascii=False, allow_nan=False)

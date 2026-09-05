import sys
from pathlib import Path
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from analysis import engine as e
import numpy as np
import pandas as pd


def hypothesis(kind="relation"):
    spec = {"source": "csv", "symbol": "test", "label": "test", "field": "value"}
    return {"kind": kind, "target": spec, "signal": spec, "controls": [], "start": "2020-01-01", "end": "2024-12-31",
            "frequency": "daily", "x_transform": "change", "y_transform": "change", "lookback": 1, "horizon": 1,
            "lag": 0, "operator": "ge", "threshold": 0, "direction": "positive"}


def item(dates, values):
    return {"rows": [{"date": d.strftime("%Y-%m-%d"), "value": float(v)} for d,v in zip(dates,values)],
            "spec": {"symbol":"test","label":"test"}, "meta":{"native_frequency":"D"}}


def dataset(signal, target, dates):
    return {"series":{"signal":item(dates,signal),"target":item(dates,target)},"summary":[],"warnings":[],"demo":True}


class EngineTests(unittest.TestCase):
    def test_concurrent_returns_use_same_day_and_keep_last_day(self):
        dates = pd.bdate_range("2020-01-01", periods=180)
        x = np.arange(180, dtype=float) ** 2 + 100
        y = np.arange(180, dtype=float) ** 3 + 200
        h = hypothesis()
        h.update(timing="concurrent", x_transform="return", y_transform="return")
        frame, _, _ = e.prepare(h, dataset(x, y, dates))
        self.assertAlmostEqual(frame.loc[dates[50], "x"], (x[50] / x[49] - 1) * 100)
        self.assertAlmostEqual(frame.loc[dates[50], "y"], (y[50] / y[49] - 1) * 100)
        self.assertIn(dates[-1], frame.index)
        h["timing"] = "forward"
        future, _, _ = e.prepare(h, dataset(x, y, dates))
        self.assertAlmostEqual(future.loc[dates[50], "y"], (y[51] / y[50] - 1) * 100)
        self.assertNotIn(dates[-1], future.index)
        del h["timing"]
        legacy, _, _ = e.prepare(h, dataset(x, y, dates))
        pd.testing.assert_frame_equal(future, legacy)

    def test_concurrent_missing_dates_reject_mismatched_windows_and_controls(self):
        dates = pd.bdate_range("2020-01-01", periods=180)
        values = np.arange(180, dtype=float) ** 2 + 100
        for role in ("signal", "control1"):
            data = dataset(values, values * 2, dates)
            data["series"]["control1"] = item(dates, values * 3)
            data["series"][role]["rows"].pop(80)
            h = hypothesis()
            h.update(timing="concurrent", horizon=3, lookback=3)
            frame, _, _ = e.prepare(h, data)
            for day in dates[80:84]:
                self.assertNotIn(day, frame.index)
            self.assertIn(dates[84], frame.index)
            self.assertAlmostEqual(frame.loc[dates[84], "y"], 2 * (values[84] - values[81]))

    def test_concurrent_detects_simultaneous_relation_and_regime_comparisons(self):
        rng = np.random.default_rng(17)
        dates = pd.bdate_range("2020-01-01", periods=450)
        x = rng.normal(size=len(dates))
        y = 2 * x + rng.normal(0, .1, len(dates))
        data = dataset(100 + np.cumsum(x), 300 + np.cumsum(y), dates)
        h = hypothesis()
        h.update(timing="concurrent", flat_band=.2)
        model = {"method":"pearson", "confidence":.95, "hac_lags":5}
        out = e.analyze({"hypothesis":h, "model":model, "dataset":data})
        self.assertGreater(out["ci"][0], .98)
        groups = out["comparison"]["groups"]
        self.assertEqual(sum(g["count"] for g in groups), out["n"])
        self.assertTrue(any("同期" in w for w in out["warnings"]))
        h["timing"] = "forward"
        future = e.analyze({"hypothesis":h, "model":model, "dataset":data})
        self.assertLess(abs(future["effect"]), .15)

    def test_concurrent_rejects_zero_window_or_lag(self):
        dates = pd.bdate_range("2020-01-01", periods=100)
        values = np.arange(100, dtype=float) ** 2 + 100
        for change in ({"horizon":0}, {"lag":1}, {"lookback":2}):
            h = hypothesis()
            h.update(timing="concurrent", **change)
            with self.assertRaisesRegex(e.UserError, "同期"):
                e.prepare(h, dataset(values, values, dates))

    def test_conflicting_dates_fail(self):
        with self.assertRaisesRegex(e.UserError,"冲突"):
            e.normalize([{"date":"2020-01-01","value":1},{"date":"2020-01-01","value":2}],"2020-01-01","2024-01-01")

    def test_csv_invalid_columns(self):
        with self.assertRaisesRegex(e.UserError,"需要"):
            e.csv_data({"field":"value"},"date,close\n2020-01-01,20")

    def test_calendar_join_does_not_extend_horizon(self):
        dates=pd.bdate_range("2020-01-01",periods=160)
        target=np.arange(160,dtype=float)**2+100
        signal=np.sin(np.arange(160)) + np.arange(160)
        data=dataset(signal,target,dates)
        missing_day=dates[80]
        data["series"]["signal"]["rows"]=[r for r in data["series"]["signal"]["rows"] if r["date"]!=missing_day.strftime("%Y-%m-%d")]
        frame,_,_=e.prepare(hypothesis(),data)
        self.assertAlmostEqual(frame.loc[dates[79],"y"],target[80]-target[79])
        self.assertNotIn(dates[-1],frame.index)

    def test_signal_lag_never_uses_future_value(self):
        dates=pd.bdate_range("2020-01-01",periods=160)
        values=np.arange(160,dtype=float)**2+100
        data=dataset(values,values,dates);h=hypothesis();h["lag"]=2
        frame,_,_=e.prepare(h,data)
        self.assertEqual(frame.loc[dates[50],"x"],values[48]-values[47])

    def test_low_frequency_not_inflated(self):
        dates=pd.date_range("2020-01-01",periods=48,freq="MS")
        data=item(dates,np.arange(48)+100);data["meta"]["native_frequency"]="M"
        with self.assertRaisesRegex(e.UserError,"低频"):
            e.frequency_series(data,"daily","2024-01-01")

    def test_incomplete_month_not_used(self):
        dates=pd.bdate_range("2020-01-01","2020-03-10")
        monthly=e.frequency_series(item(dates,np.arange(len(dates))+100),"monthly","2020-03-10")
        self.assertEqual(monthly.index[-1],pd.Timestamp("2020-02-29"))

    def test_nonpositive_returns_rejected(self):
        with self.assertRaisesRegex(e.UserError,"为正"):
            e.transform(pd.Series([20,0,-10]),"return",1)

    def test_known_event_effect_and_reproducibility(self):
        rng=np.random.default_rng(123);n=650;dates=pd.bdate_range("2020-01-01",periods=n)
        x=rng.normal(size=n);outcome=2*(x>=0)+rng.normal(0,.3,n)
        # Y(t+1)-Y(t) = outcome(t); X is explicitly a level condition.
        target=100+np.concatenate([[0],np.cumsum(outcome[:-1])])
        data=dataset(x,target,dates);h=hypothesis("event");h["x_transform"]="level"
        req={"hypothesis":h,"model":{"method":"event","confidence":.95,"hac_lags":5},"dataset":data}
        out=e.analyze(req)
        self.assertAlmostEqual(out["effect"],2,delta=.1)
        self.assertGreater(out["ci"][0],1.8)
        self.assertLess(out["p_value"],.01)
        self.assertEqual(out["group"]["event_count"]+out["group"]["control_count"],out["n"])

    def test_correlation_block_interval_and_no_iid_pvalue(self):
        rng=np.random.default_rng(7);n=350;dates=pd.bdate_range("2020-01-01",periods=n)
        x=rng.normal(size=n);y=100+np.concatenate([[0],np.cumsum(x[:-1]+rng.normal(0,.3,n-1))])
        data=dataset(x,y,dates);h=hypothesis();h["x_transform"]="level"
        req={"hypothesis":h,"model":{"method":"pearson","confidence":.95,"hac_lags":5},"dataset":data}
        first=e.analyze(req);second=e.analyze(req)
        self.assertGreater(first["ci"][0],.8)
        self.assertEqual(first["ci"],second["ci"])
        self.assertIsNone(first["p_value"])

    def test_fred_missing_observations_and_key_not_returned(self):
        responses=[{"seriess":[{"title":"Oil","units":"USD","frequency_short":"D"}]},
                   {"count":2,"observations":[{"date":"2020-01-01","value":"."},{"date":"2020-01-02","value":"20"}]}]
        with patch.dict(e.os.environ,{"FRED_API_KEY":"private-fred-key"}),patch.object(e,"get_json",side_effect=responses) as get:
            rows,meta=e.fred({"symbol":"DCOILWTICO"},"2020-01-01","2020-02-01")
            self.assertEqual(rows,[{"date":"2020-01-02","value":20.0}])
            self.assertNotIn("private-fred-key",str(meta))
            self.assertEqual(get.call_args_list[0].args[1]["api_key"],"private-fred-key")

    def test_sparse_event_fails(self):
        n=100;dates=pd.bdate_range("2020-01-01",periods=n);data=dataset(np.arange(n),np.arange(n)**2,dates)
        h=hypothesis("event");h["x_transform"]="level";h["threshold"]=95
        with self.assertRaisesRegex(e.UserError,"每组至少"):
            e.analyze({"hypothesis":h,"model":{"method":"event","confidence":.95,"hac_lags":5},"dataset":data})

    def test_regression_recovers_effect_with_control(self):
        rng=np.random.default_rng(91);n=500;dates=pd.bdate_range("2020-01-01",periods=n)
        x=rng.normal(size=n);control=.5*x+rng.normal(size=n)
        changes=1.5*x+3*control+rng.normal(0,.2,n)
        y=100+np.concatenate([[0],np.cumsum(changes[:-1])])
        data=dataset(x,y,dates);data["series"]["control1"]=item(dates,control)
        h=hypothesis();h["x_transform"]="level";h["controls"]=[h["signal"]]
        out=e.analyze({"hypothesis":h,"model":{"method":"regression","confidence":.95,"hac_lags":5},"dataset":data})
        self.assertAlmostEqual(out["effect"],1.5,delta=.06)
        self.assertGreater(out["ci"][0],1.4)

    def test_duplicate_control_rejected(self):
        dates=pd.bdate_range("2020-01-01",periods=100);values=np.arange(100)**2+100
        data=dataset(values,values,dates);data["series"]["control1"]=item(dates,values)
        with self.assertRaisesRegex(e.UserError,"共线"):
            e.analyze({"hypothesis":hypothesis(),"model":{"method":"regression","confidence":.95,"hac_lags":5},"dataset":data})

    def test_regime_boundaries_cover_all_samples_and_reconcile(self):
        x = np.tile([-2., -1., 0., 1., 2.], 40)
        y = np.arange(len(x), dtype=float) - 80
        frame = pd.DataFrame({"x": x, "y": y})
        h = hypothesis(); h["flat_band"] = 1
        result = e.compare_regimes(frame, h, .95, 5)
        groups = {g["id"]: g for g in result["groups"]}
        self.assertEqual([groups[k]["count"] for k in ("up", "flat", "down")], [40, 120, 40])
        self.assertAlmostEqual(sum(g["mean"] * g["count"] for g in groups.values()) / len(x), y.mean())
        for key, mask in {"up": x > 1, "flat": abs(x) <= 1, "down": x < -1}.items():
            g = groups[key]
            self.assertAlmostEqual(g["median"], np.median(y[mask]))
            self.assertAlmostEqual(g["negative_rate"], np.mean(y[mask] < 0) * 100)
            self.assertAlmostEqual(g["positive_rate"] + g["negative_rate"] + g["zero_rate"], 100)

    def test_regime_hac_interval_matches_independent_sandwich_calculation(self):
        from scipy.stats import t
        rng = np.random.default_rng(551)
        n, lags = 300, 5
        x = rng.choice([-1., 0., 1.], n)
        noise = rng.normal(size=n)
        for i in range(1, n): noise[i] += .6 * noise[i-1]
        y = -1.5 * x + noise
        frame = pd.DataFrame({"x": x, "y": y})
        h = hypothesis(); h["flat_band"] = .1
        result = e.compare_regimes(frame, h, .95, lags)
        design = np.column_stack([x > .1, abs(x) <= .1, x < -.1]).astype(float)
        bread = np.linalg.inv(design.T @ design)
        means = bread @ design.T @ y
        scores = design * (y - design @ means)[:, None]
        meat = scores.T @ scores
        for lag in range(1, lags + 1):
            cross = scores[lag:].T @ scores[:-lag]
            meat += (1 - lag / (lags + 1)) * (cross + cross.T)
        cov = bread @ meat @ bread * n / (n - 3)
        for pair, contrast in zip(result["pairs"], [[1,-1,0], [1,0,-1], [0,1,-1]]):
            c = np.array(contrast)
            effect = c @ means
            se = np.sqrt(c @ cov @ c)
            delta = t.ppf(1 - .05 / 6, n - 3) * se
            np.testing.assert_allclose(pair["ci"], [effect-delta, effect+delta], atol=1e-10)
            self.assertAlmostEqual(pair["p_adjusted"], min(1, 6*t.sf(abs(effect/se), n-3)))
            self.assertGreater(delta, t.ppf(.975, n-3) * se)

    def test_missing_and_sparse_regimes_are_not_reported_as_zero(self):
        rng = np.random.default_rng(552)
        for flat_count in (0, 4):
            x = np.r_[np.ones(80), np.zeros(flat_count), -np.ones(80)]
            frame = pd.DataFrame({"x": x, "y": -x + rng.normal(0, .3, len(x))})
            result = e.compare_regimes(frame, hypothesis(), .95, 5)
            flat = result["groups"][1]
            self.assertEqual(flat["count"], flat_count)
            if flat_count == 0:
                self.assertIsNone(flat["mean"])
                self.assertIsNone(flat["negative_rate"])
            for p in (result["pairs"][0], result["pairs"][2]):
                self.assertIsNone(p["ci"])
                self.assertIsNone(p["p_adjusted"])
                self.assertIn("样本不足", p["status"])
            self.assertLess(result["pairs"][1]["ci"][1], 0)
            e.json.dumps(result, allow_nan=False)

    def test_level_is_not_mislabelled_as_rising(self):
        h = hypothesis(); h["x_transform"] = "level"
        result = e.compare_regimes(pd.DataFrame({"x": [50,60], "y": [1,2]}), h, .95, 5)
        self.assertFalse(result["available"])
        self.assertIn("不能把高价叫作上涨", result["note"])

    def test_weaker_positive_returns_are_not_called_a_fall(self):
        rng = np.random.default_rng(553); n = 650
        dates = pd.bdate_range("2020-01-01", periods=n)
        x = rng.normal(size=n)
        outcome = 2 - (x >= 0) + rng.normal(0, .1, n)
        target = 100 + np.r_[0, np.cumsum(outcome[:-1])]
        data = dataset(np.cumsum(x), target, dates)
        h = hypothesis("event"); h["direction"] = "negative"; h["flat_band"] = .2
        out = e.analyze({"hypothesis":h, "model":{"method":"event","confidence":.95,"hac_lags":5}, "dataset":data})
        self.assertEqual(out["verdict_code"], "supported")
        self.assertGreater(out["group"]["event_mean"], 0)
        self.assertLess(out["effect"], 0)
        self.assertIn("平均结果并未下跌", "".join(out["takeaways"]))
        self.assertEqual(sum(g["count"] for g in out["comparison"]["groups"]), out["n"])
        e.json.dumps(out, allow_nan=False)

    def test_verdict_distinguishes_inconclusive_opposite_and_two_sided(self):
        h = hypothesis(); c = {"available":False,"note":"原水平"}
        code, _, details = e.conclusion(h, "regression", -.01, [-.1,.1], None, c, "%", "个百分点/X 单位")
        self.assertEqual(code, "inconclusive")
        self.assertIn("不等于证明没有关系", "".join(details))
        code, _, details = e.conclusion(h, "regression", -.2, [-.3,-.1], None, c, "%", "个百分点/X 单位")
        self.assertEqual(code, "opposite")
        self.assertIn("0.2个百分点", details[0])
        h["direction"] = "two_sided"
        code, verdict, _ = e.conclusion(h, "regression", -.2, [-.3,-.1], None, c, "%", "个百分点/X 单位")
        self.assertEqual(code, "supported")
        self.assertIn("存在差异或关系", verdict)


if __name__=="__main__":
    unittest.main()

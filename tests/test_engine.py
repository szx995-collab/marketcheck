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


if __name__=="__main__":
    unittest.main()

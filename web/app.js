import {lineChart, scatterChart, histogramChart} from './charts.js';

const $ = (s, scope = document) => scope.querySelector(s);
const esc = x => String(x ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const fmt = (x, digits = 4) => x == null ? '—' : Number(x).toLocaleString('zh-CN', {maximumFractionDigits: digits});
const roles = {target:'结果标的 Y', signal:'解释变量 X', control1:'控制变量 1', control2:'控制变量 2'};
const stepNames = ['提出假设', '澄清与确认', '获取数据', '选择模型', '检验结果'];
const labels = {event:'条件均值比较', pearson:'Pearson 相关', spearman:'Spearman 秩相关', regression:'线性回归'};
const state = {step:0, original:'', answers:{}, questions:[], message:'', draft:null, run:null, settings:{}, catalog:[], history:[], uploads:{}, model:null, busy:false};
let noticeTimer;
const button = (text, action, secondary = false, extra = '') => {
  const exports={'download-report':'report','download-raw':'raw.csv','download-analysis':'analysis.csv'};
  if(exports[action] && state.run) return `<a class="button${secondary?' secondary':''}" href="/api/runs/${esc(state.run.id)}/export/${exports[action]}" download>${text}</a>`;
  return `<button type="button" class="button${secondary ? ' secondary' : ''}" data-action="${action}" ${extra}>${text}</button>`;
};
const select = (name, value, options) => `<select name="${name}">${options.map(([v,l])=>`<option value="${esc(v)}" ${v===value?'selected':''}>${esc(l)}</option>`).join('')}</select>`;
const input = (name, value, type='text', extra='') => `<input name="${name}" type="${type}" value="${esc(value)}" ${extra}>`;
const heading = (eyebrow,title,text,action='') => `<div class="page-heading"><div><span class="eyebrow">${eyebrow}</span><h1>${title}</h1><p>${text}</p></div>${action}</div>`;
const warnings = values => (values || []).map(w=>`<div class="warning">${esc(w)}</div>`).join('');
const demoBanner = () => state.run?.data?.demo ? '<div class="warning demo">合成演示数据 · 本次结果不代表真实市场。新建检验并获取真实数据后再判断假设。</div>' : '';

async function api(path, body) {
  const res = await fetch(`/api/${path}`, body === undefined ? {} : {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  let data;
  try { data = await res.json(); } catch { throw new Error('服务连接中断，请确认启动窗口仍在运行。'); }
  if (!res.ok) throw new Error(data.error || '请求失败，请重试。');
  return data;
}
function notice(text, kind='') {
  clearTimeout(noticeTimer);
  const el=$('#notice'); el.textContent=text; el.className=`notice ${kind}`; el.hidden=false;
  if(kind!=='loading') noticeTimer=setTimeout(()=>el.hidden=true,9000);
}
async function work(message, fn) {
  if(state.busy) return;
  state.busy=true; notice(message,'loading'); syncBusy();
  try { await fn(); $('#notice').hidden=true; }
  catch(error) { notice(error.message,'error'); }
  finally {state.busy=false;syncBusy();}
}
function syncBusy(){document.querySelectorAll('button').forEach(b=>{if(b.dataset.action!=='close-settings')b.disabled=state.busy;});}
function pick(source,symbol,label){return {source,symbol,label,field:'value'};}
function defaultDraft(){
  const end=new Date();end.setUTCDate(end.getUTCDate()-1);
  const start=new Date(end);start.setUTCFullYear(start.getUTCFullYear()-4);
  return {original:state.original,kind:'event',target:pick('tencent','sh000300','沪深300'),signal:pick('tencent','sh000300','沪深300'),controls:[],start:start.toISOString().slice(0,10),end:end.toISOString().slice(0,10),frequency:'daily',x_transform:'return',y_transform:'return',lookback:1,horizon:1,lag:0,operator:'le',threshold:-1,direction:'positive'};
}
function normalizeDraft(d){
  const base=defaultDraft();
  const result={...base,...d,target:{...base.target,...d.target},signal:{...base.signal,...d.signal},controls:d.controls || []};
  result.controls=result.controls.slice(0,2).map(s=>({...s,field:s.field || 'value'}));
  return result;
}
function renderHistory(){
  $('#history-count').textContent=state.history.length || '';
  $('#history').innerHTML=state.history.length ? state.history.map(r=>`<button class="history-item" data-action="history" data-id="${esc(r.id)}">${esc(r.summary.slice(0,47))}…<small>${esc(r.created.slice(0,10))} · ${r.status==='complete'?'已完成':r.status==='failed'?'待重试':'进行中'}</small></button>`).join('') : '<p class="empty-history">每一次认真提问，<br>都会保存在这里。</p>';
}
function renderConnection(){
  const s=state.settings;const ok=s.provider==='openai'?s.openai_configured:s.deepseek_configured;
  $('#connection').textContent=`${ok?'●':'○'} ${s.provider==='openai'?'GPT':'DeepSeek'}${ok?' 已配置':' 待配置'}`;
}
function render(){
  $('#steps').innerHTML=stepNames.map((name,i)=>`${i?'<span class="step-line"></span>':''}<button class="step ${i===state.step?'active':i<state.step?'done':''}" data-action="step" data-step="${i}"><i>${i<state.step?'✓':i+1}</i>${name}</button>`).join('');
  $('#app').innerHTML=[homeView,draftView,dataView,modelView,resultView][state.step]();
  renderHistory();renderConnection();syncBusy();
}
function homeView(){return `${heading('FROM INTUITION TO EVIDENCE','把市场直觉，变成可检验的假设','写下你想验证的市场现象。AI 会帮你明确问题、选择数据和方法，再由内置工具计算结果。')}
  <section class="card input-card"><div class="card-top"><h2>你想检验什么？</h2><span class="pill">A 股 / 美股 / 黄金 / 原油 / 宏观</span></div>
  <textarea id="original" class="hero-input" maxlength="6000" placeholder="例如：美债收益率上升以后，黄金价格是不是更容易下跌？">${esc(state.original)}</textarea>
  <div class="input-footer"><span class="muted">不必一开始就想清所有细节，下一步会逐项澄清。</span><div class="actions" style="margin:0">${button('手动填写','manual',true)}${button('让 AI 澄清假设 <span>→</span>','clarify')}</div></div></section>
  <div class="sample-heading">或者，从一个问题开始<span></span></div><div class="samples">
  <button class="sample" data-action="example" data-example="0"><small>A 股 · 条件比较</small><b>沪深300 大跌之后，<br>短期反弹是否更明显？</b><span>↗</span></button>
  <button class="sample" data-action="example" data-example="1"><small>黄金 · 宏观关系</small><b>美债收益率上升，<br>黄金是否随后下跌？</b><span>↗</span></button>
  <button class="sample" data-action="example" data-example="2"><small>原油 · 跨市场关系</small><b>WTI 油价的变化，<br>与美股后续收益有关吗？</b><span>↗</span></button></div>
  <div class="how-it-works"><div><b>01 &nbsp; 先把问题说清楚</b>歧义变成选项，关键定义由你确认。</div><div><b>02 &nbsp; 数据与方法可选择</b>查看实际数据，再确认数学模型。</div><div><b>03 &nbsp; 让结果有据可依</b>查看效应、区间和图表，保留每次检验。</div></div>
  <div class="demo-row">还没配置 Key？${button('用合成数据体验完整计算 →','demo',true)}</div>`;}

function seriesEditor(role,spec){
  const preset=state.catalog.findIndex(c=>c.source===spec.source&&c.symbol===spec.symbol);
  const note=preset>=0?state.catalog[preset].note:'自定义代码：沪深股票 sh/sz 前缀；美股 Yahoo 代码；宏观填写 FRED 系列 ID。';
  return `<div class="series-box"><h3>${roles[role]}</h3><label>快捷选择${select(`${role}_preset`,String(preset),[['-1','自定义标的 / CSV'],...state.catalog.map((c,i)=>[String(i),`${c.group} · ${c.label}`])])}</label>
  <div class="form-grid"><label>数据源${select(`${role}_source`,spec.source,[['tencent','腾讯 · 沪深行情'],['yahoo','Yahoo · 美股/期货'],['fred','FRED · 宏观/油价'],['csv','导入 CSV']])}</label><label>代码${input(`${role}_symbol`,spec.symbol)}</label></div>
  <div class="form-grid"><label>显示名称${input(`${role}_label`,spec.label)}</label><label>字段${select(`${role}_field`,spec.field || 'value',[['value','价格 / 数值'],['volume','成交量']])}</label></div><p class="muted">${esc(note)}</p></div>`;
}
function draftView(){
  const h=state.draft || defaultDraft();
  return `${heading('DEFINE THE QUESTION','把每一个关键定义确认下来','选项是建议，你可以直接修改。最终以此表单为准；返回修改并再次确认会建立新的检验。')}
  ${state.questions.length?`<section class="card"><div class="card-top"><h2>还需要明确 ${state.questions.length} 个问题</h2><span class="pill">AI 澄清</span></div><p class="muted">${esc(state.message)}</p>${state.questions.map((q,i)=>`<div class="question"><h3>${i+1}. ${esc(q.text)}</h3><div class="options">${(q.options || []).map(opt=>`<button class="option ${state.answers[q.id]===opt?'selected':''}" data-action="answer" data-id="${esc(q.id)}" data-value="${esc(opt)}">${esc(opt)}</button>`).join('')}</div><input data-answer="${esc(q.id)}" value="${esc(state.answers[q.id] || '')}" placeholder="也可以输入自己的定义"></div>`).join('')}<div class="actions">${button('提交回答，更新假设','clarify-again')}</div></section>`:''}
  <section class="card"><div class="card-top"><h2>可检验的假设</h2><span class="micro">所有数值均可编辑</span></div><div id="draft-form">
  <div class="form-grid"><label>问题类型${select('kind',h.kind,[['event','条件出现后，与其余样本比较'],['relation','两个变量的相关 / 回归关系']])}</label><label>预期方向${select('direction',h.direction,[['positive','正向关系 / 条件组更高'],['negative','负向关系 / 条件组更低'],['two_sided','双向差异，不预设方向']])}</label></div>
  <div class="series-grid" style="margin-top:20px">${seriesEditor('target',h.target)}${seriesEditor('signal',h.signal)}</div>
  <div class="actions" style="margin-top:10px">${button('X 使用与 Y 相同的标的','same-series',true)}</div>
  <div class="form-grid three"><label>开始日期${input('start',h.start,'date')}</label><label>结束日期（最晚昨天）${input('end',h.end,'date')}</label><label>分析频率${select('frequency',h.frequency,[['daily','日频 · 各自的观测日'],['weekly','周频 · 完整周末值'],['monthly','月频 · 完整月末值']])}</label></div>
  <div class="form-grid three"><label>X 使用什么数值${select('x_transform',h.x_transform,[['return','涨跌幅（%）'],['change','原单位变化'],['level','原数值水平']])}</label><label>X 回看多少期${input('lookback',h.lookback,'number','min="1" max="60"')}</label><label>X 额外滞后多少期${input('lag',h.lag,'number','min="0" max="20"')}</label></div>
  <div class="form-grid three"><label>Y 观察什么结果${select('y_transform',h.y_transform,[['return','未来收益率（%）'],['change','未来原单位变化']])}</label><label>Y 未来多少期${input('horizon',h.horizon,'number','min="1" max="20"')}</label><label>频率口径<input value="周/月频取期末值；不填充缺失" disabled></label></div>
  ${h.kind==='event'?`<div class="form-grid"><label>触发条件：X${select('operator',h.operator,[['le','小于或等于'],['ge','大于或等于']])}</label><label>阈值（涨跌幅 -3 表示 -3%）${input('threshold',h.threshold,'number','step="any"')}</label></div><p class="muted">对照组为同区间、不满足条件的其余有效观测；比较的是平均结果。</p>`:`<details ${h.controls.length?'open':''}><summary>控制变量（可选，最多两个；加入后使用回归）</summary><div class="series-grid">${h.controls.map((s,i)=>seriesEditor(`control${i+1}`,s)).join('')}</div><div class="actions">${h.controls.length<2?button('＋ 添加控制变量','add-control',true):''}${h.controls.length?button('移除最后一个','remove-control',true):''}</div><p class="muted">控制变量使用与 X 相同的变换、回看窗口和滞后期。</p></details>`}
  </div><div class="summary-box">${esc(draftSummary(h))}</div><p class="muted">请核对：黄金期货、黄金 ETF 与现货金价不同；利率的“变化”单位为百分点。跨市场结果只解释历史关联。</p>
  <div class="actions">${button('返回输入','back-home',true)}${button('确认这条假设，继续 →','confirm-hypothesis')}</div></section>`;
}
function draftSummary(h){
  const f={daily:'观测日',weekly:'周',monthly:'月'}[h.frequency];
  const x={return:'涨跌幅(%)',change:'原单位变化',level:'数值水平'}[h.x_transform];
  const y=h.y_transform==='return'?'收益率(%)':'原单位变化';
  return `${h.start} 至 ${h.end}，使用 ${h.signal.label || h.signal.symbol} 的 ${h.lookback} ${f}${x}，额外滞后 ${h.lag} ${f}，检验 ${h.target.label || h.target.symbol} 随后 ${h.horizon} ${f}${y}。${h.kind==='event'?`条件：X ${h.operator==='le'?'≤':'≥'} ${h.threshold}；与不满足条件的样本比较。`:'检验变量之间的关系。'}`;
}
function readDraft(){
  const form=$('#draft-form');if(!form)return state.draft;
  const h={...state.draft};const value=name=>form.querySelector(`[name="${name}"]`)?.value;
  for(const key of ['kind','direction','start','end','frequency','x_transform','y_transform','operator'])if(value(key)!==undefined)h[key]=value(key);
  for(const key of ['lookback','horizon','lag','threshold'])if(value(key)!==undefined)h[key]=Number(value(key));
  function readSpec(role){return Object.fromEntries(['source','symbol','label','field'].map(k=>[k,value(`${role}_${k}`)?.trim() || '']));}
  h.target=readSpec('target');h.signal=readSpec('signal');h.controls=(h.controls || []).map((_,i)=>readSpec(`control${i+1}`));
  if(h.kind==='event')h.controls=[];
  h.original=state.original;state.draft=h;return h;
}

function dataView(){
  const r=state.run;if(!r)return '<p>请先确认假设。</p>';
  const d=r.data;const required=[r.hypothesis.target,r.hypothesis.signal,...(r.hypothesis.controls || [])];
  const csvs=[...new Map(required.filter(s=>s.source==='csv').map(s=>[s.symbol,s])).values()];
  return `${heading('CONNECT THE EVIDENCE','获取本次检验所需的数据','程序自动下载已接入来源的数据。先检查真实覆盖范围和单位，再选择检验方法。')}${demoBanner()}<div class="summary-box">${esc(r.summary)}</div>
  <section class="card"><div class="card-top"><h2>数据需求</h2><span class="pill">${r.hypothesis.start} — ${r.hypothesis.end}</span></div><div class="table-wrap"><table><thead><tr><th>用途</th><th>标的</th><th>数据源</th><th>字段</th></tr></thead><tbody>${required.map((s,i)=>`<tr><td>${i===0?'结果 Y':i===1?'信号 X':'控制变量'}</td><td>${esc(s.label)} · ${esc(s.symbol)}</td><td>${esc(s.source)}</td><td>${esc(s.field)}</td></tr>`).join('')}</tbody></table></div>
  ${required.some(s=>s.source==='fred')?`<p class="muted">FRED：${state.settings.fred_configured?'已配置 Key':'尚未配置 Key，请打开右上角连接设置'}。行情数据会缓存 12 小时；可以直接重试失败步骤。</p>`:''}
  ${csvs.map(s=>`<label class="upload-box">上传 ${esc(s.label || s.symbol)} 的 CSV（date,value，成交量使用 volume）<input type="file" accept=".csv,text/csv" data-upload="${esc(s.symbol)}"><span class="muted">${state.uploads[s.symbol]?'已载入文件，可继续获取数据':'UTF-8，日期 YYYY-MM-DD；保留足够的前置数据用于回看窗口。'}</span></label>`).join('')}
  <div class="status-line ${r.status.endsWith('running')?'running':''}">${esc(r.message)}</div><div class="actions">${button('修改假设','edit-hypothesis',true)}${button(d?'重新获取数据':'获取数据 →','fetch-data')}</div></section>
  ${d?`<section class="card"><div class="card-top"><h2>实际数据覆盖</h2><span class="micro">前置数据用于窗口计算，不计入检验区间</span></div><div class="data-grid">${d.summary.map(s=>`<div class="data-card"><h3>${esc(roles[s.role])} · ${esc(s.label)}</h3><div class="numbers">${fmt(s.count,0)}<small>条观测</small></div><div class="muted">${esc(s.start)} — ${esc(s.end)}</div><div class="metadata"><span>${esc(s.provider)}</span><span>${esc(s.units)}</span><span>${esc(s.adjustment)}</span><span>${s.cached?'12 小时内缓存':'本次读取'} · 前置 ${s.warmup_count} 条</span></div></div>`).join('')}</div>
  ${warnings(d.warnings)}<details><summary>查看原始数据预览（每个序列最后 6 行）</summary>${Object.entries(d.series).map(([role,s])=>`<p class="muted">${esc(roles[role])} · ${esc(s.spec.symbol)}</p>${dataTable(s.rows.slice(-6),['date','value'])}`).join('')}</details>
  <div class="actions">${button('下载原始数据 CSV','download-raw',true)}${button('数据已核对，选择模型 →','to-model')}</div></section>`:''}`;
}

function recommendedDefault(){const h=state.run.hypothesis;return {method:h.kind==='event'?'event':h.controls?.length?'regression':'spearman',confidence:.95,hac_lags:Math.max(5,h.horizon),reason:''};}
function modelView(){
  const r=state.run;if(!r?.data)return '<p>请先获取数据。</p>';
  const m=state.model || recommendedDefault();
  const descriptions={event:'比较触发条件与其余样本的平均结果，使用 HAC 稳健标准误。',pearson:'衡量线性相关程度，置信区间采用时间分块重采样。',spearman:'衡量单调关系，对极端值较不敏感；采用时间分块重采样。',regression:'估计 X 与 Y 的线性关系，可加入最多两个控制变量。'};
  const choices=r.hypothesis.kind==='event'?['event']:(r.hypothesis.controls?.length?['regression']:['spearman','pearson','regression']);
  return `${heading('CHOOSE THE METHOD','确认数学模型，再开始计算','AI 可以推荐方法，你也可以直接选择。确认前不会运行统计检验。')}${demoBanner()}<div class="summary-box">${esc(r.summary)}</div>
  <section class="card"><div class="card-top"><h2>检验方法</h2>${button('让 AI 推荐模型','recommend',true)}</div>
  ${r.recommendation?`<div class="summary-box"><strong>AI 推荐：${esc(labels[r.recommendation.method])}</strong><br>${esc(r.recommendation.reason)}</div>`:'<p class="muted">当前为程序默认选择。可以让 AI 基于假设和数据摘要给出推荐。</p>'}
  <div class="model-choices">${choices.map(v=>`<button class="model-choice ${m.method===v?'active':''}" data-action="choose-model" data-method="${v}"><strong>${esc(labels[v])} ${m.method===v?'✓':''}</strong><p>${descriptions[v]}</p></button>`).join('')}</div>
  <div id="model-form" class="form-grid"><label>置信水平${select('confidence',String(m.confidence),[['0.9','90%'],['0.95','95%'],['0.99','99%']])}</label><label>时间依赖阶数（至少 ${r.hypothesis.horizon}）${input('hac_lags',m.hac_lags,'number',`min="${r.hypothesis.horizon}" max="60"`)}</label></div>
  <p class="muted">回归/条件比较用于 HAC 阶数；相关模型用来设置不小于此阶数 + 1 的分块长度。检验为双侧；假设方向单独与结果核对。</p>
  ${r.hypothesis.controls?.length?'<p class="muted">控制变量与 X 使用相同变换、回看窗口和滞后期。</p>':''}
  <div class="warning">置信区间不是“假设为真的概率”。重复挑选阈值或模型会影响证据强度；本次只计算已确认的主检验。</div>
  ${r.status==='failed'?`<div class="error-box">${esc(r.message)}</div>`:''}<div class="actions">${button('返回数据','back-data',true)}${button('确认模型，运行检验 →','analyze')}</div></section>`;
}
function readModel(){
  const m={...(state.model || recommendedDefault())};
  if($('#model-form')){m.confidence=Number($('[name=confidence]').value);m.hac_lags=Number($('[name=hac_lags]').value);}
  state.model=m;return m;
}

function resultView(){
  const r=state.run, d=r?.result;if(!d)return '<p>请先确认模型并运行检验。</p>';
  const sourceNames=d.sources.map(s=>s.label).join(' / ');
  return `${heading('READ THE EVIDENCE','检验完成，看看数据怎么说',`${esc(sourceNames)} · ${d.start} — ${d.end}`,button('导出报告 ↗','download-report',true))}${demoBanner()}
  <div class="result-intro"><span class="eyebrow">${esc(labels[r.model.method])} · ${fmt(d.confidence*100,0)}% 置信水平</span><h2 style="margin-top:12px">${esc(d.verdict)}</h2><p>${esc(d.explanation)}</p></div>
  <div class="metrics"><div class="metric"><div class="label">有效样本</div><div class="value">${fmt(d.n,0)}</div><div class="note">${d.candidate_count} 个对齐候选观测</div></div><div class="metric"><div class="label">效应估计</div><div class="value">${fmt(d.effect)}</div><div class="note">${esc(d.effect_name)}</div></div><div class="metric"><div class="label">${fmt(d.confidence*100,0)}% 置信区间</div><div class="value" style="font-size:18px">[${fmt(d.ci[0],3)}, ${fmt(d.ci[1],3)}]</div><div class="note">${d.ci[0]>0||d.ci[1]<0?'区间未跨过零':'区间跨过零'}</div></div><div class="metric"><div class="label">双侧 p 值</div><div class="value">${d.p_value===null?'—':d.p_value<.0001?'< 0.0001':fmt(d.p_value,4)}</div><div class="note">${d.p_value===null?'分块区间方法，不显示 p 值':'HAC 稳健标准误'}</div></div></div>
  ${d.group?`<section class="card"><div class="card-top"><h2>条件组与对照组</h2></div>${dataTable([{组别:'满足条件',样本:d.group.event_count,平均结果:fmt(d.group.event_mean),正收益或正变化比例:fmt(d.group.event_positive_rate,1)+'%'},{组别:'其余样本',样本:d.group.control_count,平均结果:fmt(d.group.control_mean),正收益或正变化比例:fmt(d.group.control_positive_rate,1)+'%'}],['组别','样本','平均结果','正收益或正变化比例'])}</section>`:''}
  <div class="chart-grid"><section class="card chart-card full-width"><h3>原始序列走势 · 各自独立刻度</h3>${d.series.map(s=>`<div class="muted">${esc(s.label)}</div>${lineChart(s.points,esc(s.label),s.role==='target'?'#307657':'#ab9a55')}`).join('')}<p class="muted">最多显示每条序列 500 个点；统计计算使用全部有效样本。</p></section>
  <section class="card chart-card"><h3>X 与 Y 的样本关系</h3>${scatterChart(d.points)}<p class="muted">横轴 X：${esc(r.hypothesis.x_transform)}；纵轴 Y：${esc(r.hypothesis.y_transform)}。图中最多显示 700 个点。</p></section>
  <section class="card chart-card"><h3>结果变量 Y 的分布</h3>${histogramChart(d.histogram)}<p class="muted">横轴为结果数值，纵轴为样本数。</p></section></div>
  <section class="card"><div class="card-top"><h2>AI 解读</h2>${button(r.narrative?'重新解读':'让 AI 解读结果','interpret',true)}</div><div class="narrative">${esc(r.narrative || '计算结果已就绪。可让所选模型解释这些数字，补充效应大小、证据和局限。')}</div></section>
  <section class="card"><h2>口径与局限</h2><p class="muted">${esc(d.model_detail)}</p><div class="summary-box">${esc(r.summary)}</div>${warnings(d.warnings)}<details><summary>查看有效样本（前 20 行）</summary>${dataTable(d.rows.slice(0,20),Object.keys(d.rows[0]))}</details><div class="actions">${button('调整模型重新检验','to-model',true)}${button('导出有效样本 CSV','download-analysis',true)}${button('新建一个假设','new')}</div></section>`;
}
function dataTable(rows, columns){return `<div class="table-wrap"><table><thead><tr>${columns.map(c=>`<th>${esc(c)}</th>`).join('')}</tr></thead><tbody>${rows.map(row=>`<tr>${columns.map(c=>`<td>${esc(typeof row[c]==='number'?fmt(row[c],6):row[c])}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`;}

async function refreshHistory(){state.history=await api('history');renderHistory();}
async function pollRun(){
  const id=state.run.id;
  for(let i=0;i<260;i++){
    await new Promise(resolve=>setTimeout(resolve,1200));
    state.run=await api(`runs/${id}`);
    if(!state.run.status.endsWith('running')){
      await refreshHistory();
      if(state.run.status==='failed'){render();throw new Error(state.run.message);}
      return;
    }
  }
  throw new Error('运行时间较长，可以稍后从历史检验中重新打开。');
}
async function clarify(){
  const original=$('#original');if(original)state.original=original.value;
  if(state.original.trim().length<3)throw new Error('先写下一个你想检验的假设。');
  const out=await api('clarify',{original:state.original,answers:state.answers});
  state.questions=out.questions || [];state.message=out.message;state.draft=normalizeDraft(out.draft);state.step=1;render();
}
async function openSettings(){
  const f=$('#settings-form');f.reset();
  f.elements.provider.value=state.settings.provider || 'deepseek';f.elements.model.value=state.settings.model || 'deepseek-v4-flash';f.elements.remember.checked=!!state.settings.remember;
  for(const key of ['deepseek','openai','fred'])$(`#${key}-state`).textContent=state.settings[`${key}_configured`]?'已配置':'未配置';
  $('#settings-status').textContent='';$('#settings-dialog').showModal();
}
async function saveSettings(test=false){
  const f=$('#settings-form');const body=Object.fromEntries(new FormData(f));body.remember=f.elements.remember.checked;body.clear_keys=f.elements.clear_keys.checked;
  state.settings=await api('settings',body);for(const key of ['deepseek_key','openai_key','fred_key'])f.elements[key].value='';
  for(const key of ['deepseek','openai','fred'])$(`#${key}-state`).textContent=state.settings[`${key}_configured`]?'已配置':'未配置';
  renderConnection();$('#settings-status').textContent='设置已保存。';
  if(test){await api('test-model',{});$('#settings-status').textContent='模型连接成功。';}else $('#settings-dialog').close();
}
document.addEventListener('click',async event=>{
  const el=event.target.closest('[data-action]');if(!el)return;
  const action=el.dataset.action;
  if(action==='close-settings'){$('#settings-dialog').close();return;}
  if(state.busy)return;
  const simple=()=>{
    if(action==='settings'){openSettings();return true;}
    if(action==='new'){state.step=0;state.original='';state.draft=null;state.run=null;state.questions=[];state.answers={};state.uploads={};state.model=null;render();return true;}
    if(action==='manual'){state.original=$('#original')?.value || state.original;state.draft=state.draft || defaultDraft();state.step=1;render();return true;}
    if(action==='back-home'){readDraft();state.step=0;render();return true;}
    if(action==='example'){state.original=['沪深300大跌之后，短期反弹是否更明显？','美债收益率上升以后，黄金价格是不是更容易下跌？','WTI油价的变化，与美股后续收益有关吗？'][Number(el.dataset.example)];$('#original').value=state.original;$('#original').focus();return true;}
    if(action==='answer'){readDraft();state.answers[el.dataset.id]=el.dataset.value;render();return true;}
    if(action==='same-series'){readDraft();state.draft.signal={...state.draft.target};render();return true;}
    if(action==='add-control'){readDraft();state.draft.controls.push(pick('fred','DTWEXBGS','广义美元指数'));render();return true;}
    if(action==='remove-control'){readDraft();state.draft.controls.pop();render();return true;}
    if(action==='edit-hypothesis'){state.draft=structuredClone(state.run.hypothesis);state.step=1;state.questions=[];render();return true;}
    if(action==='to-model'){state.model=state.run.recommendation || state.run.model || recommendedDefault();state.step=3;render();return true;}
    if(action==='back-data'){readModel();state.step=2;render();return true;}
    if(action==='choose-model'){readModel();state.model.method=el.dataset.method;render();return true;}
    if(action==='step'){
      const step=Number(el.dataset.step);if(step===state.step)return true;
      if(state.step===1)readDraft();if(state.step===3)readModel();
      const allowed=step===0 || (step===1&&state.draft) || (step===2&&state.run) || (step===3&&state.run?.data) || (step===4&&state.run?.result);
      if(allowed){state.step=step;render();}return true;
    }
    return false;
  };
  if(simple())return;
  const messages={clarify:'AI 正在识别歧义…','clarify-again':'AI 正在根据你的回答更新假设…','confirm-hypothesis':'正在确认假设…','fetch-data':'正在下载并检查数据…',recommend:'AI 正在推荐数学模型…',analyze:'内置工具正在检验假设…',interpret:'AI 正在解读计算结果…','save-test':'正在保存并测试模型连接…',demo:'正在准备合成演示数据…',history:'正在打开历史检验…'};
  await work(messages[action] || '处理中…',async()=>{
    if(action==='clarify'||action==='clarify-again'){await clarify();}
    if(action==='save-test')await saveSettings(true);
    if(action==='confirm-hypothesis'){
      state.run=await api('runs',{hypothesis:readDraft(),confirmed:true});state.uploads={};state.step=2;state.model=null;await refreshHistory();render();
    }
    if(action==='fetch-data'){
      await api(`runs/${state.run.id}/data`,{uploads:state.uploads,demo:false});await pollRun();render();
    }
    if(action==='recommend'){
      const model=await api(`runs/${state.run.id}/recommend`,{});state.run.recommendation=model;state.model=model;render();
    }
    if(action==='analyze'){
      await api(`runs/${state.run.id}/analyze`,{model:readModel(),confirmed:true});await pollRun();state.step=4;render();
      if((state.settings.provider==='openai'?state.settings.openai_configured:state.settings.deepseek_configured)){
        notice('计算完成，AI 正在解读结果…','loading');
        try {const out=await api(`runs/${state.run.id}/interpret`,{});state.run.narrative=out.text;render();}catch(error){state.run.narrative='自动解读未完成：'+error.message+' 可点击重新解读。';render();}
      }
    }
    if(action==='interpret'){const out=await api(`runs/${state.run.id}/interpret`,{});state.run.narrative=out.text;render();}
    if(action==='history'){
      state.run=await api(`runs/${el.dataset.id}`);state.draft=structuredClone(state.run.hypothesis);state.original=state.draft.original;state.questions=[];state.model=state.run.model || state.run.recommendation;state.step=state.run.result?4:state.run.data?3:2;state.uploads={};render();
      if(state.run.status.endsWith('running')){await pollRun();state.step=state.run.result?4:2;render();}
    }
    if(action==='demo'){
      state.original='合成演示：信号变化与随后一期的结果变化是否正相关？';state.draft=defaultDraft();Object.assign(state.draft,{kind:'relation',target:pick('csv','demo_target','演示结果序列'),signal:pick('csv','demo_signal','演示信号序列'),lag:0,direction:'positive'});
      state.run=await api('runs',{hypothesis:state.draft,confirmed:true});await api(`runs/${state.run.id}/data`,{demo:true});state.step=2;await pollRun();render();
    }
  });
});
document.addEventListener('input',event=>{
  const el=event.target;
  if(el.id==='original'){state.original=el.value;state.answers={};state.questions=[];state.draft=null;}
  if(el.dataset.answer!==undefined)state.answers[el.dataset.answer]=el.value;
});
document.addEventListener('change',async event=>{
  const el=event.target;
  if(el.dataset.upload){
    const file=el.files[0];if(!file)return;
    if(file.size>3_000_000){notice('单个 CSV 请控制在 3MB 以内。','error');el.value='';return;}
    state.uploads[el.dataset.upload]=await file.text();el.nextElementSibling.textContent=`已载入 ${file.name}`;return;
  }
  if(el.closest('#settings-form')&&el.name==='provider'){$('#settings-form').elements.model.value=el.value==='openai'?'gpt-4.1-mini':'deepseek-v4-flash';return;}
  if(!el.closest('#draft-form'))return;
  readDraft();
  if(el.name.endsWith('_preset')){
    const role=el.name.replace('_preset','');const preset=state.catalog[Number(el.value)];
    if(preset){const spec={source:preset.source,symbol:preset.symbol,label:preset.label,field:'value'};if(role.startsWith('control'))state.draft.controls[Number(role.slice(-1))-1]=spec;else state.draft[role]=spec;}
    render();
  }else if(el.name==='kind')render();
  else {const summary=$('.summary-box');if(summary)summary.textContent=draftSummary(state.draft);}
});
$('#settings-form').addEventListener('submit',event=>{event.preventDefault();work('正在保存设置…',()=>saveSettings(false));});

try {const initial=await api('bootstrap');Object.assign(state,initial);render();}
catch(error){$('#app').innerHTML=`<div class="error-box">${esc(error.message)}</div>`;}

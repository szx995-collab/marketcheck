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
const termHelp = ids => `<details class="help-panel"><summary>统计术语与例子 · 点开查看${ids ? '本步骤说明' : '完整说明'}</summary><dl>${(state.glossary || []).filter(t=>!ids || ids.includes(t.id)).map(t=>`<dt>${esc(t.term)}</dt><dd>${esc(t.explanation)}<p class="muted">例：${esc(t.example)}</p></dd>`).join('')}</dl></details>`;

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
function syncBusy(){
  document.querySelectorAll('button').forEach(b=>{if(b.dataset.action!=='close-settings')b.disabled=state.busy;});
  document.querySelectorAll('#settings-form .choice-field, #settings-form select').forEach(el=>el.disabled=state.busy);
  document.querySelectorAll('#settings-form input').forEach(el=>el.readOnly=state.busy);
}
function pick(source,symbol,label){return {source,symbol,label,field:'value'};}
function defaultDraft(){
  const end=new Date();end.setUTCDate(end.getUTCDate()-1);
  const start=new Date(end);start.setUTCFullYear(start.getUTCFullYear()-4);
  return {original:state.original,kind:'event',target:pick('tencent','sh000300','沪深300'),signal:pick('tencent','sh000300','沪深300'),controls:[],start:start.toISOString().slice(0,10),end:end.toISOString().slice(0,10),frequency:'daily',timing:'forward',x_transform:'return',y_transform:'return',lookback:1,horizon:1,lag:0,operator:'le',threshold:-1,flat_band:0.1,direction:'positive'};
}
function normalizeDraft(d){
  const base=defaultDraft();
  const result={...base,...d,target:{...base.target,...d.target},signal:{...base.signal,...d.signal},controls:d.controls || []};
  if(result.timing==='concurrent'){result.lookback=result.horizon;result.lag=0;}
  result.controls=result.controls.slice(0,2).map(s=>({...s,field:s.field || 'value'}));
  return result;
}
function renderHistory(){
  $('#history-count').textContent=state.history.length || '';
  $('#history').innerHTML=state.history.length ? state.history.map(r=>`<button class="history-item" data-action="history" data-id="${esc(r.id)}">${esc(r.summary.slice(0,47))}…<small>${esc(r.created.slice(0,10))} · ${r.status==='complete'?'已完成':r.status==='failed'?'待重试':'进行中'}</small></button>`).join('') : '<p class="empty-history">每一次认真提问，<br>都会保存在这里。</p>';
}
function modelReady(){
  const s=state.settings;return s.provider==='codex'?!!state.codex?.logged_in:!!s[`${s.provider}_configured`];
}
function renderConnection(){
  const s=state.settings,ok=modelReady(),name=providerInfo(s.provider)?.label.split('（')[0] || '模型';
  $('#connection').textContent=`${ok?'●':'○'} ${name}${s.provider==='codex'?(ok?' 已登录':' 待检测 / 登录'):(ok?' 已配置':' 待配置')}`;
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
  const concurrent=h.timing==='concurrent';
  return `${heading('DEFINE THE QUESTION','把每一个关键定义确认下来','选项是建议，你可以直接修改。最终以此表单为准；返回修改并再次确认会建立新的检验。')}
  ${state.questions.length?`<section class="card"><div class="card-top"><h2>还需要明确 ${state.questions.length} 个问题</h2><span class="pill">AI 澄清</span></div><p class="muted">${esc(state.message)}</p>${state.questions.map((q,i)=>`<div class="question"><h3>${i+1}. ${esc(q.text)}</h3><div class="options">${(q.options || []).map(opt=>`<button class="option ${state.answers[q.id]===opt?'selected':''}" data-action="answer" data-id="${esc(q.id)}" data-value="${esc(opt)}">${esc(opt)}</button>`).join('')}</div><input data-answer="${esc(q.id)}" value="${esc(state.answers[q.id] || '')}" placeholder="也可以输入自己的定义"></div>`).join('')}<div class="actions">${button('提交回答，更新假设','clarify-again')}</div></section>`:''}
  <section class="card"><div class="card-top"><h2>可检验的假设</h2><span class="micro">所有数值均可编辑</span></div><div id="draft-form">
  <div class="form-grid"><label>问题类型${select('kind',h.kind,[['event','条件成立时，与其余样本比较'],['relation','两个变量的相关 / 回归关系']])}</label><label>预期方向${select('direction',h.direction,[['positive','正向关系 / 条件组更高'],['negative','负向关系 / 条件组更低'],['two_sided','双向差异，不预设方向']])}</label></div>
  <div class="series-grid" style="margin-top:20px">${seriesEditor('target',h.target)}${seriesEditor('signal',h.signal)}</div>
  <div class="actions" style="margin-top:10px">${button('X 使用与 Y 相同的标的','same-series',true)}</div>
  <div class="form-grid three"><label>开始日期${input('start',h.start,'date')}</label><label>结束日期（最晚昨天）${input('end',h.end,'date')}</label><label>分析频率${select('frequency',h.frequency,[['daily','日频 · 各自的观测日'],['weekly','周频 · 完整周末值'],['monthly','月频 · 完整月末值']])}</label></div>
  <label>时间关系${select('timing',h.timing || 'forward',[['forward','后续关系 · X 变化后，Y 随后怎样'],['concurrent','同期关系 · 同一天 / 同一周 / 同一月的联动']])}</label>
  <p class="muted">${concurrent?'同期检验：日频、窗口 1 即比较两者同一天的涨跌幅。X 与 Y 使用同样长的窗口，均截至当期，不额外滞后。':'后续检验：X 截至当期，Y 从当期开始观察未来变化。额外滞后 0 也不等于同期。'}</p>
  <div class="form-grid three"><label>X 使用什么数值${select('x_transform',h.x_transform,[['return','涨跌幅（%）'],['change','原单位变化'],['level','原数值水平']])}</label><label>X 回看多少期${input('lookback',h.lookback,'number',`min="1" max="60" ${concurrent?'disabled':''}`)}</label><label>X 额外滞后多少期${input('lag',h.lag,'number',`min="0" max="20" ${concurrent?'disabled':''}`)}</label></div>
  <div class="form-grid three"><label>Y 观察什么结果${select('y_transform',h.y_transform,[['return',concurrent?'同期收益率（%）':'未来收益率（%）'],['change',concurrent?'同期原单位变化':'未来原单位变化']])}</label><label>${concurrent?'同期窗口（1 = 同一期）':'Y 未来多少期'}${input('horizon',h.horizon,'number','min="1" max="20"')}</label><label>频率口径<input value="周/月频取期末值；不填充缺失" disabled></label></div>
  <div class="comparison-definition"><h3>也看反面：上涨 / 平稳 / 下跌时，Y 分别怎样？</h3>${h.x_transform==='level'?'<p class="muted">原数值水平不能区分上涨和下跌。要做三种情形的对照，请将 X 改为涨跌幅或原单位变化。</p>':`<label>平稳区间半宽（${h.x_transform==='return'?'百分数，例如 0.1 代表 ±0.1%':'X 的原单位，例如利率 0.1 代表 ±0.1 个百分点'}）${input('flat_band',h.flat_band ?? 0,'number','min="0" step="any" required')}</label><p class="muted">X 超过正边界为上涨，低于负边界为下跌，中间含边界为平稳。0 表示仅零变化算平稳；默认值只是起点，请按标的和频率确认。</p>`}</div>
  ${h.kind==='event'?`<div class="form-grid"><label>触发条件：X${select('operator',h.operator,[['le','小于或等于'],['ge','大于或等于']])}</label><label>阈值（涨跌幅 -3 表示 -3%）${input('threshold',h.threshold,'number','step="any"')}</label></div><p class="muted">对照组为同区间、不满足条件的其余有效观测；比较的是平均结果。</p>`:`<details ${h.controls.length?'open':''}><summary>控制变量（可选，最多两个；加入后使用回归）</summary><div class="series-grid">${h.controls.map((s,i)=>seriesEditor(`control${i+1}`,s)).join('')}</div><div class="actions">${h.controls.length<2?button('＋ 添加控制变量','add-control',true):''}${h.controls.length?button('移除最后一个','remove-control',true):''}</div><p class="muted">控制变量使用与 X 相同的变换、回看窗口和滞后期。</p></details>`}
  </div><div class="summary-box">${esc(draftSummary(h))}</div><p class="muted">请核对：黄金期货、黄金 ETF 与现货金价不同；利率的“变化”单位为百分点。跨市场结果只解释历史关联。“条件组更低”表示相对较弱，不一定是实际下跌。</p>
  ${termHelp(['variables','units','windows','comparison','event','control','evidence'])}
  <div class="actions">${button('返回输入','back-home',true)}${button('确认这条假设，继续 →','confirm-hypothesis')}</div></section>`;
}
function draftSummary(h){
  const f={daily:'观测日',weekly:'周',monthly:'月'}[h.frequency];
  const x={return:'涨跌幅(%)',change:'原单位变化',level:'数值水平'}[h.x_transform];
  const y=h.y_transform==='return'?'收益率(%)':'原单位变化';
  const when=h.timing==='concurrent'?`同期检验，检验 ${h.target.label || h.target.symbol} 截至同一期的 ${h.horizon} ${f}${y}`:`额外滞后 ${h.lag} ${f}，检验 ${h.target.label || h.target.symbol} 随后 ${h.horizon} ${f}${y}`;
  return `${h.start} 至 ${h.end}，使用 ${h.signal.label || h.signal.symbol} 的 ${h.lookback} ${f}${x}，${when}。${h.kind==='event'?`条件：X ${h.operator==='le'?'≤':'≥'} ${h.threshold}；与不满足条件的样本比较。`:'检验变量之间的关系。'}${h.x_transform==='level'?'':`补充对照：上涨 X > ${h.flat_band ?? 0}，下跌 X < ${-(h.flat_band ?? 0)}，其余平稳（单位与 X 相同）。`}`;
}
function readDraft(){
  const form=$('#draft-form');if(!form)return state.draft;
  const h={...state.draft};const value=name=>form.querySelector(`[name="${name}"]`)?.value;
  for(const key of ['kind','direction','start','end','frequency','timing','x_transform','y_transform','operator'])if(value(key)!==undefined)h[key]=value(key);
  for(const key of ['lookback','horizon','lag','threshold','flat_band'])if(value(key)!==undefined)h[key]=Number(value(key));
  if(h.timing==='concurrent'){h.lookback=h.horizon;h.lag=0;form.querySelector('[name="lookback"]').value=h.lookback;form.querySelector('[name="lag"]').value=0;}
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
  <div class="summary-box">${(state.glossary || []).filter(t=>t.id===m.method).map(t=>`<strong>${esc(t.term)}</strong><p>${esc(t.explanation)}</p><p>例：${esc(t.example)}</p>`).join('')}</div>
  <div id="model-form" class="form-grid"><label>置信水平${select('confidence',String(m.confidence),[['0.9','90%'],['0.95','95%'],['0.99','99%']])}</label><label>时间依赖阶数（至少 ${r.hypothesis.horizon}）${input('hac_lags',m.hac_lags,'number',`min="${r.hypothesis.horizon}" max="60"`)}</label></div>
  <p class="muted">回归/条件比较用于 HAC 阶数；相关模型用来设置不小于此阶数 + 1 的分块长度。检验为双侧；假设方向单独与结果核对。</p>
  ${r.hypothesis.controls?.length?'<p class="muted">控制变量与 X 使用相同变换、回看窗口和滞后期。</p>':''}
  <p class="muted">${r.hypothesis.x_transform==='level'?'本次 X 为原水平，不生成涨跌三组。':`同时提供上涨 / 平稳 / 下跌的样本表现和三项组间均值差；平稳半宽为 ${r.hypothesis.flat_band ?? 0}（X 单位）。组间差采用 HAC 与 Bonferroni 校正，辅助检查反面，不替代所选主检验。`}</p>
  ${termHelp(['event','pearson','spearman','regression','ci','p','hac','bootstrap','multiplicity','effect'])}
  <div class="warning">置信区间不是“假设为真的概率”。重复挑选阈值或模型会影响证据强度；样本比例只描述历史，不单独检验涨跌概率。</div>
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
  <div class="result-intro"><span class="eyebrow">主检验 · ${esc(labels[r.model.method])} · ${fmt(d.confidence*100,0)}% 置信水平</span><h2 style="margin-top:12px">${esc(d.verdict)}</h2><p>${esc(d.explanation)}</p>${d.takeaways?.length?`<ul class="takeaways">${d.takeaways.map(t=>`<li>${esc(t)}</li>`).join('')}</ul>`:''}</div>
  <div class="metrics"><div class="metric"><div class="label">有效样本</div><div class="value">${fmt(d.n,0)}</div><div class="note">${d.candidate_count} 个对齐候选观测</div></div><div class="metric"><div class="label">效应估计</div><div class="value">${fmt(d.effect)}</div><div class="note">${esc(d.effect_name)}</div></div><div class="metric"><div class="label">${fmt(d.confidence*100,0)}% 置信区间</div><div class="value" style="font-size:18px">[${fmt(d.ci[0],3)}, ${fmt(d.ci[1],3)}]</div><div class="note">${d.ci[0]>0||d.ci[1]<0?'区间未跨过零':'区间跨过零'}</div></div><div class="metric"><div class="label">双侧 p 值</div><div class="value">${d.p_value===null?'—':d.p_value<.0001?'< 0.0001':fmt(d.p_value,4)}</div><div class="note">${d.p_value===null?'分块区间方法，不显示 p 值':'HAC 稳健标准误'}</div></div></div>
  ${d.group?`<section class="card"><div class="card-top"><h2>主检验：条件组与对照组</h2></div><p class="muted">条件：X ${r.hypothesis.operator==='le'?'≤':'≥'} ${r.hypothesis.threshold}。均值和中位数单位：${esc(d.y_unit || (r.hypothesis.y_transform==='return'?'%':'Y 原单位'))}；收益率的均值差用百分点。条件不成立包含全部其余有效样本。</p>${dataTable(['event','control'].map(k=>({组别:k==='event'?'满足条件':'不满足条件',样本:d.group[`${k}_count`],平均结果:fmt(d.group[`${k}_mean`]),中位数:fmt(d.group[`${k}_median`]),上涨比例:rate(d.group[`${k}_positive_rate`]),下跌比例:rate(d.group[`${k}_negative_rate`])})),['组别','样本','平均结果','中位数','上涨比例','下跌比例'])}</section>`:''}
  ${comparisonView(d)}
  ${termHelp()}
  <div class="chart-grid"><section class="card chart-card full-width"><h3>原始序列走势 · 各自独立刻度</h3>${d.series.map(s=>`<div class="muted">${esc(s.label)}</div>${lineChart(s.points,esc(s.label),s.role==='target'?'#307657':'#ab9a55')}`).join('')}<p class="muted">最多显示每条序列 500 个点；统计计算使用全部有效样本。</p></section>
  <section class="card chart-card"><h3>X 与 Y 的样本关系</h3>${scatterChart(d.points)}<p class="muted">横轴 X：${esc({return:'涨跌幅（%）',change:'原单位变化',level:'原数值水平'}[r.hypothesis.x_transform])}；纵轴 Y：${esc(r.hypothesis.y_transform==='return'?(r.hypothesis.timing==='concurrent'?'同期收益率（%）':'未来收益率（%）'):(r.hypothesis.timing==='concurrent'?'同期原单位变化':'未来原单位变化'))}。图中最多显示 700 个点。</p></section>
  <section class="card chart-card"><h3>结果变量 Y 的分布</h3>${histogramChart(d.histogram)}<p class="muted">横轴为结果数值，纵轴为样本数。</p></section></div>
  <section class="card"><div class="card-top"><h2>AI 辅助解读</h2>${button(r.narrative?'重新解读':'让 AI 解读结果','interpret',true)}</div><p class="muted">AI 选择本次值得解释的术语；新版解读的事实与判断直接对应上方计算结果。</p>${r.narrative && r.narrative_version!==2?'<div class="warning">此解读来自旧版或上次未成功完成，请点击重新解读，以计算表格为准。</div>':''}<div class="narrative">${esc(r.narrative || '计算结果已就绪。可让所选模型选择解释重点，帮助理解效应大小、证据和局限。')}</div></section>
  <section class="card"><h2>口径与局限</h2><p class="muted">${esc(d.model_detail)}</p><div class="summary-box">${esc(r.summary)}</div>${warnings(d.warnings)}<details><summary>查看有效样本（前 20 行）</summary>${dataTable(d.rows.slice(0,20),Object.keys(d.rows[0]))}</details><div class="actions">${button('调整模型重新检验','to-model',true)}${button('导出有效样本 CSV','download-analysis',true)}${button('新建一个假设','new')}</div></section>`;
}
const rate = value => value == null ? '—' : fmt(value,1)+'%';
function comparisonView(d){
  const c=d.comparison;
  if(!c)return '<section class="card"><h2>补充对照</h2><p class="muted">这是旧版保存的结果。修改假设、确认平稳区间后重新检验，即可生成三组对照与更详细的结论。</p></section>';
  if(!c.available)return `<section class="card"><h2>补充对照</h2><p class="muted">${esc(c.note)}</p></section>`;
  const groups=[...c.groups,{...c.overall,label:'全部样本（总体参考）',rule:'前三组合计，不是独立对照组'}];
  return `<section class="card"><div class="card-top"><h2>补充对照：X 上涨、平稳、下跌，Y 分别怎样？</h2></div><p class="muted">Y 均值和中位数单位：${esc(d.y_unit)}。负值为下跌 / 负变化；每组不足 15 个时只展示描述数据。${state.run.hypothesis.controls?.length?'此表未调整控制变量。':''}</p>${dataTable(groups.map(g=>({情形:g.label,划分:g.rule,样本:g.count,平均结果:fmt(g.mean),中位数:fmt(g.median),上涨比例:rate(g.positive_rate),下跌比例:rate(g.negative_rate),不变比例:rate(g.zero_rate)})),['情形','划分','样本','平均结果','中位数','上涨比例','下跌比例','不变比例'])}<h3>三组之间，差多少？</h3><p class="muted">均值差 = 前组减后组，单位：${d.y_unit==='%'?'百分点':esc(d.y_unit)}。校正后的区间不含零时，才标明哪组更高 / 更低。</p>${dataTable(c.pairs.map(p=>({比较:p.label,均值差:fmt(p.effect),校正后区间:p.ci?`[${fmt(p.ci[0])}, ${fmt(p.ci[1])}]`:'—',校正后p值:p.p_adjusted==null?'—':p.p_adjusted<.0001?'< 0.0001':fmt(p.p_adjusted),判断:p.status})),['比较','均值差','校正后区间','校正后p值','判断'])}<p class="muted">${esc(c.note)}</p></section>`;
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
function providerInfo(id){return state.providers?.find(p=>p.id===id);}
function settingsKeyIDs(){return [...state.providers.filter(p=>p.id!=='codex').map(p=>p.id),'fred'];}
function initializeSettings(){
  state.savedCustomModels=[...(state.models.compatible || [])];
  $('#settings-form').elements.provider.innerHTML=state.providers.map(p=>`<option value="${esc(p.id)}">${esc(p.label)}</option>`).join('');
  $('#model-key-fields').innerHTML=state.providers.filter(p=>p.id!=='codex').map(p=>`<label data-key-provider="${esc(p.id)}">${esc(p.label)} API Key <span id="${esc(p.id)}-state" class="key-state"></span><input name="${esc(p.id)}_key" type="password" autocomplete="off" placeholder="输入所选服务的 API Key；留空保留已有 Key"></label>`).join('');
}
function renderKeyStates(){for(const key of settingsKeyIDs())$(`#${key}-state`).textContent=state.settings[`${key}_configured`]?'已配置':'未配置';}
async function openSettings(){
  const f=$('#settings-form');f.reset();
  state.models.compatible=[...state.savedCustomModels];
  f.elements.provider.value=state.settings.provider || 'deepseek';f.elements.remember.checked=!!state.settings.remember;
  f.elements.compatible_base_url.value=state.settings.compatible_base_url || '';
  f.elements.compatible_json_mode.checked=!!state.settings.compatible_json_mode;
  state.editProvider=f.elements.provider.value;
  state.editModels={[state.editProvider]:state.settings.model ?? ''};
  state.editEfforts={[`${state.editProvider}:${state.settings.model ?? ''}`]:state.settings.reasoning_effort || 'auto'};
  renderKeyStates();
  $('#settings-status').textContent='';updateProviderFields();$('#settings-dialog').showModal();
  if(f.elements.provider.value==='codex')await refreshCodexStatus();
}
function updateProviderFields(){
  const f=$('#settings-form'),id=f.elements.provider.value,codex=id==='codex',compatible=id==='compatible',provider=providerInfo(id);
  $('#codex-help').hidden=!codex;
  $('#provider-hint').textContent=provider?.hint || '';
  $('#compatible-options').hidden=!compatible;
  f.elements.compatible_base_url.required=compatible;f.elements.compatible_base_url.disabled=!compatible;
  f.elements.compatible_json_mode.disabled=!compatible;
  document.querySelectorAll('[data-key-provider]').forEach(label=>{label.hidden=label.dataset.keyProvider!==id;});
  renderModelOptions();
}
const choiceTabs=(name, selected, options)=>options.map(o=>`<label class="choice-tab"><input type="radio" name="${name}" value="${esc(o.value)}" ${o.value===selected?'checked':''}><span>${esc(o.label)}</span></label>`).join('');
function renderModelOptions(){
  const id=state.editProvider,models=state.models[id] || [];
  let selected=state.editModels[id] ?? providerInfo(id)?.default_model;
  if(!models.some(m=>m.id===selected))selected=models[0]?.id;
  state.editModels[id]=selected;
  $('#llm-model-options').innerHTML=models.length?choiceTabs('model',selected,models.map(m=>({value:m.id,label:m.label}))):'<p class="muted">填写接口地址与 Key，点击“读取模型列表”后选择。</p>';
  renderEffortOptions();
}
function renderEffortOptions(){
  const id=state.editProvider,model=state.editModels[id],spec=state.models[id]?.find(m=>m.id===model),key=`${id}:${model}`;
  let selected=state.editEfforts[key] ?? spec?.default_effort;
  if(spec && !spec.efforts.some(e=>e.value===selected))selected=spec.default_effort;
  state.editEfforts[key]=selected;
  $('#effort-options').innerHTML=spec?choiceTabs('reasoning_effort',selected,spec.efforts):'';
  $('#effort-hint').textContent=spec?.hint || '先选择模型，即可查看它支持的推理档位。';
}
async function loadModelOptions(){
  const f=$('#settings-form');
  $('#settings-status').textContent='正在读取接口提供的模型列表…';
  try{
    state.models.compatible=await api('models',{base_url:f.elements.compatible_base_url.value,key:f.elements.compatible_key.value,clear_keys:f.elements.clear_keys.checked});
    renderModelOptions();syncBusy();$('#settings-status').textContent=`已读取 ${state.models.compatible.length} 个模型，请选择后保存。未知模型使用服务默认推理设置。`;
  }catch(error){$('#settings-status').textContent=error.message;throw error;}
}
async function refreshCodexStatus(){
  $('#codex-status').textContent='正在检测本机 Codex 登录…';
  try {state.codex=await api('codex/status');$('#codex-status').textContent=state.codex.message;}
  catch(error){state.codex={logged_in:false};$('#codex-status').textContent=error.message;}
  renderConnection();
}
async function saveSettings(test=false){
  const f=$('#settings-form');if(!f.reportValidity())return;
  const body=Object.fromEntries(new FormData(f));body.remember=f.elements.remember.checked;body.clear_keys=f.elements.clear_keys.checked;body.compatible_json_mode=f.elements.compatible_json_mode.checked;
  body.provider=state.editProvider;body.model=state.editModels[body.provider];body.reasoning_effort=state.editEfforts[`${body.provider}:${body.model}`];
  if(body.model===undefined || (body.model==='' && body.provider!=='codex'))throw new Error('请先读取并选择模型。');
  state.settings=await api('settings',body);for(const key of settingsKeyIDs())f.elements[`${key}_key`].value='';
  state.savedCustomModels=[...state.models.compatible];
  renderKeyStates();
  renderConnection();$('#settings-status').textContent='设置已保存。';
  f.elements.clear_keys.checked=false;
  if(test){
    $('#settings-status').textContent='设置已保存，正在按所选模型与强度测试连接；高强度可能需要数分钟…';
    try {await api('test-model',{});if(state.settings.provider==='codex')await refreshCodexStatus();$('#settings-status').textContent='模型连接成功。';}
    catch(error){$('#settings-status').textContent=error.message;throw error;}
  }else $('#settings-dialog').close();
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
    if(action==='load-models')await loadModelOptions();
    if(action==='codex-status')await refreshCodexStatus();
    if(action==='confirm-hypothesis'){
      const hypothesis=readDraft();
      if(hypothesis.x_transform!=='level' && (!$('[name=flat_band]').value.trim() || !Number.isFinite(hypothesis.flat_band) || hypothesis.flat_band<0))throw new Error('请填写大于或等于 0 的平稳区间半宽。');
      state.run=await api('runs',{hypothesis,confirmed:true});state.uploads={};state.step=2;state.model=null;await refreshHistory();render();
    }
    if(action==='fetch-data'){
      await api(`runs/${state.run.id}/data`,{uploads:state.uploads,demo:false});await pollRun();render();
    }
    if(action==='recommend'){
      const model=await api(`runs/${state.run.id}/recommend`,{});state.run.recommendation=model;state.model=model;render();
    }
    if(action==='analyze'){
      await api(`runs/${state.run.id}/analyze`,{model:readModel(),confirmed:true});await pollRun();state.step=4;render();
      if(modelReady()){
        notice('计算完成，AI 正在解读结果…','loading');
        try {const out=await api(`runs/${state.run.id}/interpret`,{});state.run.narrative=out.text;state.run.narrative_version=2;render();}catch(error){state.run.narrative='自动解读未完成：'+error.message+' 可点击重新解读。';render();}
      }
    }
    if(action==='interpret'){const out=await api(`runs/${state.run.id}/interpret`,{});state.run.narrative=out.text;state.run.narrative_version=2;render();}
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
  if(el.closest('#draft-form')){readDraft();const summary=$('#draft-form + .summary-box');if(summary)summary.textContent=draftSummary(state.draft);}
});
document.addEventListener('change',async event=>{
  const el=event.target;
  if(el.dataset.upload){
    const file=el.files[0];if(!file)return;
    if(file.size>3_000_000){notice('单个 CSV 请控制在 3MB 以内。','error');el.value='';return;}
    state.uploads[el.dataset.upload]=await file.text();el.nextElementSibling.textContent=`已载入 ${file.name}`;return;
  }
  if(el.closest('#settings-form')&&el.name==='provider'){
    state.editProvider=el.value;
    updateProviderFields();if(el.value==='codex')await refreshCodexStatus();return;
  }
  if(el.closest('#settings-form')&&el.name==='model'){
    state.editModels[state.editProvider]=el.value;renderEffortOptions();return;
  }
  if(el.closest('#settings-form')&&el.name==='reasoning_effort'){
    state.editEfforts[`${state.editProvider}:${state.editModels[state.editProvider]}`]=el.value;return;
  }
  if(el.closest('#settings-form')&&el.name==='compatible_base_url'){
    state.models.compatible=[];delete state.editModels.compatible;renderModelOptions();return;
  }
  if(!el.closest('#draft-form'))return;
  readDraft();
  if(el.name.endsWith('_preset')){
    const role=el.name.replace('_preset','');const preset=state.catalog[Number(el.value)];
    if(preset){const spec={source:preset.source,symbol:preset.symbol,label:preset.label,field:'value'};if(role.startsWith('control'))state.draft.controls[Number(role.slice(-1))-1]=spec;else state.draft[role]=spec;}
    render();
  }else if(el.name==='kind'||el.name==='timing')render();
  else if(el.name==='x_transform'){state.draft.flat_band=0;render();notice('X 的单位已变更，平稳半宽已重置为 0；请按新单位确认。');}
  else {const summary=$('.summary-box');if(summary)summary.textContent=draftSummary(state.draft);}
});
$('#settings-form').addEventListener('submit',event=>{event.preventDefault();work('正在保存设置…',()=>saveSettings(false));});

try {const initial=await api('bootstrap');Object.assign(state,initial);initializeSettings();render();if(state.settings.provider==='codex')await refreshCodexStatus();}
catch(error){$('#app').innerHTML=`<div class="error-box">${esc(error.message)}</div>`;}

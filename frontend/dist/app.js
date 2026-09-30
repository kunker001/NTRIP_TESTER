'use strict';

const $ = id => document.getElementById(id);
const HIST = 300;
const TL = 120;
const MAX_CONSOLE = 1500;
const MAX_LOG = 300;
const CFG_KEY = 'ntrip-tester-cfg';
const STATE_TEXT = { idle: '未连接', connecting: '连接中…', connected: '已连接', reconnecting: '等待重连', error: '连接错误' };
const MSM_SYS = { 107: 'GPS', 108: 'GLO', 109: 'GAL', 110: 'SBAS', 111: 'QZSS', 112: 'BDS', 113: 'NavIC' };
const EPH_SYS = { 1019: 'GPS', 1020: 'GLO', 1041: 'NavIC', 1042: 'BDS', 1044: 'QZSS', 1045: 'GAL', 1046: 'GAL' };

const S = {
  stats: null,
  bitrate: [],
  timeline: new Map(),
  lastCounts: new Map(),
  lastFrames: 0,
  satHist: new Map(),
  table: null,
  stList: [],
  selectedMount: '',
  stSort: { key: 'distance', asc: true },
  msgSort: { key: 'type', asc: true },
  consoleLines: [],
  tab: 'overview',
};

const api = () => window.go.main.App;

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
}
function fmtBytes(n) {
  if (n < 1024) return n + ' B';
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
  if (n < 1073741824) return (n / 1048576).toFixed(2) + ' MB';
  return (n / 1073741824).toFixed(2) + ' GB';
}
function fmtRate(bps) {
  if (bps < 1000) return bps.toFixed(0) + ' bps';
  if (bps < 1e6) return (bps / 1000).toFixed(1) + ' kbps';
  return (bps / 1e6).toFixed(2) + ' Mbps';
}
function fmtDur(sec) {
  sec = Math.floor(sec);
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = sec % 60;
  return (h ? h + ':' + String(m).padStart(2, '0') : String(m)) + ':' + String(s).padStart(2, '0');
}
function fmtNum(n) { return Number(n).toLocaleString('en-US'); }
function fmtDeg(v, p, n) { return `${Math.abs(v).toFixed(8)}° ${v >= 0 ? p : n}`; }
function latClass(ms) { const a = Math.abs(ms); return a < 1500 ? 'lat-ok' : a < 5000 ? 'lat-mid' : 'lat-bad'; }
function num(id) { const v = parseFloat($(id).value); return isFinite(v) ? v : 0; }
function pushCap(arr, v, n) { arr.push(v); if (arr.length > n) arr.splice(0, arr.length - n); }

function haversine(lat1, lon1, lat2, lon2) {
  const r = d => (d * Math.PI) / 180;
  const a = Math.sin(r(lat2 - lat1) / 2) ** 2 + Math.cos(r(lat1)) * Math.cos(r(lat2)) * Math.sin(r(lon2 - lon1) / 2) ** 2;
  return 2 * 6371.0088 * Math.asin(Math.min(1, Math.sqrt(a)));
}

function msgColor(t) {
  if (t >= 1071 && t <= 1137) return SYS_COLORS[MSM_SYS[Math.floor(t / 10)]] || '#38bdf8';
  if (EPH_SYS[t]) return hexA(SYS_COLORS[EPH_SYS[t]], 0.55);
  if ((t >= 1005 && t <= 1008) || t === 1033) return '#f59e0b';
  if (t === 1230) return '#fb923c';
  if (t >= 4001 && t <= 4095) return '#e879f9';
  return '#38bdf8';
}

function sortBy(list, { key, asc }) {
  list.sort((a, b) => {
    const x = a[key], y = b[key];
    const r = typeof x === 'string' || typeof y === 'string'
      ? String(x ?? '').localeCompare(String(y ?? ''))
      : x === y ? 0 : x < y ? -1 : 1;
    return asc ? r : -r;
  });
}

function setupSortHeader(table, cols, sortState, onChange) {
  const head = table.tHead;
  head.innerHTML = '<tr>' + cols.map(([k, t]) => `<th data-k="${k}">${t}</th>`).join('') + '</tr>';
  head.onclick = e => {
    const k = e.target.dataset.k;
    if (!k) return;
    if (sortState.key === k) sortState.asc = !sortState.asc;
    else { sortState.key = k; sortState.asc = true; }
    onChange();
  };
}
function markSorted(table, sortState) {
  for (const th of table.tHead.querySelectorAll('th')) {
    const on = th.dataset.k === sortState.key;
    th.classList.toggle('sorted', on);
    th.classList.toggle('desc', on && !sortState.asc);
  }
}

// ---------- config ----------

function readCfg() {
  return {
    host: $('host').value.trim(),
    port: parseInt($('port').value, 10) || 2101,
    tls: $('tls').checked,
    selfSign: $('selfSign').checked,
    version: $('version').value,
    username: $('username').value,
    password: $('password').value,
    mountpoint: $('mountpoint').value.trim(),
    ggaInterval: parseInt($('ggaInterval').value, 10) || 0,
    lat: num('lat'), lon: num('lon'), alt: num('alt'), sep: num('sep'),
    autoReconnect: $('autoReconnect').checked,
    timeout: parseInt($('timeout').value, 10) || 30,
  };
}

function loadCfg() {
  let c = null;
  try { c = JSON.parse(localStorage.getItem(CFG_KEY) || 'null'); } catch { c = null; }
  if (!c) return;
  for (const k of ['host', 'port', 'version', 'username', 'password', 'mountpoint', 'ggaInterval', 'timeout']) {
    if (c[k] !== undefined && c[k] !== null) $(k).value = c[k];
  }
  for (const k of ['lat', 'lon', 'alt', 'sep']) if (c[k]) $(k).value = c[k];
  for (const k of ['tls', 'selfSign', 'autoReconnect', 'savePwd']) if (k in c) $(k).checked = !!c[k];
}

function saveCfg() {
  const c = readCfg();
  c.savePwd = $('savePwd').checked;
  if (!c.savePwd) c.password = '';
  localStorage.setItem(CFG_KEY, JSON.stringify(c));
}

function pushReference() {
  api().SetReference(num('lat'), num('lon'), num('alt'));
}

// ---------- event log ----------

function addLog(level, msg, time) {
  const box = $('eventLog');
  const d = document.createElement('div');
  d.className = level;
  d.textContent = `${time || new Date().toTimeString().slice(0, 8)}  ${msg}`;
  const stick = box.scrollTop + box.clientHeight >= box.scrollHeight - 20;
  box.appendChild(d);
  while (box.childElementCount > MAX_LOG) box.firstChild.remove();
  if (stick) box.scrollTop = box.scrollHeight;
}

// ---------- actions ----------

async function fetchTable() {
  saveCfg();
  const btn = $('btnTable');
  btn.disabled = true;
  btn.textContent = '获取中…';
  try {
    const t = await api().FetchSourceTable(readCfg());
    t.mounts = t.mounts || [];
    S.table = t;
    $('mountList').innerHTML = t.mounts.map(m => `<option value="${esc(m.name)}">${esc(m.identifier)} · ${esc(m.format)}</option>`).join('');
    $('stRaw').textContent = t.raw || '';
    switchTab('sourcetable');
  } catch (e) {
    addLog('error', '获取源列表失败：' + e);
  } finally {
    btn.disabled = false;
    btn.textContent = '获取源列表';
  }
}

async function connect() {
  saveCfg();
  try {
    await api().Connect(readCfg());
    resetHist();
    S.consoleLines = [];
    if (S.tab === 'console') rebuildConsole();
  } catch (e) {
    addLog('error', String(e));
  }
}

async function disconnect() {
  try { await api().Disconnect(); } catch (e) { addLog('error', String(e)); }
}

async function toggleRecord() {
  try {
    if (S.stats && S.stats.recording) await api().StopRecording();
    else await api().StartRecording();
  } catch (e) {
    addLog('error', '记录失败：' + e);
  }
}

async function resetStats() {
  await api().ResetStats();
  resetHist();
  S.consoleLines = [];
  if (S.tab === 'console') rebuildConsole();
}

function resetHist() {
  S.bitrate = [];
  S.timeline.clear();
  S.lastCounts.clear();
  S.satHist.clear();
  S.lastFrames = 0;
}

function useArp() {
  const st = S.stats && S.stats.station;
  if (!st || !st.hasArp) { addLog('warn', '尚未收到基站坐标（1005/1006）'); return; }
  $('lat').value = st.lat.toFixed(8);
  $('lon').value = st.lon.toFixed(8);
  $('alt').value = st.height.toFixed(3);
  saveCfg();
  pushReference();
  addLog('info', '已将基站 ARP 坐标设为 GGA / 参考位置');
}

function useMount() {
  const name = $('mountpoint').value.trim();
  const m = S.table && (S.table.mounts || []).find(x => x.name === name);
  if (!m || (!m.lat && !m.lon)) { addLog('warn', '源列表中找不到该挂载点的坐标，请先获取源列表'); return; }
  $('lat').value = m.lat;
  $('lon').value = m.lon;
  saveCfg();
  pushReference();
  addLog('info', `已将挂载点 ${name} 的坐标设为 GGA / 参考位置`);
  if (S.tab === 'sourcetable') renderSourceTable();
}

function selectMount(name, scroll) {
  S.selectedMount = name;
  $('mountpoint').value = name;
  saveCfg();
  for (const tr of $('stTable').tBodies[0].rows) {
    const on = tr.dataset.name === name;
    tr.classList.toggle('sel', on);
    if (on && scroll) tr.scrollIntoView({ block: 'nearest' });
  }
  drawMap();
}

// ---------- state & header ----------

function updateState(state, msg) {
  $('stateDot').className = 'dot ' + state;
  $('stateText').textContent = STATE_TEXT[state] || state;
  if (msg !== undefined) {
    $('stateMsg').textContent = msg;
    $('stateMsg').title = msg;
  }
  const busy = state === 'connecting' || state === 'connected' || state === 'reconnecting';
  $('btnDisconnect').disabled = !busy;
  $('btnConnect').textContent = busy ? '重新连接' : '连接';
}

function updateHeader(s) {
  $('kUptime').textContent = s.connectedFor > 0 ? fmtDur(s.connectedFor) : '--';
  $('kBitrate').textContent = fmtRate(s.bitrate);
  $('kAvg').textContent = `${fmtRate(s.avgBitrate)} / ${fmtRate(s.peakBitrate)}`;
  $('kBytes').textContent = fmtBytes(s.totalBytes);
  $('kFrames').textContent = fmtNum(s.frames);
  const crc = $('kCrc');
  crc.textContent = `${s.crcErrors} / ${fmtBytes(s.garbageBytes)}`;
  crc.classList.toggle('bad', s.crcErrors > 0);
  const last = $('kLast');
  last.textContent = s.lastDataAgo < 0 ? '--' : s.lastDataAgo < 1 ? '< 1 s' : s.lastDataAgo.toFixed(0) + ' s 前';
  last.classList.toggle('bad', s.state === 'connected' && s.lastDataAgo > 5);
  const fresh = (s.systems || []).filter(x => x.ageSec < 5);
  const lat = $('kLatency');
  if (fresh.length) {
    const ms = Math.min(...fresh.map(x => x.latencyMs));
    lat.textContent = ms.toFixed(0) + ' ms';
    lat.className = latClass(ms);
    lat.title = fresh.map(x => `${x.system}: ${x.latencyMs.toFixed(0)} ms`).join('\n') + '\n（本机时钟 - 观测历元，受本机时间精度影响）';
  } else {
    lat.textContent = '--';
    lat.className = '';
  }
}

function updateSidebar(s) {
  const btn = $('btnRecord');
  btn.textContent = s.recording ? '停止记录' : '记录原始数据';
  btn.classList.toggle('rec', s.recording);
  const info = [];
  if (s.recording) info.push(`记录中：${s.recordPath}（${fmtBytes(s.recordBytes)}）`);
  if (s.ggaSent) info.push(`已发送 GGA ${s.ggaSent} 次`);
  if (s.reconnects) info.push(`重连 ${s.reconnects} 次`);
  $('recordInfo').textContent = info.join(' · ');
}

// ---------- stats ----------

function onStats(s) {
  s.messages = s.messages || [];
  s.systems = s.systems || [];
  s.ephemeris = s.ephemeris || [];
  s.texts = s.texts || [];
  S.stats = s;
  if (s.frames < S.lastFrames) resetHist();
  S.lastFrames = s.frames;

  const live = s.state === 'connected' || s.state === 'connecting' || s.state === 'reconnecting';
  if (live) {
    pushCap(S.bitrate, s.bitrate / 1000, HIST);
    for (const m of s.messages) {
      const prev = S.lastCounts.get(m.type);
      let d = prev === undefined ? m.count : m.count - prev;
      if (d < 0) d = m.count;
      S.lastCounts.set(m.type, m.count);
      let arr = S.timeline.get(m.type);
      if (!arr) S.timeline.set(m.type, (arr = []));
      pushCap(arr, d, TL);
    }
    const cur = new Map(s.systems.map(x => [x.system, x.ageSec < 5 ? x.numSats : 0]));
    for (const k of cur.keys()) if (!S.satHist.has(k)) S.satHist.set(k, []);
    for (const [k, arr] of S.satHist) pushCap(arr, cur.get(k) || 0, HIST);
  }

  updateState(s.state, s.stateMsg);
  updateHeader(s);
  updateSidebar(s);
  if (S.tab !== 'sourcetable' && S.tab !== 'console') render();
}

function render() {
  if (S.tab === 'overview') renderOverview();
  else if (S.tab === 'messages') renderMessages();
  else if (S.tab === 'signals') renderSignals();
  else if (S.tab === 'sourcetable') drawMap();
}

// ---------- overview ----------

function renderOverview() {
  const s = S.stats;
  const msgs = s ? s.messages : [];
  drawLineChart($('cvBitrate'), {
    series: [{ name: '码率', color: '#38bdf8', data: S.bitrate, fill: true }],
    length: HIST,
    fmt: v => (v >= 100 ? v.toFixed(0) : v.toFixed(1)),
    empty: '连接后显示码率',
  });
  drawDonut($('cvDonut'),
    msgs.map(m => ({ label: `${m.type} ${m.name}`, value: m.bytes, color: msgColor(m.type) })),
    [fmtBytes(s ? s.rtcmBytes : 0), `${msgs.length} 种消息`]);

  const rows = msgs.map(m => ({
    label: `${m.type}  ${m.name}`,
    color: msgColor(m.type),
    cells: S.timeline.get(m.type) || [],
    right: m.rate ? m.rate.toFixed(m.rate < 1 ? 2 : 1) + ' Hz' : '',
  }));
  const cv = $('cvTimeline');
  const hh = Math.max(80, rows.length * 18 + 24) + 'px';
  if (cv.style.height !== hh) cv.style.height = hh;
  drawTimeline(cv, rows, TL);

  $('stationBox').innerHTML = stationHtml(s && s.station);
  $('sysBox').innerHTML = systemsHtml(s ? s.systems : []);
  const texts = s ? s.texts : [];
  $('textCard').hidden = !texts.length;
  $('textBox').innerHTML = texts.slice().reverse().map(t => `<div><span class="muted">${t.time} 站号 ${t.station}</span> ${esc(t.text)}</div>`).join('');
}

function stationHtml(st) {
  if (!st) return '<div class="empty">尚未收到基准站消息（1005 / 1006 / 1007 / 1008 / 1033）</div>';
  const kv = [];
  const add = (k, v) => { if (v !== '' && v !== undefined && v !== null) kv.push(`<div class="k">${k}</div><div class="v">${v}</div>`); };
  add('站号', st.stationId);
  if (st.hasArp) {
    add('坐标消息', `RTCM ${st.arpType}${st.refStation ? '（物理参考站）' : ''}`);
    add('ITRF 实现年', st.itrfYear || '未指定');
    add('支持系统', esc((st.systems || []).join(' / ')));
    add('ECEF X', st.x.toFixed(4) + ' m');
    add('ECEF Y', st.y.toFixed(4) + ' m');
    add('ECEF Z', st.z.toFixed(4) + ' m');
    add('纬度', fmtDeg(st.lat, 'N', 'S'));
    add('经度', fmtDeg(st.lon, 'E', 'W'));
    add('椭球高', st.height.toFixed(3) + ' m');
    if (st.arpType === 1006) add('天线高', st.antennaHeight.toFixed(4) + ' m');
    if (st.hasDistance) add('距参考位置', `<b>${st.distanceKm.toFixed(3)} km</b> · 方位 ${st.bearing.toFixed(1)}°`);
  }
  if (st.antenna) add('天线', esc(st.antenna) + (st.setupId ? ` <span class="muted">(Setup ${st.setupId})</span>` : ''));
  if (st.antennaSerial) add('天线序列号', esc(st.antennaSerial));
  if (st.receiver) add('接收机', esc(st.receiver));
  if (st.firmware) add('固件版本', esc(st.firmware));
  if (st.receiverSerial) add('接收机序列号', esc(st.receiverSerial));
  return `<div class="kv">${kv.join('')}</div>`;
}

function systemsHtml(list) {
  if (!list.length) return '<div class="empty">等待 MSM 观测消息…</div>';
  return list.map(x => {
    const c = SYS_COLORS[x.system] || '#38bdf8';
    const stale = x.ageSec > 5;
    return `<div class="sys-card" style="border-left-color:${c}${stale ? ';opacity:.5' : ''}">
      <div class="sys-name" style="color:${c}">${x.system} <span class="muted">MSM${x.msgType % 10} · ${x.msgType}</span></div>
      <div class="sys-big">${x.numSats}<span>颗卫星</span></div>
      <div>${x.numCells} 个观测 · ${esc((x.signals || []).join(' '))}</div>
      <div>延迟 <span class="${latClass(x.latencyMs)}">${x.latencyMs.toFixed(0)} ms</span> · ${x.ageSec.toFixed(1)} s 前</div>
    </div>`;
  }).join('');
}

// ---------- messages ----------

const MSG_COLS = [
  ['type', '类型'], ['name', '名称'], ['count', '数量'], ['rate', '频率 (Hz)'], ['interval', '平均间隔 (s)'],
  ['lastAgo', '距上次 (s)'], ['bytes', '字节'], ['share', '占比'], ['maxLen', '长度 (B)'], ['info', '解析信息'],
];

function renderMessages() {
  const s = S.stats;
  const tb = $('msgTable');
  if (!tb.tHead.rows.length) setupSortHeader(tb, MSG_COLS, S.msgSort, renderMessages);
  markSorted(tb, S.msgSort);
  if (!s) return;
  const total = s.rtcmBytes || 1;
  const list = s.messages.map(m => ({ ...m, share: m.bytes / total }));
  sortBy(list, S.msgSort);
  tb.tBodies[0].innerHTML = list.map(m => {
    const stale = m.count > 2 && m.interval > 0 && m.lastAgo > Math.max(3 * m.interval, 5);
    const pct = (m.share * 100).toFixed(1);
    return `<tr class="${stale ? 'stale' : ''}" title="${stale ? '该消息已超过 3 倍平均间隔未更新' : ''}">
      <td><span class="tag" style="background:${msgColor(m.type)}"></span>${m.type}</td>
      <td>${esc(m.name)}</td>
      <td class="num">${fmtNum(m.count)}</td>
      <td class="num">${m.rate.toFixed(2)}</td>
      <td class="num">${m.interval ? m.interval.toFixed(2) : '-'}</td>
      <td class="num">${m.lastAgo.toFixed(1)}</td>
      <td class="num">${fmtBytes(m.bytes)}</td>
      <td><div class="bar-wrap"><div class="bar"><i style="width:${pct}%"></i></div>${pct}%</div></td>
      <td class="num">${m.minLen === m.maxLen ? m.maxLen : m.minLen + '–' + m.maxLen}</td>
      <td>${esc(m.info || '')}</td>
    </tr>`;
  }).join('') || '<tr><td colspan="10" class="empty">暂无消息</td></tr>';
}

// ---------- signals ----------

function renderSignals() {
  const s = S.stats;
  const systems = s ? s.systems : [];
  const sel = $('sysFilter');
  const opts = ['全部', ...systems.map(x => x.system)];
  if (sel.dataset.opts !== opts.join(',')) {
    const cur = sel.value;
    sel.innerHTML = opts.map(o => `<option>${o}</option>`).join('');
    sel.value = opts.includes(cur) ? cur : '全部';
    sel.dataset.opts = opts.join(',');
  }
  const f = sel.value || '全部';
  const sats = [];
  const summary = [];
  for (const x of systems) {
    if (f !== '全部' && f !== x.system) continue;
    let sum = 0, cnt = 0;
    for (const sat of x.sats || []) {
      const sigs = (sat.signals || []).filter(g => g.cnr > 0);
      sigs.forEach(g => { sum += g.cnr; cnt++; });
      if (sigs.length) sats.push({ system: x.system, prn: sat.prn, signals: sigs });
    }
    if (cnt) summary.push(`${x.system} 平均 ${(sum / cnt).toFixed(1)}`);
  }
  $('cnrSummary').textContent = summary.join(' · ');
  drawCnr($('cvCnr'), sats);
  drawLineChart($('cvSatHist'), {
    series: [...S.satHist].map(([k, d]) => ({ name: k, color: SYS_COLORS[k] || '#38bdf8', data: d })),
    length: HIST,
    fmt: v => v.toFixed(0),
    empty: '等待 MSM 观测消息',
  });
  $('ephBox').innerHTML = ephHtml(s ? s.ephemeris : []);
}

function ephHtml(list) {
  if (!list.length) return '<div class="empty">尚未收到星历消息（1019 / 1020 / 1042 / 1044 / 1045 / 1046）</div>';
  const groups = new Map();
  for (const e of list) {
    if (!groups.has(e.system)) groups.set(e.system, []);
    groups.get(e.system).push(e);
  }
  return [...groups].map(([sys, arr]) => `<div class="eph-sys">
    <b style="color:${SYS_COLORS[sys] || '#ccc'}">${sys}</b> <span class="muted">${arr.length} 颗</span>
    <div>${arr.map(e => `<span class="chip${e.ageSec > 600 ? ' old' : ''}" title="RTCM ${e.msgType} · 收到 ${e.count} 次 · ${e.ageSec.toFixed(0)} s 前">${e.prn}</span>`).join('')}</div>
  </div>`).join('');
}

// ---------- sourcetable ----------

const ST_COLS = [
  ['name', '挂载点'], ['identifier', '标识'], ['format', '格式'], ['formatDetails', '格式详情'], ['navSystem', '导航系统'],
  ['network', '网络'], ['country', '国家'], ['lat', '纬度'], ['lon', '经度'], ['distance', '距离 (km)'],
  ['nmea', 'GGA'], ['auth', '认证'], ['bitrate', '码率'],
];

function renderSourceTable() {
  const tb = $('stTable');
  if (!tb.tHead.rows.length) setupSortHeader(tb, ST_COLS, S.stSort, renderSourceTable);
  markSorted(tb, S.stSort);
  const t = S.table;
  if (!t) {
    tb.tBodies[0].innerHTML = `<tr><td colspan="${ST_COLS.length}" class="empty">填写 Caster 地址后点击左侧“获取源列表”</td></tr>`;
    $('stCount').textContent = '';
    S.stList = [];
    drawMap();
    return;
  }
  const lat = num('lat'), lon = num('lon');
  const hasRef = lat !== 0 || lon !== 0;
  const q = $('stFilter').value.trim().toLowerCase();
  let list = t.mounts.map(m => ({
    ...m,
    distance: hasRef && (m.lat || m.lon) ? haversine(lat, lon, m.lat, m.lon) : Infinity,
  }));
  if (q) {
    list = list.filter(m => [m.name, m.identifier, m.format, m.formatDetails, m.navSystem, m.network, m.country]
      .join(' ').toLowerCase().includes(q));
  }
  sortBy(list, S.stSort);
  S.stList = list;
  $('stCount').textContent = `${list.length} / ${t.mounts.length} 个`;
  tb.tBodies[0].innerHTML = list.map(m => `<tr data-name="${esc(m.name)}" class="${m.name === S.selectedMount ? 'sel' : ''}">
    <td><b>${esc(m.name)}</b></td>
    <td>${esc(m.identifier)}</td>
    <td>${esc(m.format)}</td>
    <td class="wrap">${esc(m.formatDetails)}</td>
    <td>${esc(m.navSystem)}</td>
    <td>${esc(m.network)}</td>
    <td>${esc(m.country)}</td>
    <td class="num">${m.lat.toFixed(2)}</td>
    <td class="num">${m.lon.toFixed(2)}</td>
    <td class="num">${isFinite(m.distance) ? m.distance.toFixed(1) : '-'}</td>
    <td>${m.nmea ? '需要' : '-'}</td>
    <td>${esc(m.auth)}</td>
    <td class="num">${m.bitrate || '-'}</td>
  </tr>`).join('') || `<tr><td colspan="${ST_COLS.length}" class="empty">没有匹配的挂载点</td></tr>`;
  drawMap();
}

function drawMap() {
  if (S.tab !== 'sourcetable') return;
  const lat = num('lat'), lon = num('lon');
  drawMountMap($('cvMap'), S.stList, lat || lon ? { lat, lon } : null, S.selectedMount);
}

// ---------- console ----------

function onConsole(lines) {
  if ($('cPause').checked) return;
  for (const l of lines) S.consoleLines.push(l);
  if (S.consoleLines.length > MAX_CONSOLE) S.consoleLines.splice(0, S.consoleLines.length - MAX_CONSOLE);
  if (S.tab === 'console') appendConsole(lines);
}

function lineEl(l, hex) {
  const d = document.createElement('div');
  if (l.kind === 'warn') {
    d.className = 'ln warn';
    d.textContent = `${l.time}  ⚠ ${l.summary}`;
    return d;
  }
  d.className = 'ln';
  d.innerHTML = `<span class="t">${l.time}</span> <span class="ty">${l.type}</span> <span class="nm">${esc(l.name)}</span> <span class="sz">[${l.len} B]</span> ${esc(l.summary)}`
    + (hex ? `<span class="hex">${esc(l.hex)}</span>` : '');
  return d;
}

function appendConsole(lines) {
  const box = $('consoleBox');
  const q = $('cFilter').value.trim().toLowerCase();
  const hex = $('cHex').checked;
  const stick = box.scrollTop + box.clientHeight >= box.scrollHeight - 30;
  const frag = document.createDocumentFragment();
  for (const l of lines) {
    if (q && !`${l.type} ${l.name} ${l.summary}`.toLowerCase().includes(q)) continue;
    frag.appendChild(lineEl(l, hex));
  }
  box.appendChild(frag);
  while (box.childElementCount > MAX_CONSOLE) box.firstChild.remove();
  if (stick) box.scrollTop = box.scrollHeight;
  $('cInfo').textContent = `缓存 ${S.consoleLines.length} 行`;
}

function rebuildConsole() {
  $('consoleBox').innerHTML = '';
  appendConsole(S.consoleLines);
}

// ---------- tabs & init ----------

function switchTab(tab) {
  S.tab = tab;
  document.querySelectorAll('.tab').forEach(b => b.classList.toggle('active', b.dataset.tab === tab));
  document.querySelectorAll('.page').forEach(p => p.classList.toggle('active', p.id === 'tab-' + tab));
  if (tab === 'console') rebuildConsole();
  else if (tab === 'sourcetable') renderSourceTable();
  else render();
}

function init() {
  if (!window.runtime || !window.go || !window.go.main) { setTimeout(init, 50); return; }
  loadCfg();

  document.querySelectorAll('.tab').forEach(b => { b.onclick = () => switchTab(b.dataset.tab); });
  $('btnTable').onclick = fetchTable;
  $('btnConnect').onclick = connect;
  $('btnDisconnect').onclick = disconnect;
  $('btnRecord').onclick = toggleRecord;
  $('btnReset').onclick = resetStats;
  $('btnUseArp').onclick = useArp;
  $('btnUseMount').onclick = useMount;
  $('btnClearLog').onclick = () => { $('eventLog').innerHTML = ''; };
  $('mountpoint').addEventListener('keydown', e => { if (e.key === 'Enter') connect(); });
  $('mountpoint').addEventListener('change', () => { S.selectedMount = $('mountpoint').value.trim(); saveCfg(); });
  for (const id of ['host', 'port', 'version', 'username', 'password', 'ggaInterval', 'timeout', 'sep', 'tls', 'selfSign', 'autoReconnect', 'savePwd']) {
    $(id).addEventListener('change', saveCfg);
  }
  for (const id of ['lat', 'lon', 'alt']) {
    $(id).addEventListener('change', () => {
      saveCfg();
      pushReference();
      if (S.tab === 'sourcetable') renderSourceTable();
    });
  }

  $('sysFilter').onchange = () => renderSignals();
  $('stFilter').oninput = () => renderSourceTable();
  $('cFilter').oninput = () => rebuildConsole();
  $('cHex').onchange = () => rebuildConsole();
  $('cClear').onclick = () => { S.consoleLines = []; rebuildConsole(); };

  const stBody = $('stTable').tBodies[0];
  stBody.addEventListener('click', e => {
    const tr = e.target.closest('tr[data-name]');
    if (tr) selectMount(tr.dataset.name, false);
  });
  stBody.addEventListener('dblclick', e => {
    const tr = e.target.closest('tr[data-name]');
    if (tr) { selectMount(tr.dataset.name, false); connect(); }
  });

  const map = $('cvMap');
  map.addEventListener('click', e => {
    const r = map.getBoundingClientRect();
    const x = e.clientX - r.left, y = e.clientY - r.top;
    let best = null, bd = 100;
    for (const p of map._pts || []) {
      const d = (p.x - x) ** 2 + (p.y - y) ** 2;
      if (d < bd) { bd = d; best = p; }
    }
    if (best) selectMount(best.name, true);
  });
  map.addEventListener('mousemove', e => {
    const r = map.getBoundingClientRect();
    const x = e.clientX - r.left, y = e.clientY - r.top;
    const hit = (map._pts || []).find(p => (p.x - x) ** 2 + (p.y - y) ** 2 < 64);
    map.title = hit ? hit.name : '';
  });

  const cnr = $('cvCnr');
  cnr.addEventListener('mousemove', e => {
    const x = e.clientX - cnr.getBoundingClientRect().left;
    const hit = (cnr._hit || []).find(o => x >= o.x0 && x < o.x1);
    cnr.title = hit ? `${hit.sat.prn}\n` + hit.sat.signals.map(g => `${g.code}: ${g.cnr.toFixed(1)} dB-Hz`).join('\n') : '';
  });

  window.runtime.EventsOn('stats', onStats);
  window.runtime.EventsOn('console', onConsole);
  window.runtime.EventsOn('log', e => addLog(e.level, e.message, e.time));
  window.runtime.EventsOn('state', e => updateState(e.state, e.message));

  let resizeTimer = 0;
  window.addEventListener('resize', () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => (S.tab === 'sourcetable' ? drawMap() : render()), 100);
  });

  S.selectedMount = $('mountpoint').value.trim();
  pushReference();
  switchTab('overview');
  addLog('info', '就绪。填写 Caster 地址，获取源列表并选择挂载点后连接。');
}

document.addEventListener('DOMContentLoaded', init);

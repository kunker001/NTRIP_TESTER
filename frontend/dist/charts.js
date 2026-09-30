'use strict';

const SYS_COLORS = { GPS: '#4ade80', GLO: '#f87171', GAL: '#60a5fa', BDS: '#fbbf24', QZSS: '#c084fc', SBAS: '#94a3b8', NavIC: '#f472b6' };
const GRID = '#223046';
const AXIS = '#7d8aa3';
const CARD_BG = '#161f2c';

function hexA(hex, a) {
  const n = parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},${a})`;
}

function prepCanvas(canvas) {
  const dpr = window.devicePixelRatio || 1;
  const w = Math.max(10, canvas.clientWidth);
  const h = Math.max(10, canvas.clientHeight);
  const W = Math.round(w * dpr), H = Math.round(h * dpr);
  if (canvas.width !== W || canvas.height !== H) { canvas.width = W; canvas.height = H; }
  const ctx = canvas.getContext('2d');
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);
  ctx.font = '11px "Segoe UI", "Microsoft YaHei", sans-serif';
  return { ctx, w, h };
}

function niceMax(v) {
  if (!(v > 0)) return 1;
  const p = Math.pow(10, Math.floor(Math.log10(v)));
  const n = v / p;
  return (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * p;
}

function niceStep(v) {
  if (!(v > 0)) return 1;
  const p = Math.pow(10, Math.floor(Math.log10(v)));
  const n = v / p;
  return (n < 1.5 ? 1 : n < 3.5 ? 2 : n < 7.5 ? 5 : 10) * p;
}

function drawEmpty(ctx, w, h, text) {
  ctx.fillStyle = AXIS;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(text || '暂无数据', w / 2, h / 2);
}

function clipText(ctx, t, maxW) {
  if (ctx.measureText(t).width <= maxW) return t;
  while (t.length > 1 && ctx.measureText(t + '…').width > maxW) t = t.slice(0, -1);
  return t + '…';
}

function drawTimeAxis(ctx, x0, pw, y, n) {
  ctx.fillStyle = AXIS;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  for (let i = 0; i <= 5; i++) {
    const sec = Math.round(n - (n * i) / 5);
    ctx.fillText(sec ? `-${sec}s` : '现在', x0 + (pw * i) / 5, y);
  }
}

// opt: { series: [{name, color, data, fill}], length, fmt, empty }
function drawLineChart(canvas, opt) {
  const { ctx, w, h } = prepCanvas(canvas);
  const series = opt.series.filter(s => s.data && s.data.length);
  if (!series.length) return drawEmpty(ctx, w, h, opt.empty);
  const n = opt.length;
  const pad = { l: 46, r: 14, t: 22, b: 20 };
  const pw = w - pad.l - pad.r, ph = h - pad.t - pad.b;
  const fmt = opt.fmt || (v => String(v));
  let max = 0;
  for (const s of series) for (const v of s.data) if (v > max) max = v;
  max = niceMax(max * 1.05);

  ctx.lineWidth = 1;
  ctx.textAlign = 'right';
  ctx.textBaseline = 'middle';
  for (let i = 0; i <= 4; i++) {
    const y = Math.round(pad.t + ph - (ph * i) / 4) + 0.5;
    ctx.strokeStyle = GRID;
    ctx.beginPath(); ctx.moveTo(pad.l, y); ctx.lineTo(w - pad.r, y); ctx.stroke();
    ctx.fillStyle = AXIS;
    ctx.fillText(fmt((max * i) / 4), pad.l - 6, y);
  }
  drawTimeAxis(ctx, pad.l, pw, h - pad.b + 5, n);

  for (const s of series) {
    const data = s.data.slice(-n);
    const len = data.length;
    const xAt = j => pad.l + (pw * (n - len + j)) / Math.max(1, n - 1);
    const yAt = v => pad.t + ph - (ph * Math.min(v, max)) / max;
    ctx.beginPath();
    data.forEach((v, j) => (j ? ctx.lineTo(xAt(j), yAt(v)) : ctx.moveTo(xAt(j), yAt(v))));
    ctx.strokeStyle = s.color;
    ctx.lineWidth = 1.6;
    ctx.stroke();
    if (s.fill && len > 1) {
      ctx.lineTo(xAt(len - 1), pad.t + ph);
      ctx.lineTo(xAt(0), pad.t + ph);
      ctx.closePath();
      const g = ctx.createLinearGradient(0, pad.t, 0, pad.t + ph);
      g.addColorStop(0, hexA(s.color, 0.35));
      g.addColorStop(1, hexA(s.color, 0));
      ctx.fillStyle = g;
      ctx.fill();
    }
  }

  let lx = pad.l + 4;
  ctx.textAlign = 'left';
  ctx.textBaseline = 'middle';
  for (const s of series) {
    const label = `${s.name} ${fmt(s.data[s.data.length - 1])}`;
    ctx.fillStyle = s.color;
    ctx.fillRect(lx, 8, 10, 3);
    ctx.fillStyle = '#cbd5e1';
    ctx.fillText(label, lx + 14, 10);
    lx += ctx.measureText(label).width + 28;
  }
}

// rows: [{label, color, cells: [count per second], right}]
function drawTimeline(canvas, rows, n) {
  const { ctx, w, h } = prepCanvas(canvas);
  if (!rows.length) return drawEmpty(ctx, w, h, '等待 RTCM 消息…');
  const padL = 190, padR = 58, padT = 4, padB = 18;
  const rh = (h - padT - padB) / rows.length;
  const pw = w - padL - padR;
  const cw = pw / n;
  rows.forEach((r, i) => {
    const y = padT + i * rh;
    if (i % 2 === 0) { ctx.fillStyle = 'rgba(255,255,255,0.025)'; ctx.fillRect(padL, y, pw, rh); }
    ctx.fillStyle = r.color;
    ctx.fillRect(4, y + rh / 2 - 3, 6, 6);
    ctx.fillStyle = '#cbd5e1';
    ctx.textAlign = 'left';
    ctx.textBaseline = 'middle';
    ctx.fillText(clipText(ctx, r.label, padL - 22), 16, y + rh / 2);
    const cells = r.cells.slice(-n);
    const off = n - cells.length;
    ctx.fillStyle = r.color;
    for (let j = 0; j < cells.length; j++) {
      const c = cells[j];
      if (!c) continue;
      ctx.globalAlpha = Math.min(1, 0.45 + 0.15 * c);
      ctx.fillRect(padL + (off + j) * cw + 0.5, y + 2, Math.max(1, cw - 1), Math.max(2, rh - 4));
    }
    ctx.globalAlpha = 1;
    ctx.fillStyle = AXIS;
    ctx.fillText(r.right || '', w - padR + 6, y + rh / 2);
  });
  drawTimeAxis(ctx, padL, pw, h - padB + 4, n);
}

// items: [{label, value, color}], center: [big, small]
function drawDonut(canvas, items, center) {
  const { ctx, w, h } = prepCanvas(canvas);
  const total = items.reduce((a, b) => a + b.value, 0);
  if (!total) return drawEmpty(ctx, w, h, '暂无数据');
  const r = Math.max(20, Math.min(h / 2 - 10, w * 0.2));
  const cx = r + 16, cy = h / 2;
  let a0 = -Math.PI / 2;
  ctx.strokeStyle = CARD_BG;
  ctx.lineWidth = 1.5;
  for (const it of items) {
    const a1 = a0 + (it.value / total) * Math.PI * 2;
    ctx.beginPath(); ctx.moveTo(cx, cy); ctx.arc(cx, cy, r, a0, a1); ctx.closePath();
    ctx.fillStyle = it.color; ctx.fill(); ctx.stroke();
    a0 = a1;
  }
  ctx.beginPath(); ctx.arc(cx, cy, r * 0.6, 0, Math.PI * 2); ctx.fillStyle = CARD_BG; ctx.fill();
  ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
  ctx.fillStyle = '#e2e8f0'; ctx.font = '600 13px "Segoe UI", sans-serif';
  ctx.fillText(center[0], cx, cy - 7);
  ctx.fillStyle = AXIS; ctx.font = '11px "Segoe UI", "Microsoft YaHei", sans-serif';
  ctx.fillText(center[1], cx, cy + 10);

  const sorted = [...items].sort((a, b) => b.value - a.value);
  const lx = cx + r + 24;
  const maxRows = Math.max(1, Math.floor((h - 8) / 16));
  const shown = sorted.length > maxRows ? sorted.slice(0, maxRows - 1) : sorted;
  if (sorted.length > maxRows) {
    const rest = sorted.slice(maxRows - 1).reduce((a, b) => a + b.value, 0);
    shown.push({ label: `其他 ${sorted.length - maxRows + 1} 种`, value: rest, color: '#475569' });
  }
  const y0 = (h - shown.length * 16) / 2 + 8;
  ctx.textBaseline = 'middle';
  shown.forEach((it, i) => {
    const y = y0 + i * 16;
    ctx.fillStyle = it.color; ctx.fillRect(lx, y - 4, 8, 8);
    ctx.fillStyle = '#cbd5e1'; ctx.textAlign = 'left';
    ctx.fillText(clipText(ctx, it.label, w - lx - 70), lx + 14, y);
    ctx.fillStyle = AXIS; ctx.textAlign = 'right';
    ctx.fillText(((it.value / total) * 100).toFixed(1) + '%', w - 8, y);
  });
}

// sats: [{system, prn, signals: [{code, cnr}]}]
function drawCnr(canvas, sats) {
  const parentW = canvas.parentElement.clientWidth;
  canvas.style.width = Math.max(parentW, 50 + sats.length * 30) + 'px';
  const { ctx, w, h } = prepCanvas(canvas);
  canvas._hit = [];
  if (!sats.length) return drawEmpty(ctx, w, h, '等待 MSM4/5/6/7 观测消息（MSM1-3 不含载噪比）');
  const pad = { l: 36, r: 8, t: 12, b: 34 };
  const pw = w - pad.l - pad.r, ph = h - pad.t - pad.b, max = 60;
  ctx.lineWidth = 1;
  ctx.textAlign = 'right';
  ctx.textBaseline = 'middle';
  for (let v = 0; v <= max; v += 10) {
    const y = Math.round(pad.t + ph - (ph * v) / max) + 0.5;
    ctx.strokeStyle = v === 40 ? '#334155' : GRID;
    ctx.setLineDash(v === 40 ? [4, 3] : []);
    ctx.beginPath(); ctx.moveTo(pad.l, y); ctx.lineTo(w - pad.r, y); ctx.stroke();
    ctx.fillStyle = AXIS; ctx.fillText(String(v), pad.l - 6, y);
  }
  ctx.setLineDash([]);
  const gw = pw / sats.length;
  sats.forEach((s, i) => {
    const x0 = pad.l + i * gw;
    const color = SYS_COLORS[s.system] || '#38bdf8';
    const nsig = Math.max(1, s.signals.length);
    const bw = Math.max(2, (gw - 6) / nsig);
    let best = 0;
    s.signals.forEach((g, k) => {
      const bh = (ph * Math.min(g.cnr, max)) / max;
      ctx.fillStyle = hexA(color, Math.max(0.3, 1 - k * 0.22));
      ctx.fillRect(x0 + 3 + k * bw, pad.t + ph - bh, Math.max(1, bw - 1), bh);
      best = Math.max(best, g.cnr);
    });
    ctx.textAlign = 'center';
    ctx.textBaseline = 'top';
    ctx.fillStyle = '#cbd5e1';
    ctx.fillText(s.prn, x0 + gw / 2, pad.t + ph + 4);
    ctx.fillStyle = AXIS;
    ctx.fillText(best.toFixed(0), x0 + gw / 2, pad.t + ph + 18);
    canvas._hit.push({ x0, x1: x0 + gw, sat: s });
  });
}

// mounts: [{name, lat, lon, nmea}], ref: {lat, lon} | null
function drawMountMap(canvas, mounts, ref, selected) {
  const { ctx, w, h } = prepCanvas(canvas);
  canvas._pts = [];
  const pts = mounts.filter(m => m.lat || m.lon);
  if (!pts.length) return drawEmpty(ctx, w, h, '没有带坐标的挂载点');
  let minLat = Infinity, maxLat = -Infinity, minLon = Infinity, maxLon = -Infinity;
  const grow = (la, lo) => {
    minLat = Math.min(minLat, la); maxLat = Math.max(maxLat, la);
    minLon = Math.min(minLon, lo); maxLon = Math.max(maxLon, lo);
  };
  pts.forEach(m => grow(m.lat, m.lon));
  if (ref) grow(ref.lat, ref.lon);
  const padLat = Math.max(0.25, (maxLat - minLat) * 0.08);
  const padLon = Math.max(0.25, (maxLon - minLon) * 0.08);
  minLat = Math.max(-89, minLat - padLat); maxLat = Math.min(89, maxLat + padLat);
  minLon -= padLon; maxLon += padLon;
  const kx = Math.cos((((minLat + maxLat) / 2) * Math.PI) / 180);
  const spanX = (maxLon - minLon) * kx, spanY = maxLat - minLat;
  const scale = Math.min((w - 24) / spanX, (h - 24) / spanY);
  const ox = (w - spanX * scale) / 2, oy = (h - spanY * scale) / 2;
  const P = (la, lo) => [ox + (lo - minLon) * kx * scale, oy + (maxLat - la) * scale];

  const step = niceStep(Math.max(spanY, maxLon - minLon) / 6);
  ctx.strokeStyle = '#1b2638';
  ctx.lineWidth = 1;
  ctx.fillStyle = '#4b5b75';
  ctx.textBaseline = 'top';
  ctx.textAlign = 'left';
  for (let la = Math.ceil(minLat / step) * step; la <= maxLat; la += step) {
    const [, y] = P(la, minLon);
    ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke();
    ctx.fillText(la.toFixed(step < 1 ? 1 : 0) + '°', 4, y + 2);
  }
  for (let lo = Math.ceil(minLon / step) * step; lo <= maxLon; lo += step) {
    const [x] = P(minLat, lo);
    ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, h); ctx.stroke();
    ctx.fillText(lo.toFixed(step < 1 ? 1 : 0) + '°', x + 2, h - 14);
  }

  let sel = null;
  for (const m of pts) {
    const [x, y] = P(m.lat, m.lon);
    canvas._pts.push({ x, y, name: m.name });
    if (m.name === selected) { sel = { x, y, m }; continue; }
    ctx.fillStyle = m.nmea ? '#38bdf8' : '#4ade80';
    ctx.globalAlpha = 0.8;
    ctx.beginPath(); ctx.arc(x, y, 2.5, 0, Math.PI * 2); ctx.fill();
  }
  ctx.globalAlpha = 1;
  if (ref) {
    const [x, y] = P(ref.lat, ref.lon);
    ctx.strokeStyle = '#f87171'; ctx.lineWidth = 2;
    ctx.beginPath(); ctx.moveTo(x - 7, y); ctx.lineTo(x + 7, y); ctx.moveTo(x, y - 7); ctx.lineTo(x, y + 7); ctx.stroke();
    if (sel) {
      ctx.setLineDash([4, 3]); ctx.lineWidth = 1;
      ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(sel.x, sel.y); ctx.stroke(); ctx.setLineDash([]);
    }
  }
  if (sel) {
    ctx.fillStyle = '#fbbf24';
    ctx.beginPath(); ctx.arc(sel.x, sel.y, 5, 0, Math.PI * 2); ctx.fill();
    ctx.font = '600 12px "Segoe UI", sans-serif';
    ctx.textBaseline = 'bottom';
    ctx.textAlign = sel.x > w - 100 ? 'right' : 'left';
    ctx.fillText(sel.m.name, sel.x + (ctx.textAlign === 'right' ? -8 : 8), sel.y - 4);
  }
  ctx.font = '11px "Segoe UI", "Microsoft YaHei", sans-serif';
  ctx.textAlign = 'right'; ctx.textBaseline = 'top';
  const legend = [['#38bdf8', '需要 GGA'], ['#4ade80', '无需 GGA'], ['#fbbf24', '已选择'], ['#f87171', '参考位置']];
  legend.forEach(([c, t], i) => {
    ctx.fillStyle = c; ctx.fillRect(w - 76, 8 + i * 15, 8, 8);
    ctx.fillStyle = '#cbd5e1'; ctx.textAlign = 'left'; ctx.fillText(t, w - 64, 6 + i * 15);
  });
}

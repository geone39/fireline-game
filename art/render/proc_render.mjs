#!/usr/bin/env node
// Процедурный рендерер пайплайна ART-2 (Node >= 18, без пакетов): модели из models.json -> кадры PNG по одному на файл.
// Запасной путь и проверки CI; 4 направления юнита. Основан на art/spike/procedural/render.mjs (ART-1).
// Упаковку в атласы и манифест делает art/tools/build_art.py.
//   node art/render/proc_render.mjs --models art/src/units/models.json --palette art/src/palette.json --out DIR [--ids a,b] [--ss 4]
// Результат: DIR/<id>/frames/<имя>.png и DIR/<id>/meta.json (формат тот же, что у blender_render.py).
import fs from 'node:fs';
import path from 'node:path';
import zlib from 'node:zlib';

const args = process.argv.slice(2);
const opt = (k, d) => (args.includes(k) ? args[args.indexOf(k) + 1] : d);
const OUT = path.resolve(opt('--out', ''));
const ONLY = new Set(opt('--ids', '').split(',').filter(Boolean));
const SS = Number(opt('--ss', 4)); // суперсэмплинг
const DIRS = 4; // направления юнита (ТЗ: 4 для процедурного пути)
const OUTLINE_PX = { unit: 1.0, building: 1.6 };

const pal = JSON.parse(fs.readFileSync(opt('--palette'), 'utf8'));
const models = JSON.parse(fs.readFileSync(opt('--models'), 'utf8')).models;
const hex = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16));
const col = (k) => {
  if (!pal[k]) throw new Error(`no palette colour ${k}`);
  return hex(pal[k]);
};

// ---------- проекция: ребро тайла по x/y -> (32,16) px; единица z -> ZPX px ----------
const TILE_HALF_W = 32;
const TILE_HALF_H = 16;
const ZPX = (TILE_HALF_W * Math.SQRT2) * Math.cos(Math.PI / 6); // 39.19: вертикаль при угле камеры 30°
const VCAM = [Math.cos(Math.PI / 6) * Math.SQRT1_2, Math.cos(Math.PI / 6) * Math.SQRT1_2, 0.5]; // к камере
const L = (() => {
  const d = pal.light.dir;
  const n = Math.hypot(...d);
  return d.map((v) => v / n);
})();

// ---------- линейная алгебра (аффинные 3x4) ----------
const I = () => [1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0];
const mul = (a, b) => {
  const r = new Array(12);
  for (let i = 0; i < 3; i++) {
    for (let j = 0; j < 3; j++) r[i * 4 + j] = a[i * 4] * b[j] + a[i * 4 + 1] * b[4 + j] + a[i * 4 + 2] * b[8 + j];
    r[i * 4 + 3] = a[i * 4] * b[3] + a[i * 4 + 1] * b[7] + a[i * 4 + 2] * b[11] + a[i * 4 + 3];
  }
  return r;
};
const T = (x, y, z) => [1, 0, 0, x, 0, 1, 0, y, 0, 0, 1, z];
const rad = (d) => (d * Math.PI) / 180;
const Rx = (a) => [1, 0, 0, 0, 0, Math.cos(a), -Math.sin(a), 0, 0, Math.sin(a), Math.cos(a), 0];
const Ry = (a) => [Math.cos(a), 0, Math.sin(a), 0, 0, 1, 0, 0, -Math.sin(a), 0, Math.cos(a), 0];
const Rz = (a) => [Math.cos(a), -Math.sin(a), 0, 0, Math.sin(a), Math.cos(a), 0, 0, 0, 0, 1, 0];
const euler = (rx, ry, rz) => mul(Rz(rad(rz)), mul(Ry(rad(ry)), Rx(rad(rx)))); // XYZ как в Blender
const xf = (m, p) => [
  m[0] * p[0] + m[1] * p[1] + m[2] * p[2] + m[3],
  m[4] * p[0] + m[5] * p[1] + m[6] * p[2] + m[7],
  m[8] * p[0] + m[9] * p[1] + m[10] * p[2] + m[11],
];
const sub = (a, b) => [a[0] - b[0], a[1] - b[1], a[2] - b[2]];
const cross = (a, b) => [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]];
const dot = (a, b) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
const norm = (a) => {
  const l = Math.hypot(...a) || 1;
  return a.map((v) => v / l);
};

// ---------- меши (локальные, центр в 0) ----------
const SEG = 12;
function meshBox(sx, sy, sz) {
  const x = sx / 2, y = sy / 2, z = sz / 2;
  const v = [[-x, -y, -z], [x, -y, -z], [x, y, -z], [-x, y, -z], [-x, -y, z], [x, -y, z], [x, y, z], [-x, y, z]];
  const q = [[0, 3, 2, 1], [4, 5, 6, 7], [0, 1, 5, 4], [2, 3, 7, 6], [1, 2, 6, 5], [3, 0, 4, 7]];
  const tris = [];
  for (const f of q) tris.push([v[f[0]], v[f[1]], v[f[2]]], [v[f[0]], v[f[2]], v[f[3]]]);
  return tris;
}
function meshCyl(sx, sy, sz, cone) {
  const rx = sx / 2, ry = sy / 2, z = sz / 2, tris = [];
  const ring = (zz, k) => Array.from({ length: SEG }, (_, i) => [Math.cos((i / SEG) * 2 * Math.PI) * rx * k, Math.sin((i / SEG) * 2 * Math.PI) * ry * k, zz]);
  const lo = ring(-z, 1), hi = ring(z, cone ? 0 : 1);
  for (let i = 0; i < SEG; i++) {
    const j = (i + 1) % SEG;
    if (cone) tris.push([lo[i], lo[j], [0, 0, z]]);
    else tris.push([lo[i], lo[j], hi[j]], [lo[i], hi[j], hi[i]]);
    tris.push([[0, 0, -z], lo[j], lo[i]]);
    if (!cone) tris.push([[0, 0, z], hi[i], hi[j]]);
  }
  return tris;
}
function meshSphere(sx, sy, sz) {
  const RINGS = 6, SEGS = 10, tris = [];
  const p = (r, s) => {
    const th = (r / RINGS) * Math.PI, ph = (s / SEGS) * 2 * Math.PI;
    return [Math.sin(th) * Math.cos(ph) * sx / 2, Math.sin(th) * Math.sin(ph) * sy / 2, Math.cos(th) * sz / 2];
  };
  for (let r = 0; r < RINGS; r++) {
    for (let s = 0; s < SEGS; s++) {
      if (r > 0) tris.push([p(r, s), p(r, s + 1), p(r + 1, s + 1)]);
      if (r < RINGS - 1) tris.push([p(r, s), p(r + 1, s + 1), p(r + 1, s)]);
    }
  }
  return tris;
}
const meshOf = (pt) =>
  pt.shape === 'box' ? meshBox(...pt.s) : pt.shape === 'sphere' ? meshSphere(...pt.s) : meshCyl(...pt.s, pt.shape === 'cone');

// ---------- позы ----------
function groupMatrices(model, pose) {
  const cache = {};
  const get = (name) => {
    if (cache[name]) return cache[name];
    const g = model.groups[name];
    const p = pose[name] || {};
    const piv = g.pivot;
    let m = mul(T(piv[0] + (p.dx || 0), piv[1] + (p.dy || 0), piv[2] + (p.dz || 0)), mul(euler(p.rx || 0, p.ry || 0, p.rz || 0), T(-piv[0], -piv[1], -piv[2])));
    if (g.parent) m = mul(get(g.parent), m);
    return (cache[name] = m);
  };
  return get;
}
const framePose = (model, anim, i) => {
  const out = JSON.parse(JSON.stringify(model.rest || {}));
  for (const [g, v] of Object.entries(anim.frames[i])) if (g[0] !== '_') out[g] = { ...(out[g] || {}), ...v };
  return out;
};

// ---------- растеризатор ----------
// падение в мировых осях: поворот на fallDeg вокруг (1,-1,0)/√2 (верх тела уходит от камеры, к (-1,-1))
function axisRot(a, th) {
  const [x, y, z] = a, c = Math.cos(th), s = Math.sin(th), t = 1 - c;
  return [t * x * x + c, t * x * y - s * z, t * x * z + s * y, 0, t * x * y + s * z, t * y * y + c, t * y * z - s * x, 0, t * x * z - s * y, t * y * z + s * x, t * z * z + c, 0];
}

function renderScene(model, pose, yawDeg, frameW, frameH, fallDeg = 0) {
  const [ax, ay] = model.anchor;
  const W = frameW * SS, H = frameH * SS;
  const depth = new Float32Array(W * H).fill(-1e9);
  const idb = new Int32Array(W * H).fill(-1);
  const rgb = new Uint8Array(W * H * 3);
  const yawM = mul(axisRot([Math.SQRT1_2, -Math.SQRT1_2, 0], rad(fallDeg)), Rz(rad(yawDeg)));
  const getG = model.groups ? groupMatrices(model, pose) : null;
  const proj = (p) => [(ax + (p[0] - p[1]) * TILE_HALF_W) * SS, (ay + (p[0] + p[1]) * TILE_HALF_H - p[2] * ZPX) * SS];
  const bbox = [1e9, 1e9, -1e9, -1e9];
  model.parts.forEach((pt, pi) => {
    let m = mul(T(...pt.c), euler(...(pt.rot || [0, 0, 0])));
    if (pt.g) m = mul(getG(pt.g), m);
    m = mul(yawM, m);
    const base = col(pt.col);
    for (const tri of meshOf(pt)) {
      const w = tri.map((v) => xf(m, v));
      const n = norm(cross(sub(w[1], w[0]), sub(w[2], w[0])));
      if (dot(n, VCAM) <= 1e-6) continue; // задняя грань
      const nl = dot(n, L);
      const band = pal.light.bands.find((b) => nl >= b.min).mul;
      const c = base.map((v, k) => Math.min(255, Math.round(v * band[k])));
      const cls = Math.abs(n[2]) > 0.7 ? 1 : 0; // верх / бок: граница между ними рисуется линией
      const id = pi * 2 + cls;
      const s = w.map(proj);
      const d = w.map((p) => dot(p, VCAM));
      for (const q of s) {
        bbox[0] = Math.min(bbox[0], q[0] / SS); bbox[1] = Math.min(bbox[1], q[1] / SS);
        bbox[2] = Math.max(bbox[2], q[0] / SS); bbox[3] = Math.max(bbox[3], q[1] / SS);
      }
      const minx = Math.max(0, Math.floor(Math.min(s[0][0], s[1][0], s[2][0]))), maxx = Math.min(W - 1, Math.ceil(Math.max(s[0][0], s[1][0], s[2][0])));
      const miny = Math.max(0, Math.floor(Math.min(s[0][1], s[1][1], s[2][1]))), maxy = Math.min(H - 1, Math.ceil(Math.max(s[0][1], s[1][1], s[2][1])));
      const area = (s[1][0] - s[0][0]) * (s[2][1] - s[0][1]) - (s[2][0] - s[0][0]) * (s[1][1] - s[0][1]);
      if (Math.abs(area) < 1e-9) continue;
      for (let y = miny; y <= maxy; y++) {
        for (let x = minx; x <= maxx; x++) {
          const px = x + 0.5, py = y + 0.5;
          const w0 = ((s[1][0] - px) * (s[2][1] - py) - (s[2][0] - px) * (s[1][1] - py)) / area;
          const w1 = ((s[2][0] - px) * (s[0][1] - py) - (s[0][0] - px) * (s[2][1] - py)) / area;
          const w2 = 1 - w0 - w1;
          if (w0 < -1e-6 || w1 < -1e-6 || w2 < -1e-6) continue;
          const dd = w0 * d[0] + w1 * d[1] + w2 * d[2];
          const i = y * W + x;
          if (dd <= depth[i]) continue;
          depth[i] = dd; idb[i] = id;
          rgb[i * 3] = c[0]; rgb[i * 3 + 1] = c[1]; rgb[i * 3 + 2] = c[2];
        }
      }
    }
  });
  return { W, H, depth, idb, rgb, bbox };
}

function finish(sc, kind, shadow) {
  const { W, H, idb, rgb } = sc;
  const out = new Uint8ClampedArray(W * H * 4); // RGBA, прямая альфа на суперсэмпле
  const oc = col('outline');
  // тень-пятно под юнитом (до спрайта)
  if (shadow) {
    const [cx, cy, rx, ry] = shadow;
    for (let y = 0; y < H; y++) for (let x = 0; x < W; x++) {
      if ((((x + 0.5) - cx * SS) / (rx * SS)) ** 2 + (((y + 0.5) - cy * SS) / (ry * SS)) ** 2 <= 1) {
        const i = (y * W + x) * 4; out[i] = 20; out[i + 1] = 24; out[i + 2] = 20; out[i + 3] = 70;
      }
    }
  }
  const has = (x, y) => x >= 0 && y >= 0 && x < W && y < H && idb[y * W + x] >= 0;
  // внутренние линии: граница между деталями / верхом и боком
  const r = Math.max(1, Math.round(SS * 0.2));
  for (let y = 0; y < H; y++) for (let x = 0; x < W; x++) {
    const k = y * W + x;
    if (idb[k] < 0) continue;
    let edge = false;
    for (let dy = -r; dy <= r && !edge; dy++) for (let dx = -r; dx <= r; dx++) {
      const xx = x + dx, yy = y + dy;
      if (xx < 0 || yy < 0 || xx >= W || yy >= H) continue;
      const o = idb[yy * W + xx];
      if (o >= 0 && o !== idb[k] && (dx > 0 || dy > 0 || (dx === 0 && dy === 0)) && true) { edge = true; break; }
    }
    const f = edge ? 0.55 : 1;
    out[k * 4] = rgb[k * 3] * f; out[k * 4 + 1] = rgb[k * 3 + 1] * f; out[k * 4 + 2] = rgb[k * 3 + 2] * f; out[k * 4 + 3] = 255;
  }
  // силуэтная обводка: дилатация альфы
  const R = OUTLINE_PX[kind] * SS;
  const offs = [];
  for (let dy = -Math.ceil(R); dy <= Math.ceil(R); dy++) for (let dx = -Math.ceil(R); dx <= Math.ceil(R); dx++) if (dx * dx + dy * dy <= R * R) offs.push([dx, dy]);
  const ring = new Uint8Array(W * H);
  for (let y = 0; y < H; y++) for (let x = 0; x < W; x++) {
    if (idb[y * W + x] >= 0) continue;
    for (const [dx, dy] of offs) if (has(x + dx, y + dy)) { ring[y * W + x] = 1; break; }
  }
  for (let k = 0; k < W * H; k++) if (ring[k]) { out[k * 4] = oc[0]; out[k * 4 + 1] = oc[1]; out[k * 4 + 2] = oc[2]; out[k * 4 + 3] = 255; }
  return out;
}

// SS -> 1x, усреднение с премультиплицированной альфой
function downsample(buf, W, H) {
  const w = W / SS, h = H / SS;
  const out = new Uint8Array(w * h * 4);
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
    let r = 0, g = 0, b = 0, a = 0;
    for (let dy = 0; dy < SS; dy++) for (let dx = 0; dx < SS; dx++) {
      const i = ((y * SS + dy) * W + x * SS + dx) * 4, al = buf[i + 3];
      r += buf[i] * al; g += buf[i + 1] * al; b += buf[i + 2] * al; a += al;
    }
    const o = (y * w + x) * 4, n = SS * SS;
    if (a > 0) { out[o] = Math.round(r / a); out[o + 1] = Math.round(g / a); out[o + 2] = Math.round(b / a); out[o + 3] = Math.round(a / n); }
  }
  return { w, h, data: out };
}

// ---------- PNG (stdlib zlib), атлас ----------
const crcT = (() => { const t = new Uint32Array(256); for (let n = 0; n < 256; n++) { let c = n; for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1; t[n] = c >>> 0; } return t; })();
const crc = (b) => { let c = 0xffffffff; for (const v of b) c = crcT[(c ^ v) & 255] ^ (c >>> 8); return (c ^ 0xffffffff) >>> 0; };
function pngEncode(w, h, rgba) {
  const raw = Buffer.alloc((w * 4 + 1) * h);
  for (let y = 0; y < h; y++) { raw[y * (w * 4 + 1)] = 1; /* Sub */ for (let i = 0; i < w * 4; i++) { const v = rgba[y * w * 4 + i], p = i >= 4 ? rgba[y * w * 4 + i - 4] : 0; raw[y * (w * 4 + 1) + 1 + i] = (v - p) & 255; } }
  const chunk = (t, d) => { const b = Buffer.alloc(12 + d.length); b.writeUInt32BE(d.length, 0); b.write(t, 4, 'latin1'); d.copy(b, 8); b.writeUInt32BE(crc(b.subarray(4, 8 + d.length)), 8 + d.length); return b; };
  const ihdr = Buffer.alloc(13); ihdr.writeUInt32BE(w, 0); ihdr.writeUInt32BE(h, 4); ihdr[8] = 8; ihdr[9] = 6;
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk('IHDR', ihdr), chunk('IDAT', zlib.deflateSync(raw, { level: 9 })), chunk('IEND', Buffer.alloc(0))]);
}
function pack(frames, maxW) {
  // shelf-упаковка; frames: [{name,w,h,data}]
  const sorted = [...frames].sort((a, b) => b.h - a.h || (a.name < b.name ? -1 : 1));
  let x = 0, y = 0, rowH = 0, usedW = 0;
  const place = {};
  for (const f of sorted) {
    if (x + f.w > maxW) { x = 0; y += rowH + PAD; rowH = 0; }
    place[f.name] = { x, y, w: f.w, h: f.h };
    x += f.w + PAD; rowH = Math.max(rowH, f.h); usedW = Math.max(usedW, x - PAD);
  }
  const pot = (n) => { let p = 16; while (p < n) p *= 2; return p; };
  const AW = pot(usedW), AH = pot(y + rowH);
  const img = new Uint8Array(AW * AH * 4);
  for (const f of sorted) {
    const p = place[f.name];
    for (let r = 0; r < f.h; r++) img.set(f.data.subarray(r * f.w * 4, (r + 1) * f.w * 4), ((p.y + r) * AW + p.x) * 4);
  }
  return { AW, AH, img, place };
}

// ---------- сборка ----------
function renderModel(model, t) {
  const [fw, fh] = model.frame;
  const kind = model.kind;
  const out = []; // {name, anim, dir, n, w, h, data}
  if (kind === 'building') {
    const sc = renderScene(model, {}, 0, fw, fh);
    const e = 0.5;
    if (sc.bbox[0] < -e || sc.bbox[1] < -e || sc.bbox[2] > fw + e || sc.bbox[3] > fh + e) console.warn(`WARN ${model.id}: геометрия вылезает за кадр`, sc.bbox.map((v) => v.toFixed(1)));
    const ds = downsample(finish(sc, kind, null), sc.W, sc.H);
    out.push({ name: `${model.id}_0`, ...ds });
    return out;
  }
  for (const [an, anim] of Object.entries(model.anims)) {
    for (let d = 0; d < DIRS; d++) {
      for (let n = 0; n < anim.frames.length; n++) {
        const yaw = 45 + (360 / DIRS) * d; // dir 0 = лицом к камере (S), далее по часовой стрелке на экране
        const sc = renderScene(model, framePose(model, anim, n), yaw, fw, fh, anim.frames[n]._fall || 0);
        if (sc.bbox[0] < -0.5 || sc.bbox[1] < -0.5 || sc.bbox[2] > fw + 0.5 || sc.bbox[3] > fh + 0.5) console.warn(`WARN ${model.id} ${an} d${d} f${n}: вылез за кадр`);
        const ds = downsample(finish(sc, kind, [model.anchor[0], model.anchor[1], 0.3 * TILE_HALF_W * Math.SQRT2 / 1.4, 0.3 * TILE_HALF_H * Math.SQRT2 / 1.4]), sc.W, sc.H);
        out.push({ name: `${model.id}_${an}_d${d}_${n}`, ...ds });
      }
    }
  }
  return out;
}

const pickModels = models.filter((m) => ONLY.size === 0 || ONLY.has(m.id));
if (ONLY.size && pickModels.length !== ONLY.size) {
  console.error('нет моделей: ' + [...ONLY].filter((i) => !pickModels.some((m) => m.id === i)).join(', '));
  process.exit(1);
}
for (const m of pickModels) {
  const t = process.hrtime.bigint();
  const frames = renderModel(m);
  const dir = path.join(OUT, m.id);
  fs.mkdirSync(path.join(dir, 'frames'), { recursive: true });
  for (const f of frames) fs.writeFileSync(path.join(dir, 'frames', `${f.name}.png`), pngEncode(f.w, f.h, f.data));
  const anims = m.anims ? Object.fromEntries(Object.entries(m.anims).map(([k, a]) => [k, { fps: a.fps, frames: a.frames.length, loop: a.loop !== false }])) : {};
  const meta = { id: m.id, kind: m.kind, renderer: 'procedural', dirs: m.kind === 'unit' ? DIRS : 1, anchor: m.anchor, frame: m.frame, anims, frames: frames.map((f) => f.name) };
  fs.writeFileSync(path.join(dir, 'meta.json'), JSON.stringify(meta, null, 1) + '\n');
  console.log(`${m.id}: ${frames.length} frames, ${(Number(process.hrtime.bigint() - t) / 1e9).toFixed(1)} s`);
}

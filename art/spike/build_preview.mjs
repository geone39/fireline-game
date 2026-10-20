#!/usr/bin/env node
// Собирает art/spike/preview.html (без зависимостей, открывается двойным кликом по file://).
// Манифесты вшиваются в страницу; картинки подключаются относительными путями из */out/.
//   node art/spike/build_preview.mjs
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const read = (p) => JSON.parse(fs.readFileSync(path.join(here, p), 'utf8'));
const data = { procedural: read('procedural/out/manifest.json'), blender: read('blender/out/manifest.json') };
const pal = read('palette.json');
// атласы вшиваются как data: URI — страница самодостаточна (работает и из file://, и из превью-панелей)
const b64 = {};
for (const p of ['procedural', 'blender']) {
  for (const a of Object.values(data[p].atlases)) b64[a.image] = 'data:image/png;base64,' + fs.readFileSync(path.join(here, '..', a.image)).toString('base64');
}
const timings = { procedural: read('procedural/out/timings.json'), blender: read('blender/out/timings.json') };

const page = String.raw`<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><title>ART-1 spike: procedural vs Blender</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
:root{--bg:#2a2a22;--fg:#eadfb4;--ac:#ea7424;--pn:#3a3a2e}
body{margin:0;background:var(--bg);color:var(--fg);font:14px/1.4 system-ui,sans-serif;padding:16px}
h1{font-size:20px;margin:0 0 4px}h2{font-size:15px;margin:20px 0 6px;color:var(--ac)}
.cols{display:grid;grid-template-columns:repeat(auto-fit,minmax(420px,1fr));gap:16px}
.pn{background:var(--pn);border-radius:6px;padding:10px;overflow-x:auto}
canvas{background:#7a9a5a;image-rendering:pixelated;display:block;margin-top:4px}
button{background:#555546;color:var(--fg);border:1px solid #777;border-radius:4px;padding:3px 8px;margin:2px;cursor:pointer}
button.on{background:var(--ac);color:#111}
.sw{display:inline-block;width:70px;height:30px;margin:2px;font-size:10px;color:#000;text-align:center;line-height:30px;border:1px solid #0008}
small{opacity:.75}
</style></head><body>
<h1>ART-1: стиль A01-lite — процедурный 2D vs Blender</h1>
<small>Один исходник моделей (models.json), одна палитра (palette.json), одна проекция 2:1 (тайл 64×32). «1×» = реальный размер в игре.</small>
<h2>Палитра P01 + P03</h2><div id="pal"></div>
<h2>Мини-база (1×): штаб, две шахты, четыре стрелка (idle / ход / атака / смерть)</h2><div class="cols" id="scenes"></div>
<h2>Юнит: анимация и направления</h2>
<div>анимация: <span id="anims"></span> зум: <span id="zoom"></span></div>
<div class="cols" id="units"></div>
<h2>Читаемость на 48 px (все направления, idle: реальный размер и ×3)</h2><div class="cols" id="read"></div>
<h2>Здания (1×) с контуром футпринта</h2><div class="cols" id="blds"></div>
<h2>Время рендера</h2><pre id="tm"></pre>
<script>
const IMGSRC=__IMG__,DATA=__DATA__,PAL=__PAL__,TM=__TM__;
const IMG={},tiles=[];let zoom=2,anim='walk',tick=0;
const $=id=>document.getElementById(id);
for(const[k,v]of Object.entries(PAL)){if(k[0]==='_'||typeof v!=='string')continue;const d=document.createElement('span');d.className='sw';d.style.background=v;d.textContent=k;$('pal').append(d)}
const load=p=>Promise.all(Object.entries(DATA[p].atlases).map(([k,a])=>new Promise(r=>{const i=new Image();i.onload=()=>{IMG[k]=i;r()};i.onerror=()=>r();i.src=IMGSRC[a.image]})));
const spr=(p,id)=>DATA[p].sprites.find(s=>s.id===id);
function draw(c,p,id,name,x,y,z){const s=spr(p,id),f=s.frames.find(f=>f.name===name),im=IMG[s.atlas];if(!f||!im)return;const a=s.anchors.ground;c.imageSmoothingEnabled=false;c.drawImage(im,f.x,f.y,f.w,f.h,Math.round(x-a.x*z),Math.round(y-a.y*z),f.w*z,f.h*z)}
function diamond(c,x,y,w,h){c.beginPath();c.moveTo(x,y-h*16);c.lineTo(x+w*32,y);c.lineTo(x,y+h*16);c.lineTo(x-w*32,y);c.closePath()}
const nm=(an,d,n)=>'u_rifleman_'+an+'_d'+d+'_'+n;
function mk(parent,title,w,h){const d=document.createElement('div');d.className='pn';d.innerHTML='<b>'+title+'</b>';const c=document.createElement('canvas');c.width=w;c.height=h;d.append(c);parent.append(d);return c.getContext('2d')}
const clear=(c,col='#7a9a5a')=>{c.fillStyle=col;c.fillRect(0,0,c.canvas.width,c.canvas.height)};
async function main(){
 await Promise.all([load('procedural'),load('blender')]);
 const an=DATA.procedural._anims.u_rifleman;
 const mark=()=>{document.querySelectorAll('#anims button').forEach(b=>b.classList.toggle('on',b.dataset.a===anim));document.querySelectorAll('#zoom button').forEach(b=>b.classList.toggle('on',+b.dataset.z===zoom))};
 for(const a of an.order){const b=document.createElement('button');b.textContent=a;b.dataset.a=a;b.onclick=()=>{anim=a;mark()};$('anims').append(b)}
 for(const z of[1,2,3,4]){const b=document.createElement('button');b.textContent=z+'×';b.dataset.z=z;b.onclick=()=>{zoom=z;mark()};$('zoom').append(b)}
 mark();
 for(const p of['procedural','blender']){
  const dirs=DATA[p]._anims.u_rifleman.dirs;
  const sc=mk($('scenes'),p+' ('+dirs+' напр.)',660,360);
  tiles.push(()=>{clear(sc,'#6f9150');const O=[330,120],iso=(x,y)=>[O[0]+(x-y)*32,O[1]+(x+y)*16];
   sc.strokeStyle='#0002';for(let g=-6;g<=6;g++){for(const[a,b]of[[iso(g,-6),iso(g,6)],[iso(-6,g),iso(6,g)]]){sc.beginPath();sc.moveTo(...a);sc.lineTo(...b);sc.stroke()}}
   const bl=[['b_hq',-0.5,-0.5],['b_cash_mine',4.5,-2.5],['b_cash_mine',-3.5,3]].sort((a,b)=>(a[1]+a[2])-(b[1]+b[2]));
   for(const[id,gx,gy]of bl){const q=iso(gx,gy);draw(sc,p,id,id+'_0',q[0],q[1],1)}
   ['idle','walk','attack','die'].forEach((a,i)=>{const g=[[1,4.5],[3,5],[5,5.5],[7,5.5]][i],q=iso(g[0]-1,g[1]);draw(sc,p,'u_rifleman',nm(a,(i*2)%dirs,(tick>>2)%an[a].frames),q[0],q[1],1)})});
  const u=mk($('units'),p+': '+dirs+' направлений',8*(48*4+6)+10,48*4+20);
  tiles.push(()=>{clear(u);const z=zoom,n=an[anim].frames;for(let d=0;d<dirs;d++){const cx=10+d*(48*z+6)+24*z;draw(u,p,'u_rifleman',nm(anim,d,(tick>>2)%n),cx,10+43*z,z)}});
  const r=mk($('read'),p,dirs*(48*3+4)+10,48*3+10+60);
  tiles.push(()=>{clear(r);for(let d=0;d<dirs;d++){draw(r,p,'u_rifleman',nm('idle',d,0),10+d*(48*3+4)+72,10+43*3,3);draw(r,p,'u_rifleman',nm('idle',d,0),12+d*50+24,48*3+10+50,1)}});
  const b=mk($('blds'),p,520,260);
  tiles.push(()=>{clear(b);draw(b,p,'b_hq','b_hq_0',130,140,1);draw(b,p,'b_cash_mine','b_cash_mine_0',390,120,1);b.strokeStyle='#fff8';diamond(b,130,140,4,4);b.stroke();diamond(b,390,120,3,3);b.stroke()});
 }
 $('tm').textContent=['procedural','blender'].map(p=>p+': всего '+(+TM[p].total_s).toFixed(2)+' с; '+Object.entries(TM[p].per_asset_s).map(([k,v])=>k+' = '+(+v).toFixed(2)+' с').join(', ')).join('\n');
 setInterval(()=>{tick++;tiles.forEach(t=>t())},1000/24);tiles.forEach(t=>t());
}
main();
</script></body></html>`;

fs.writeFileSync(
  path.join(here, 'preview.html'),
  page.replace('__IMG__', () => JSON.stringify(b64)).replace('__DATA__', () => JSON.stringify(data)).replace('__PAL__', () => JSON.stringify(pal)).replace('__TM__', () => JSON.stringify(timings)),
);
console.log('preview.html written');

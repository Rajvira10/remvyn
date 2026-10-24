// Original geometric artwork. SVG paths are authored here, never traced.
// Renderer: npm install --prefix branding/.tools --cache branding/.npm-cache --no-audit --no-fund @resvg/resvg-js
const fs = require('node:fs');
const path = require('node:path');
const { Resvg } = require('./.tools/node_modules/@resvg/resvg-js');
const dir = __dirname;
const blue = '#7AA2F7';
const channel = 'M768 192H416C292.288 192 192 292.288 192 416V448C192 536.366 263.634 608 352 608H448V480H352C334.327 480 320 465.673 320 448V416C320 362.981 362.981 320 416 320H768Z';
const symbol = `<g fill="${blue}"><path d="${channel}"/><path d="${channel}" transform="rotate(180 512 512)"/></g>`;
const svg = (title, body, box='0 0 1024 1024', size='width="1024" height="1024"') => `<?xml version="1.0" encoding="UTF-8"?>\n<svg xmlns="http://www.w3.org/2000/svg" ${size} viewBox="${box}" role="img" aria-labelledby="title"><title id="title">${title}</title>${body}</svg>\n`;
function write(name, content) { fs.writeFileSync(path.join(dir,name),content); }
write('mark.svg', svg('Remvyn — paired connection channels',symbol));
const tile = '<path d="M256 48H768C903 48 976 121 976 256V768C976 903 903 976 768 976H256C121 976 48 903 48 768V256C48 121 121 48 256 48Z" fill="#15161A"/>';
const edge = '<path d="M256 49H768C902.4 49 975 121.6 975 256V768C975 902.4 902.4 975 768 975H256C121.6 975 49 902.4 49 768V256C49 121.6 121.6 49 256 49Z" fill="none" stroke="#30343F" stroke-width="2"/>';
write('app-icon.svg',svg('Remvyn desktop app icon', tile+edge+`<g transform="translate(51.2 51.2) scale(.9)">${symbol}</g>`));
// The complete symbol fits inside the centered 66dp-diameter safe circle.
// Artwork bounds: x/y 29..79, furthest painted point < 33dp from (54,54).
write('android-foreground.svg',svg('Remvyn Android adaptive foreground',`<g transform="translate(14 14) scale(.078125)">${symbol}</g>`,'0 0 108 108','width="108" height="108"'));
write('android-background.txt','#15161A\n');
// Velqin: two communicating rails form a V, with a separate live endpoint.
const alt = `<g fill="${blue}"><path d="M160 224H304L512 672L720 224H864L578 832H446Z"/><path d="M448 224H576V352H448Z"/></g>`;
write('alt-mark.svg',svg('Velqin — converging connection rails and live endpoint',alt));
// Custom lowercase lettering: hand-authored closed outlines; no font dependency.
const letters = {
 r:'M0 3H20V25C27 8 40 0 57 0H65V21H56C34 21 21 39 21 65V120H0Z',
 m:'M0 3H20V19C28 6 40 0 54 0C72 0 84 8 91 22C100 7 113 0 130 0C157 0 172 17 172 47V120H151V50C151 29 143 19 127 19C110 19 99 33 99 55V120H78V50C78 29 70 19 54 19C34 19 21 33 21 55V120H0Z',
 v:'M-2 3H21L53 96L85 3H108L64 120H42Z',
 y:'M-2 3H21L54 96L86 3H109L62 137C54 160 43 170 23 170H8V151H20C31 151 38 145 43 132L47 120Z',
 s:'M83 17C65 4 45 0 29 4C10 8 0 20 0 36C0 55 14 65 38 71L49 74C61 77 66 82 66 89C66 98 57 104 44 104C30 104 17 99 5 91L-4 108C10 118 27 123 44 123C69 123 86 110 86 89C86 69 72 59 48 53L36 50C23 47 19 43 19 36C19 28 27 22 39 22C51 22 64 26 74 33Z',
 e:'M100 68H21C23 91 36 104 57 104C72 104 84 98 95 90L103 107C90 118 74 123 56 123C22 123 0 99 0 62C0 25 21 0 53 0C84 0 101 21 101 56ZM21 50H81C79 29 69 18 53 18C36 18 25 29 21 50Z',
 l:'M0-43H21V120H0Z',
 q:'M104 3V163H83V104C75 116 63 123 47 123C18 123 0 99 0 62C0 25 19 0 48 0C64 0 76 7 84 19V3ZM83 62C83 35 72 19 53 19C33 19 21 35 21 62C21 89 33 104 53 104C72 104 83 89 83 62Z',
 n:'M0 3H20V20C28 7 41 0 57 0C84 0 99 17 99 47V120H78V50C78 29 69 19 52 19C33 19 21 33 21 55V120H0Z'
};
let x=0;const advances={r:96,s:106,e:124,l:53,q:130,n:122,m:195,v:126,y:131};let lettering='';
for(const c of 'remvyn'){lettering+=`<path d="${letters[c]}" transform="translate(${x} 0)"/>`;x+=advances[c];}
write('wordmark.svg',svg('Remvyn',`<g transform="translate(-4 26) scale(.2421875)">${symbol}</g><g fill="#EFF2FA" fill-rule="evenodd" transform="translate(270 88)">${lettering}</g>`,'0 0 1100 300','width="1100" height="300"'));
for(const [source,target,width] of [
 ['app-icon.svg','app-icon-1024.png',1024],['mark.svg','favicon-32.png',32],
 ['mark.svg','mark-16.png',16],['mark.svg','mark-24.png',24],['alt-mark.svg','alt-mark-32.png',32]
]){
 const r=new Resvg(fs.readFileSync(path.join(dir,source),'utf8'),{fitTo:{mode:'width',value:width}});
 write(target,r.render().asPng());
}
// Raster QA board shows actual PNGs at 1x plus nearest-neighbor enlargements.
const data=n=>'data:image/png;base64,'+fs.readFileSync(path.join(dir,n)).toString('base64');
const sample=(n,w)=>`<div class="sample"><img width="${w}" height="${w}" src="${data(n)}"><span>${w}px · actual</span><img class="zoom" width="128" height="128" src="${data(n)}"><span>pixel enlargement</span></div>`;
const html=`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Remvyn identity</title><style>
*{box-sizing:border-box}body{margin:0;color:var(--foreground,#eff2fa);font:15px/1.5 system-ui,sans-serif}h1,h2,p{margin:0}h1{font-size:36px;letter-spacing:-1.3px}h2{font-size:16px;font-weight:600}.intro{margin-bottom:24px}.intro p{color:var(--muted-foreground,#969ba8);max-width:600px;margin-top:8px}.hero{background:#111216;border-radius:16px;padding:32px;display:flex;align-items:center;gap:28px;color:#eff2fa}.hero>img{width:142px;height:142px}.lockup{width:min(100%,410px);height:auto}.hero p{color:#969ba8;font-size:13px;margin-top:12px}.section{margin-top:28px}.small{display:flex;gap:36px;align-items:start;flex-wrap:wrap;background:#15161a;padding:24px;border-radius:16px;margin-top:12px}.sample{display:flex;flex-direction:column;align-items:center;gap:10px;color:#969ba8;font-size:11px;min-width:128px}.sample>img:first-child{margin:0 auto;object-fit:contain}.zoom{image-rendering:pixelated}.bottom{display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-top:12px}.panel{background:#15161a;border-radius:16px;padding:24px;color:#eff2fa}.panel p{font-size:12px;color:#969ba8;margin-top:12px}.android{display:flex;gap:20px;align-items:center;margin:12px 0}.android svg{width:84px;height:84px;background:#15161a;border:1px solid #30343f}.circle{border-radius:50%}.squircle{border-radius:25%}.alt{display:flex;align-items:center;gap:18px}.alt svg{width:80px;height:80px}.palette{display:flex;gap:18px;flex-wrap:wrap;margin-top:12px}.swatch{font:12px ui-monospace,monospace}.swatch i{display:block;width:76px;height:36px;border-radius:6px;margin-bottom:6px;border:1px solid #30343f}.foot{color:var(--muted-foreground,#969ba8);font-size:12px;margin:24px 0 8px}@media(max-width:550px){.hero{padding:24px;gap:18px;flex-direction:column;align-items:flex-start}.hero>img{width:100px;height:100px}.bottom{grid-template-columns:1fr}.small{gap:20px}.intro h1{font-size:30px}}
</style></head><body><div class="intro"><h1>Remvyn</h1><p>REM-vin · SSH access and a clear view of your servers. Two opposing channels: one connection, traffic in both directions.</p></div><div class="hero"><img src="${data('app-icon-1024.png')}" alt="Remvyn app icon"><div>${fs.readFileSync(path.join(dir,'wordmark.svg'),'utf8').replace(/<\?xml[^>]+>/,'').replace('<svg ','<svg class="lockup" ')}<p>Original vector geometry · #7AA2F7 on #15161A</p></div></div><div class="section"><h2>Small-size inspection</h2><div class="small">${sample('mark-16.png',16)}${sample('mark-24.png',24)}${sample('favicon-32.png',32)}</div></div><div class="section"><h2>Android & alternative</h2><div class="bottom"><div class="panel"><h2>Adaptive icon</h2><div class="android">${['circle','squircle'].map(c=>fs.readFileSync(path.join(dir,'android-foreground.svg'),'utf8').replace(/<\?xml[^>]+>/,'').replace('<svg ',`<svg class="${c}" `)).join('')}</div><p>Foreground fits the centered 66dp safe circle.<br>Background: #15161A</p></div><div class="panel"><div class="alt">${fs.readFileSync(path.join(dir,'alt-mark.svg'),'utf8').replace(/<\?xml[^>]+>/,'')}<div><h2>Velqin</h2><p>VEL-kin · runner-up<br>Converging rails, live endpoint.</p></div></div><p>Exact name also appears in retail and supplements. No relevant SSH / infrastructure product found.</p></div></div></div><div class="section"><h2>Palette</h2><div class="palette">${[['#7AA2F7','Connection blue'],['#EFF2FA','Wordmark'],['#111216','Canvas'],['#15161A','Icon surface']].map(([c,n])=>`<div class="swatch"><i style="background:${c}"></i>${c}<br>${n}</div>`).join('')}</div></div><p class="foot">No relevant software collision found for Remvyn. GitHub search: 0 repositories; exact handle and npm: 404. All four domain checks return not-found. TMview contains search: no rows. Full trademark clearance and registrar checkout eligibility remain unverified. See BRANDING.md for evidence.</p></body></html>`;
write('identity-preview.html',html);
console.log('Generated vector masters, five PNGs, Android color, and self-contained preview.');

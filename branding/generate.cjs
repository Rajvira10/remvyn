// Remvyn round 2: original closed vector geometry, hand-authored, never traced.
// npm install --prefix branding/.tools --cache branding/.npm-cache --no-audit --no-fund @resvg/resvg-js@2.6.2
const fs = require('node:fs');
const path = require('node:path');
const { Resvg } = require('./.tools/node_modules/@resvg/resvg-js');
const dir = __dirname;
const blue = '#7AA2F7';
const svg = (title,body,box='0 0 1024 1024',size='width="1024" height="1024"') => `<?xml version="1.0" encoding="UTF-8"?>\n<svg xmlns="http://www.w3.org/2000/svg" ${size} viewBox="${box}" role="img" aria-labelledby="title"><title id="title">${title}</title>${body}</svg>\n`;
const write=(name,content)=>{fs.mkdirSync(path.dirname(path.join(dir,name)),{recursive:true});fs.writeFileSync(path.join(dir,name),content);};
const read=name=>fs.readFileSync(path.join(dir,name),'utf8');
const png=(source,width,background)=>new Resvg(source,{font:{loadSystemFonts:false,fontFiles:['/System/Library/Fonts/Supplemental/Arial.ttf'],defaultFontFamily:'Arial'},fitTo:{mode:'width',value:width},...(background?{background}:{})}).render().asPng();
const concepts=[
 {id:'01-routed-r',name:'Routed R',
  body:`<path fill="${blue}" fill-rule="evenodd" d="M192 192H544C703.058 192 832 277.961 832 384C832 473.4 751 535.6 648 561L736 704H832V832H656L496 576H320V832H192ZM320 320V448H544C632.366 448 704 423.346 704 384C704 344.654 632.366 320 544 320Z"/>`},
 {id:'02-linked-ports',name:'Linked ports',
  body:`<path fill="${blue}" fill-rule="evenodd" d="M320 304C214 304 128 390 128 496V528C128 634 214 720 320 720C409 720 484 659 506 576H518C540 659 615 720 704 720C810 720 896 634 896 528V496C896 390 810 304 704 304C615 304 540 365 518 448H506C484 365 409 304 320 304ZM256 496C256 461 285 432 320 432C355 432 384 461 384 496V528C384 563 355 592 320 592C285 592 256 563 256 528ZM640 496C640 461 669 432 704 432C739 432 768 461 768 496V528C768 563 739 592 704 592C669 592 640 563 640 528Z"/>`},
 {id:'03-cursor-signal',name:'Cursor + signal',
  body:`<g fill="${blue}"><path d="M128 464H256V272H480V656H512V464H608V752H384V368H352V560H128Z"/><path d="M704 320H832V704H704Z"/></g>`},
 {id:'04-split-aperture',name:'Split aperture',
  body:`<g fill="${blue}"><path d="M224 272L512 128L800 272V416L512 272L224 416Z"/><path d="M224 480L448 368V816L224 704Z"/><path d="M576 368L800 480V704L576 816Z"/></g>`}
];
for(const c of concepts){
 const source=svg(`Remvyn concept ${c.name}`,c.body);
 write(`concepts/concept-${c.id}.svg`,source);
 for(const size of [16,24,32,256]){
  write(`concepts/concept-${c.id}-${size}.png`,png(source,size));
  write(`concepts/review/concept-${c.id}-${size}-dark.png`,png(source,size,'#111216'));
 }
}
// A contact sheet is itself vector-assembled, with the exact PNG exports embedded.
// Text is permitted only on review sheets; icon artwork has no text or images.
const data=name=>'data:image/png;base64,'+fs.readFileSync(path.join(dir,name)).toString('base64');
const readings=['R / square-ended route','Spectacles / infinity','Stepped waveform + cursor','Box / negative-space T'];
let sheet='<rect width="1200" height="1320" fill="#111216"/><g fill="#EFF2FA" font-family="Arial, sans-serif"><text x="40" y="48" font-size="26" font-weight="bold">Remvyn / symbol exploration / round 2</text><text x="40" y="78" font-size="15" fill="#A6ADBB">Hand-authored vectors · actual sizes and 8× 16px enlargement · #7AA2F7 on #111216</text>';
for(let i=0;i<concepts.length;i++){
 const c=concepts[i],y=104+i*300;
 sheet+=`<rect x="24" y="${y}" width="1152" height="284" rx="12" fill="#111216" stroke="#30343F"/><text x="48" y="${y+32}" font-size="20">${c.name}</text><text x="48" y="${y+68}" font-size="14" fill="#A6ADBB">${readings[i]}</text><text x="48" y="${y+94}" font-size="14" fill="#7AA2F7">${i===0?'SELECTED':'REJECTED'}</text>`;
 for(const [size,x] of [[256,330],[16,660],[24,742],[32,834]]){
  sheet+=`<image href="${data(`concepts/concept-${c.id}-${size}.png`)}" x="${x}" y="${y+52}" width="${size}" height="${size}"/><text x="${x}" y="${y+230}" font-size="14" fill="#A6ADBB">${size}px</text>`;
 }
 sheet+=`<image href="${data(`concepts/concept-${c.id}-16.png`)}" x="960" y="${y+58}" width="128" height="128" image-rendering="optimizeSpeed"/><text x="960" y="${y+230}" font-size="14" fill="#A6ADBB">16px × 8</text>`;
}
sheet+='</g>';
write('concepts/contact-sheet.svg',svg('Remvyn round 2 concept contact sheet',sheet,'0 0 1200 1320','width="1200" height="1320"'));
write('concepts/contact-sheet.png',png(read('concepts/contact-sheet.svg'),1200));
console.log('Generated four concept SVGs, 16 transparent PNGs, 16 dark review PNGs, and contact sheet.');

// Selected master: explicit R with a square-ended routed leg.
const symbol=concepts[0].body;
write('mark.svg',svg('Remvyn — routed R',symbol));
const tile='<path d="M256 48H768C903 48 976 121 976 256V768C976 903 903 976 768 976H256C121 976 48 903 48 768V256C48 121 121 48 256 48Z" fill="#15161A"/>';
const edge='<path d="M256 49H768C902.4 49 975 121.6 975 256V768C975 902.4 902.4 975 768 975H256C121.6 975 49 902.4 49 768V256C49 121.6 121.6 49 256 49Z" fill="none" stroke="#30343F" stroke-width="2"/>';
write('app-icon.svg',svg('Remvyn desktop app icon',tile+edge+`<g transform="translate(51.2 51.2) scale(.9)">${symbol}</g>`));
// Bounds: 30.96..77.04dp; farthest point (77.04,77.04) is radius 32.58348dp.
// The 66dp-diameter adaptive safe circle is centered at (54,54).
write('android-foreground.svg',svg('Remvyn Android adaptive foreground',`<g transform="translate(17.136 17.136) scale(.072)">${symbol}</g>`,'0 0 108 108','width="108" height="108"'));
// The approved letter outlines AND their internal spacing/placement are copied exactly.
const approvedLettering=read('archive/round1/wordmark.svg').match(/<g fill="#EFF2FA"[\s\S]*?<\/g>/)[0];
write('wordmark.svg',svg('Remvyn',`<g transform="translate(-4 26) scale(.2421875)">${symbol}</g>${approvedLettering}`,'0 0 1100 300','width="1100" height="300"'));
const rasterExports=[['app-icon.svg','app-icon-1024.png',1024],['mark.svg','favicon-32.png',32],['mark.svg','mark-16.png',16],['mark.svg','mark-24.png',24]];
for(const [source,target,width] of rasterExports)write(target,png(read(source),width));
for(const size of [16,24,32,256])write(`review/mark-${size}-dark.png`,png(read('mark.svg'),size,'#111216'));
write('review/app-icon-256-dark.png',png(read('app-icon.svg'),256,'#111216'));
const inline=n=>read(n).replace(/<\?xml[^>]+>/,'');
const decisions=[
 ['Routed R','R at every size; selected.'],
 ['Linked ports','Spectacles / infinity; too generic.'],
 ['Cursor + signal','Stepped waveform + bar; noisy at 16px.'],
 ['Split aperture','Box / T-shaped gap; wrong initial.']
];
const samples=[16,24,32].map(size=>`<div class="sample"><div class="actual"><img src="${data(size===32?'favicon-32.png':`mark-${size}.png`)}" width="${size}" height="${size}" alt="${size}px R"></div><span>${size}px actual</span><img class="zoom" src="${data(size===32?'favicon-32.png':`mark-${size}.png`)}" width="96" height="96" alt="Pixel enlargement"><span>pixel enlargement</span></div>`).join('');
const html=`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Remvyn / routed R / round 2</title><style>
*{box-sizing:border-box}body{margin:0;font:14px/1.5 system-ui,sans-serif;color:var(--foreground,#eff2fa)}p,h1,h2{margin:0}h2{font-size:16px;font-weight:600}p{color:var(--muted-foreground,#a6adbb)}.hero{display:flex;align-items:center;gap:26px;padding:28px;background:#111216;border-radius:16px;color:#eff2fa}.hero>img{width:128px;height:128px}.name{flex:1;min-width:0}.name svg{width:100%;height:auto;display:block}.eyebrow{font-size:11px;letter-spacing:.14em;color:#7aa2f7}.hero p{font-size:12px;color:#a6adbb;margin:8px 0 0}.section{margin-top:24px}.section>p{margin:6px 0 12px}.sizes{display:flex;justify-content:space-around;gap:20px;padding:20px;background:#111216;border-radius:12px}.sample{display:flex;align-items:center;flex-direction:column;gap:8px;color:#a6adbb;font-size:11px}.actual{height:34px;display:flex;align-items:center;justify-content:center}.zoom{image-rendering:pixelated}.adaptive{display:flex;align-items:center;gap:24px;padding:20px;background:#111216;border-radius:12px;color:#a6adbb;font-size:12px}.masks{display:flex;gap:16px}.mask{width:86px;height:86px;background:#15161a;overflow:hidden;flex:none}.mask svg{width:100%;height:100%;display:block}.circle{border-radius:50%}.round{border-radius:25%}.square{border-radius:0}.concepts{display:grid;grid-template-columns:repeat(4,1fr);gap:12px}.concept{border:1px solid var(--border,#30343f);border-radius:12px;padding:14px;font-size:11px}.concept img{display:block;width:88px;height:88px;background:#111216;border-radius:8px;margin-bottom:10px}.concept strong{font-size:12px;display:block}.concept p{margin-top:4px}.note{font-size:11px;margin-top:14px}@media(max-width:520px){.hero{padding:20px;gap:12px}.hero>img{width:76px;height:76px}.sizes{gap:8px;padding:16px}.adaptive{align-items:flex-start;flex-direction:column;gap:14px}.mask{width:72px;height:72px}.concepts{grid-template-columns:repeat(2,1fr)}.concept img{width:64px;height:64px}}
</style></head><body><div class="hero"><img src="${data('app-icon-1024.png')}" alt="Remvyn app icon"><div class="name"><div class="eyebrow">ROUTED R / ROUND 2</div>${inline('wordmark.svg')}<p>Approved lowercase lettering · new R symbol</p></div></div><div class="section"><h2>An R with a routed leg</h2><p>A closed bowl and full-height spine establish the initial. The leg turns into a square connection endpoint. One continuous silhouette; no extra detail to lose.</p></div><div class="sizes">${samples}</div><div class="section"><h2>Android adaptive foreground</h2><p>Shown through circle, rounded-square and square masks.</p><div class="adaptive"><div class="masks">${['circle','round','square'].map(c=>`<div class="mask ${c}">${inline('android-foreground.svg')}</div>`).join('')}</div><div>108dp layer · 66dp safe circle<br>Artwork radius ≤ 32.584dp<br>Background #15161A</div></div></div><div class="section"><h2>Four directions, one decision</h2><p>Each rendered and inspected at 16, 24, 32 and 256px.</p><div class="concepts">${concepts.map((c,i)=>`<div class="concept"><img src="${data(`concepts/concept-${c.id}-256.png`)}" alt="${c.name}"><strong>${decisions[i][0]}</strong><p>${decisions[i][1]}</p></div>`).join('')}</div></div><p class="note">Palette and lettering preserved. Round-one art archived. Visual comparison covers the nine requested brands plus Render, Raycast and Replit; no obvious resemblance found for the selected R. Readings are a designer assessment, not a user study.</p></body></html>`;
write('identity-preview.html',html);
// Re-render the actual SVG files and byte-compare against each delivered PNG.
const crypto=require('node:crypto');
const hash=b=>crypto.createHash('sha256').update(b).digest('hex');
const verified=rasterExports.map(([source,target,width])=>{
 const expected=png(read(source),width),actual=fs.readFileSync(path.join(dir,target));
 if(!expected.equals(actual))throw new Error(`Raster mismatch: ${target}`);
 return {source,target,width,height:width,svgSha256:hash(read(source)),pngSha256:hash(actual),exactRerenderMatch:true};
});
// Independently derive the Android control hull from the master path coordinates.
const tokens=symbol.match(/ d="([^"]+)"/)[1].match(/[MLHVCZ]|[-+]?(?:\d*\.\d+|\d+\.?\d*)/g);
let ti=0,cx=0,cy=0,startX=0,startY=0;const hull=[];
while(ti<tokens.length){
 const cmd=tokens[ti++];
 const number=()=>Number(tokens[ti++]);
 if(cmd==='M'||cmd==='L'){cx=number();cy=number();hull.push([cx,cy]);if(cmd==='M'){startX=cx;startY=cy;}}
 else if(cmd==='H'){cx=number();hull.push([cx,cy]);}
 else if(cmd==='V'){cy=number();hull.push([cx,cy]);}
 else if(cmd==='C'){for(let k=0;k<3;k++){cx=number();cy=number();hull.push([cx,cy]);}}
 else if(cmd==='Z'){cx=startX;cy=startY;}
 else throw new Error(`Unsupported master path command ${cmd}`);
}
const fgTransform=read('android-foreground.svg').match(/translate\(([\d.]+) ([\d.]+)\) scale\(([\d.]+)\)/);
const [fgX,fgY,fgScale]=fgTransform.slice(1).map(Number);
const fgHull=hull.map(([x,y])=>[fgX+x*fgScale,fgY+y*fgScale]);
const hullRadius=Math.max(...fgHull.map(([x,y])=>Math.hypot(x-54,y-54)));
const hullBounds=[Math.min(...fgHull.map(p=>p[0])),Math.min(...fgHull.map(p=>p[1])),Math.max(...fgHull.map(p=>p[0])),Math.max(...fgHull.map(p=>p[1]))];
if(hullRadius>33)throw new Error('Android foreground exceeds adaptive safe circle');
const currentLettering=read('wordmark.svg').match(/<g fill="#EFF2FA"[\s\S]*?<\/g>/)[0];
if(currentLettering!==approvedLettering)throw new Error('Approved lettering changed');
write('verification.json',JSON.stringify({renderer:'@resvg/resvg-js 2.6.2',exports:verified,approvedLettering:{byteIdentical:true,sha256:hash(currentLettering)},android:{layer:108,safeCircleCenter:[54,54],safeCircleRadius:33,artworkBounds:hullBounds,conservativeHullRadius:hullRadius,hullPointCount:fgHull.length,insideSafeCircle:hullRadius<33},scope:'Design artifacts only; visual reading is a designer assessment.'},null,2)+'\n');
console.log('Generated final SVG/PNG masters, dark review samples, responsive preview, and verification.json; PNG rerenders match and approved lettering is byte-identical.');

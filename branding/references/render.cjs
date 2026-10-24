// Reference sheet only: third-party logos retain their original geometry/colors.
const fs=require('node:fs'),path=require('node:path');
const {Resvg}=require('../.tools/node_modules/@resvg/resvg-js');
const dir=__dirname;const rows=JSON.parse(fs.readFileSync(path.join(dir,'sources.json')));
const height=30+Math.ceil(rows.filter(r=>r.file).length/3)*240;
let body=`<rect width="960" height="${height}" fill="#252833"/>`;
rows.filter(r=>r.file).forEach((r,i)=>{
 const x=20+(i%3)*310,y=20+Math.floor(i/3)*240;
 const f=r.file==='sentry.ico'?'sentry.png':r.file;
 const data=fs.readFileSync(path.join(dir,f));
 body+=`<rect x="${x}" y="${y}" width="290" height="220" rx="10" fill="#FFFFFF"/><image href="data:${f.endsWith('.svg')?'image/svg+xml':'image/png'};base64,${data.toString('base64')}" x="${x+35}" y="${y+20}" width="220" height="140"/><text x="${x+20}" y="${y+198}" fill="#111216" font-size="20" font-family="Arial">${r.name}</text>`;
});
const svg=`<svg xmlns="http://www.w3.org/2000/svg" width="960" height="${height}" viewBox="0 0 960 ${height}">${body}</svg>`;
fs.writeFileSync(path.join(dir,'comparison.png'),new Resvg(svg,{font:{loadSystemFonts:false,fontFiles:['/System/Library/Fonts/Supplemental/Arial.ttf']}}).render().asPng());

const W=680,H=210,P={left:57,right:20,top:16,bottom:32};
const number=x=>Number(x).toLocaleString('zh-CN',{maximumFractionDigits:2});
function range(values){let lo=Math.min(...values),hi=Math.max(...values);if(lo===hi){lo-=1;hi+=1;}const pad=(hi-lo)*.06;return [lo-pad,hi+pad];}
function axes(min,max,xStart,xEnd){let svg='';for(let i=0;i<=4;i++){const y=P.top+(H-P.top-P.bottom)*i/4;svg+=`<line class="grid" x1="${P.left}" x2="${W-P.right}" y1="${y}" y2="${y}"/><text x="${P.left-9}" y="${y+3}" text-anchor="end">${number(max-(max-min)*i/4)}</text>`;}return svg+`<text x="${P.left}" y="${H-9}">${xStart}</text><text x="${W-P.right}" y="${H-9}" text-anchor="end">${xEnd}</text>`;}
function frame(contents,label){return `<svg viewBox="0 0 ${W} ${H}" role="img" aria-label="${label}">${contents}</svg>`;}
export function lineChart(points,label,color){
  if(!points.length)return '';
  const [min,max]=range(points.map(p=>p.value));const start=Date.parse(points[0].date),end=Date.parse(points.at(-1).date);
  const coords=points.map(p=>[P.left+(Date.parse(p.date)-start)/Math.max(1,end-start)*(W-P.left-P.right),P.top+(max-p.value)/(max-min)*(H-P.top-P.bottom)]);
  const line=coords.map(([x,y])=>`${x.toFixed(2)},${y.toFixed(2)}`).join(' ');
  return frame(axes(min,max,points[0].date,points.at(-1).date)+`<polygon points="${coords[0][0]},${H-P.bottom} ${line} ${coords.at(-1)[0]},${H-P.bottom}" fill="${color}" opacity=".06"/><polyline points="${line}" fill="none" stroke="${color}" stroke-width="1.8" stroke-linejoin="round"/>`,label+'原始序列走势图');
}
export function scatterChart(points){
  if(!points.length)return '';
  const [xmin,xmax]=range(points.map(p=>p.x)),[ymin,ymax]=range(points.map(p=>p.y));
  return frame(axes(ymin,ymax,number(xmin),number(xmax))+points.map(p=>`<circle cx="${P.left+(p.x-xmin)/(xmax-xmin)*(W-P.left-P.right)}" cy="${P.top+(ymax-p.y)/(ymax-ymin)*(H-P.top-P.bottom)}" r="2.7" fill="#467f5e" opacity=".36"><title>${p.date}: X=${number(p.x)}, Y=${number(p.y)}</title></circle>`).join(''),'X与Y的散点图');
}
export function histogramChart(hist){
  const max=Math.max(...hist.counts)*1.1||1,bw=(W-P.left-P.right)/hist.counts.length;
  return frame(axes(0,max,number(hist.edges[0]),number(hist.edges.at(-1)))+hist.counts.map((v,i)=>`<rect x="${P.left+i*bw+1}" y="${P.top+(1-v/max)*(H-P.top-P.bottom)}" width="${Math.max(1,bw-2)}" height="${v/max*(H-P.top-P.bottom)}" rx="2" fill="#9daf7d"><title>${number(hist.edges[i])} — ${number(hist.edges[i+1])}: ${v}</title></rect>`).join(''),'结果变量分布直方图');
}

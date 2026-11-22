'use strict';
const glyphs={
 back:'m14 5-7 7 7 7',next:'m9 5 7 7-7 7',down:'m6 9 6 6 6-6',plus:'M12 5v14 M5 12h14',check:'m4 12 5 5L20 6',close:'m5 5 14 14 M19 5 5 19',
 card:'M3 6h18v13H3z M3 10h18 M15 15h3',wallet:'M3 6h15v3H4v11h16V9 M15 12h6v5h-6z',compass:'M12 2a10 10 0 1 0 .01 0 M16 7l-3 7-6 3 3-7z',
 settings:'M4 7h9 M17 7h3 M4 17h3 M11 17h9 M13 4h4v6h-4z M7 14h4v6H7z',
 chip:'M6 6h12v12H6z M9 2v4m6-4v4 M9 18v4m6-4v4 M2 9h4m-4 6h4 M18 9h4m-4 6h4 M10 10h4v4h-4z',
 coffee:'M4 7h12v7a5 5 0 0 1-5 5H9a5 5 0 0 1-5-5z M16 8h2a3 3 0 0 1 0 6h-2 M3 21h16 M7 3v1m4-1v1',pin:'M19 10c0 5-7 11-7 11S5 15 5 10a7 7 0 1 1 14 0z M14 10a2 2 0 1 1-4 0 2 2 0 1 1 4 0',
 clock:'M12 3a9 9 0 1 0 .01 0 M12 7v6l4 2',bell:'M5 16h14l-2-3V9a5 5 0 0 0-10 0v4z M10 20h4',shield:'m12 2 8 3v6c0 5-4 8-8 11-4-3-8-6-8-11V5z M8.5 11.5l2.5 2.5 4.5-5',
 qr:'M3 3h6v6H3z M15 3h6v6h-6z M3 15h6v6H3z M14 14h3v3h4v4h-6v-2 M20 13h1',ble:'M7 6l10 11-5 4V3l5 4L7 18',list:'M8 6h13 M8 12h13 M8 18h13 M3 6h1 M3 12h1 M3 18h1',
 book:'M3 4h7l2 2 2-2h7v15h-7l-2 2-2-2H3z M12 6v15',globe:'M12 2a10 10 0 1 0 .01 0 M2 12h20 M12 2c-5 4-5 16 0 20 M12 2c5 4 5 16 0 20',
 send:'M4 19 19 4 M7 4h12v12',recv:'M19 4 4 19 M16 19H4V7',receipt:'M5 3l2 1 2-1 3 1 3-1 2 1 2-1v18l-3-1-4 1-4-1-3 1z M8 8h8 M8 12h8 M8 16h5',
 user:'M15.5 7.5a3.5 3.5 0 1 1-7 0 3.5 3.5 0 1 1 7 0 M4 21v-2a8 7 0 0 1 16 0v2',bag:'M4 8h16l1 13H3z M8 9V7a4 4 0 1 1 8 0v2',
 lock:'M5 10h14v11H5z M8 10V7a4 4 0 1 1 8 0v3 M12 14v3',search:'M17 10a7 7 0 1 1-14 0 7 7 0 1 1 14 0 M15 15l6 6',
 refresh:'M20 7v5h-5 M4 17v-5h5 M19 11a7 7 0 0 0-12-5 M5 13a7 7 0 0 0 12 5',shop:'m3 9 2-6h14l2 6v3H3z M5 12v9h14v-9 M9 21v-6h6v6',calendar:'M3 5h18v16H3z M7 2v6m10-6v6 M3 10h18',
 chart:'M4 20h16 M6 16v-4m5 4V7m5 9V4',layers:'m12 3 10 5-10 5L2 8z M2 12l10 5 10-5 M2 16l10 5 10-5',download:'M12 3v12m-5-5 5 5 5-5 M4 16v5h16v-5',phone:'M6 2h12v20H6z M10 5h4 M11 19h2',flag:'M5 21V3 M5 4h13l-3 4 3 4H5',key:'M14 10a5 5 0 1 0-2 3l3 3h3v3h3v-4l-7-7',eye:'M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12z M15 12a3 3 0 1 1-6 0 3 3 0 1 1 6 0',info:'M12 2a10 10 0 1 0 .01 0 M12 11v6 M12 7v1'
};
function icon(k){return `<svg class="icon" aria-hidden="true" viewBox="0 0 24 24"><path d="${glyphs[k]}"/></svg>`}
const brand=(name='Wallet')=>`<span class="logo"><span class="brandglyph"><i></i><i></i><i></i></span><span>NU <em>${name}</em></span></span>`;
const badge=(text,cls='',k='')=>`<span class="badge ${cls}">${k?icon(k):''}${text}</span>`;
const btn=(text,cls='',k='')=>`<button type="button" tabindex="-1" class="btn ${cls}">${k?icon(k):''}${text}</button>`;
const pair=(label,value)=>`<div class="pair"><span>${label}</span><b>${value}</b></div>`;
const photo=(n,alt,cls='')=>`<div class="photo p${n} ${cls}" role="img" aria-label="${alt}"><i></i></div>`;
const amount=(num,cls='')=>`<div class="amount ${cls}">${num}<span class="unit">TestUSDC</span></div>`;
const square=k=>`<span class="squareicon">${icon(k)}</span>`;
const contours=()=>`<svg class="contours" viewBox="0 0 180 240" aria-hidden="true">${Array.from({length:15},(_,i)=>`<ellipse cx="135" cy="132" rx="${20+i*6}" ry="${40+i*7}" fill="none" stroke="#dbf2a4" stroke-width="1.2"/>`).join('')}</svg>`;
function sys(time){return `<div class="sys"><span>${time}</span><svg aria-hidden="true" viewBox="0 0 49 14" fill="currentColor"><path d="M1 12V9h3v3zm5 0V7h3v5zm5 0V4h3v8zm5 0V2h3v10z"/><rect x="25" y="3" width="19" height="9" rx="2" fill="none" stroke="currentColor"/><rect x="27" y="5" width="14" height="5" rx="1"/><path d="M46 5h2v4h-2z"/></svg></div>`}
const bottom=(active)=>`<div class="bnav">${[['wallet','지갑'],['card','결제'],['compass','둘러보기'],['settings','설정']].map(([k,t])=>`<div class="navitem ${active===t?'active':''}">${icon(k)}${t}</div>`).join('')}</div>`;
const phone=(body,nav='',time='10:41')=>`<article class="phone">${sys(time)}<div class="phonecontent">${body}</div>${nav?bottom(nav):''}<div class="gesture"></div></article>`;
const mobile=(...screens)=>`<div class="mobileboard">${screens.join('')}</div>`;
const bar=(title,right='')=>`<div class="bar"><div class="titleline">${icon('back')}<b>${title}</b></div>${right}</div>`;
const line=(k,title,sub,tail='')=>`<div class="listrow">${square(k)}<div><b>${title}</b><small>${sub}</small></div><div class="right">${tail}</div></div>`;
const statusline=(k,text)=>`<div class="lineitem"><span class="dot">${icon(k)}</span><span>${text}</span></div>`;
const topgap='<div class="fill"></div>';
const fixture={order:'S-0142',pickup:'A17',merchant:'여울 커피 성수점',network:'StableNet 테스트넷',address:demoValue('{{address:0x62c9…8e41}}'),hash:demoValue('{{hash:0xa17f…d142}}')};

'use strict';
const {pages,refs,products,board} = JSON.parse(document.querySelector('#book-data').textContent,(_key,value)=>demoValue(value));
const byId = new Map(pages.map(p=>[p.id,p]));
const productOf = new Map(products.products.flatMap(x=>x.pages.map(id=>[id,x])));
const main = document.querySelector('main');
const toneIcon={normal:'○',success:'✓',warning:'◷',blocked:'⊘'};
const kindLabel={home:'홈',form:'정보 입력',review:'내용 확인',receipt:'결과 상세',list:'목록·탐색',monitor:'상태 조회'};
function esc(v){return String(v).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));}
function url(target){return '#'+target.replace('#','/');}
function destination(target){const [id,state='start']=target.split('#');const p=byId.get(id);return p.title+' · '+p.states.find(s=>s.id===state).label;}
function link(label,target,cls=''){return `<a class="${cls}" href="${esc(url(target))}">${esc(label)}</a>`;}
function productSurface(p){return productOf.get(p.id).surface;}
function boardFlows(p){return [...new Set(p.states.map(s=>board.states[p.id+'#'+s.id]?.[0]).filter(Boolean))];}
function inScope(p,scope){return !scope||(scope==='hardware'?boardFlows(p).length>0:productOf.get(p.id).id===scope);}
function choices(q='',scope=''){
 const text=q.trim().toLowerCase();
 return pages.filter(p=>inScope(p,scope)&&(!text||[p.id,p.title,p.role,p.area,...p.refs,...p.states.map(s=>s.label)].join(' ').toLowerCase().includes(text)));
}
function tile(p,extra=''){return `<a class="tile" href="${url(p.id)}"><span class="pagecode">${p.id}</span><span><b>${esc(p.title)}</b><small>${esc(extra||p.role)} · ${p.states.length}개 상태</small></span><span class="go">›</span></a>`;}
function showList(q,scope){
 const items=choices(q,scope);
 const list=document.querySelector('#page-list');
 const groups=products.products.map(x=>{
  const found=items.filter(p=>inScope(p,scope)&&productOf.get(p.id).id===x.id&&(scope!=='hardware'));
  if(!found.length)return '';
  return `<section class="index-section"><div class="listhead"><h2>${esc(x.name)}</h2><span>${found.length}페이지</span></div><p>${esc(x.summary)}</p><div class="tiles">${found.map(p=>tile(p)).join('')}</div></section>`;
 });
 const hw=items.filter(p=>boardFlows(p).length);
 const steps=Object.entries(board.steps).filter(([k])=>!['done','failed'].includes(k));
 if(hw.length&&(!scope||scope==='hardware'))groups.push(`<section class="index-section hardware"><div class="listhead"><h2>${esc(products.hardware.name)}</h2><span>보드 단계를 안내하는 모바일 페이지 ${hw.length}개</span></div><p>${esc(products.hardware.summary)}</p>
 <ul class="steplegend">${steps.map(([k,v])=>`<li><b>${esc(v.label)}</b><span>${esc(v.do)}</span></li>`).join('')}</ul>
 <div class="tiles">${hw.map(p=>tile(p,'모바일 앱 표시 · '+boardFlows(p).map(f=>board.flows[f].label).join(', '))).join('')}</div></section>`);
 list.innerHTML=groups.join('');
 if(!items.length)list.innerHTML='<p class="empty">조건에 맞는 페이지가 없습니다. 다른 기능명이나 제품 ID로 찾아보세요.</p>';
 document.querySelector('#result-count').textContent=items.length+'페이지';
}
function indexView(){
 main.innerHTML=`<div class="indexwrap"><header class="indexhero"><div><p class="eyebrow">NU 하드월렛 · 상용 설계</p><h1>페이지별 화면 설계</h1><p>페이지를 열고 사용자 동작과 실패 시 복귀 경로를 따라가세요.</p></div><div class="sum"><strong>${pages.length}</strong><span>페이지</span><strong>${pages.reduce((n,p)=>n+p.states.length,0)}</strong><span>상태</span></div></header>
 <form class="filters" onsubmit="return false"><label>페이지 검색<input id="query" type="search" placeholder="예: 환불, P09.UC4, W10"></label><label>제품<select id="scope"><option value="">전체 제품</option>${products.products.map(x=>`<option value="${x.id}">${esc(x.name)}</option>`).join('')}<option value="hardware">${esc(products.hardware.name)} 단계</option></select></label><output id="result-count" aria-live="polite"></output></form><div id="page-list"></div></div>`;
 const update=()=>showList(document.querySelector('#query').value,document.querySelector('#scope').value);
 document.querySelector('#query').oninput=update;document.querySelector('#scope').onchange=update;update();
}
function getArtwork(p,s){
 if(p.id==='W07'&&s.id==='xrpcard')return '<figure class="previewqr"><img src="receive-xrp.svg" alt="XRPL Testnet, 수신 주소와 DestinationTag 142를 함께 담은 공유 카드 QR"><figcaption>주소·태그·네트워크 공유 카드</figcaption></figure>';
 if(p.id==='W27'&&s.id==='font200')return '<section class="type-sample" aria-label="결제 화면 글자 200퍼센트 예시"><small>결제 확인 · 글자 미리보기</small><strong>여울 커피 성수점</strong><strong>6.50 TestUSDC</strong><span>거절</span></section>';
 const receiveQr={qr:'evm',ethshare:'evm',baseshare:'evm',btcshare:'btc',solshare:'sol'};
 if(p.id==='W07'&&receiveQr[s.id])return '<figure class="previewqr"><img src="receive-'+receiveQr[s.id]+'.svg" alt="아래에 표시한 테스트 수신 주소를 인코딩한 QR"><figcaption>문서용 테스트 주소 · 사용 전 네트워크 확인</figcaption></figure>';
 if(s.id!=='start')return '';
 if(['K04','K08'].includes(p.id))return '<figure class="previewqr"><img src="preview-qr.svg" alt="결제 미리보기용 모의 QR"><figcaption>폰으로 원주문 확인 · 시안용 QR</figcaption></figure>';
 if(p.id==='T01')return '<div class="district" role="img" aria-label="성수역과 여울 커피를 표시한 예시 지도"><span>성수역</span><b>● 여울 커피</b></div>';
 if(['T03'].includes(p.id))return '<div class="shopart" role="img" aria-label="여울 커피 매장 입구 예시"></div>';
 if(p.id==='K01')return '<div class="menuart" aria-hidden="true"><i></i><i></i><i></i></div>';
 return '';
}
const tabs=[['지갑','W01'],['결제','W09'],['둘러보기','W13'],['설정','W27']];
function mobileTabs(p,s){
 if(!['W01','W09','W13','W27','T01','T10','L01'].includes(p.id)||s.id!=='start')return '';
 const current=p.id.startsWith('T')||p.id.startsWith('L')?'W13':p.id;
 return '<nav class="apptabs" aria-label="모바일 기본 탐색">'+tabs.map(([text,id])=>link(text,id,id===current?'here':'')).join('')+'</nav>';
}
function fieldList(s){
 if(!s.inputs?.length)return '';
 return `<div class="inputs" aria-label="입력 양식의 고정 예시">${s.inputs.map(([k,v])=>`<label>${esc(k)}<input type="text" value="${esc(v)}" readonly></label>`).join('')}</div>`;
}
// Board drawing that matches the NU54DK v2 silkscreen: SW4, SW3, SW2, SW1 top to bottom on the
// right edge (P0.04, P1.08, P1.09, P1.13) and LED1-LED4 in one row beside SW2/SW1 (status_led.c
// drives all four together). Each step animates only what the user must do or watch.
const boardButtons=[['SW4',58],['SW3',86],['SW2',114],['SW1',142]];
const boardArtSteps={window:{hold:['SW4']},recovery:{hold:['SW3']},hold:{hold:['SW3']},approve:{tap:['SW1'],alt:['SW2'],led:'wait'},
 pin:{seq:['SW1','SW3'],alt:['SW2'],led:'pin'},update:{press:['SW1','SW2'],later:['SW3'],led:'two'},locate:{led:'locate'},
 display:{display:['독립 표시 장치',['금액','받는 곳','네트워크']]},
 working:{still:true},done:{},failed:{},recoveryproof:{display:['독립 복구 장치',['새 휴대전화 지문','소유 세대']]},confirm:{}};
function boardArt(stepId){
 const a=boardArtSteps[stepId]||{};
 if(a.display){const [name,rows]=a.display;return `<svg class="boardart art-display" viewBox="0 0 120 190" role="img" aria-label="${esc(name)} 그림">
 <rect class="indepbody" x="4" y="44" width="112" height="92" rx="9"/><rect class="indepscreen" x="12" y="52" width="96" height="76" rx="4"/>
 ${rows.map((row,i)=>`<text class="indeprow" x="18" y="${68+i*24}">${esc(row)}</text><path class="indepline" d="M18 ${75+i*24}h${62-i*12}"/>`).join('')}
 <text class="indeptitle" x="60" y="156">${esc(name)}</text></svg>`;}
 const role=name=>a.hold?.includes(name)?'hold':a.tap?.includes(name)?'tap':a.press?.includes(name)?'press':
  a.seq?.includes(name)?'seq-'+name:a.later?.includes(name)?'later':a.alt?.includes(name)?'alt':'';
 const buttons=boardButtons.map(([name,y])=>{const r=role(name);return `<g class="sw ${r}"><rect class="pad" x="66" y="${y-11}" width="22" height="22" rx="4"/><circle class="cap" cx="77" cy="${y}" r="7"/>${r==='hold'?`<circle class="ring" cx="77" cy="${y}" r="13" pathLength="100"/>`:''}<text x="94" y="${y+3.5}">${name}</text></g>`;}).join('');
 const leds=[0,1,2,3].map(i=>`<rect class="led led${i}" x="${14+i*12}" y="126" width="9" height="6" rx="1.5"/>`).join('');
 const marked=boardButtons.map(([n])=>n).filter(n=>role(n)&&role(n)!=='alt'&&role(n)!=='later');
 const said=marked.length?'보드 그림: '+marked.join('·')+' 강조':a.still?'보드 그림: 버튼을 누르지 않는 단계':'보드 그림';
 return `<svg class="boardart art-${esc(stepId)} led-${a.led||'off'}${a.still?' still':''}" viewBox="0 0 120 190" role="img" aria-label="${esc(said)}">
 <rect class="pcb" x="4" y="4" width="112" height="182" rx="8"/><rect class="module" x="14" y="12" width="46" height="34" rx="4"/>
 <text class="mod" x="37" y="32">NU-54V</text><rect class="usb" x="42" y="172" width="36" height="10" rx="3"/>
 ${leds}<text class="ledlabel" x="14" y="143">LED</text>${buttons}</svg>`;
}
function boardPanel(p,s){
 const mark=board.states[p.id+'#'+s.id];
 if(!mark)return '';
 const [flowId,stepId]=mark, flow=board.flows[flowId], step=board.steps[stepId];
 const at=flow.steps.indexOf(stepId), ended=at<0;
 const status=i=>ended?['ended','요청 종료']:i<at?['past','완료']:i===at?['now','현재']:['next','다음'];
 const track=flow.steps.map((id,i)=>{const [cls,said]=status(i);return `<li class="${cls}"${i===at?' aria-current="step"':''}><span>${i+1}</span>${esc(board.steps[id].label)}<em class="sr">${said}</em></li>`;}).join('');
 const keys=step.buttons.map(([b,how])=>`<li><kbd>${esc(b)}</kbd>${esc(how)}</li>`).join('');
 const later=ended?[]:flow.steps.slice(at+1).filter(id=>board.steps[id].buttons.length);
 const ahead=later.length?`<div class="bsahead"><strong>이어서 할 일</strong><ul role="list">${later.map(id=>`<li><b>${esc(board.steps[id].label)}</b> ${board.steps[id].buttons.map(([b,how])=>`<kbd>${esc(b)}</kbd>${esc(how)}`).join(' ')}</li>`).join('')}</ul></div>`:'';
 return `<section class="boardsteps step-${esc(stepId)}" aria-label="하드월렛 진행 단계">
 <div class="bshead"><span>하드월렛 · ${esc(flow.label)}</span><b>${ended?esc(step.label):`${at+1} / ${flow.steps.length}`}</b></div>
 <ol class="bstrack" role="list">${track}</ol>
 <div class="bsnow">${boardArt(stepId)}<div><p class="bsdo"><strong>${ended?'결과':'지금 할 일'}</strong>${esc(step.do)}</p>
 ${keys?`<ul class="bskeys" role="list">${keys}</ul>`:''}${step.led?`<p class="bsled">표시등 · ${esc(step.led)}</p>`:''}</div></div>
 <p class="bsapp">${esc(step.app)}</p>${step.pending?`<p class="bspending">${esc(step.pending)}</p>`:''}${ahead}</section>`;
}
function appPreview(p,s){
 const group=[,,productOf.get(p.id).brand], device=productSurface(p), tone=s.tone||'normal';
 const nav=mobileTabs(p,s);
 const actions=(s.actions||[]).filter(([label,target])=>!nav||!tabs.some(([n,t])=>n===label&&t===target.split('#')[0]));
 const protectedReview=p.id==='W10'&&['start','bleReview','repriced'].includes(s.id);
 const identityKeys=['요청 출처','가맹점','수령 주소'];
 const rows=items=>items.map(([k,v])=>`<div><dt>${esc(k)}</dt><dd>${esc(v)}</dd></div>`).join('');
 const identity=protectedReview?`<dl class="fields recipient">${rows(identityKeys.flatMap(key=>s.rows.filter(([k])=>k===key)))}</dl>`:'';
 const list=rows(protectedReview?s.rows.filter(([k])=>!identityKeys.includes(k)):s.rows);
 const note=`<p class="note">${esc(s.note)}</p>`;
 return `<article class="surface ${device} kind-${p.kind} tone-${tone}" data-page="${p.id}" aria-label="제품 화면">
 <div class="surfacehead"><span class="appword"><span class="mark mini">n</span>${esc(group[2])}</span><span>${esc(p.area)}</span></div>
 <div class="surfacebody"><div class="pageline">${esc(p.title)}</div><span class="condition"><i aria-hidden="true">${toneIcon[tone]}</i> ${esc(s.label)}</span>
 ${identity}<h2 id="screen-title">${esc(s.headline)}</h2>${protectedReview?'':note}${boardPanel(p,s)}${getArtwork(p,s)}${fieldList(s)}
 <dl class="fields">${list}</dl>${protectedReview?note:''}
 <div class="actions">${actions.map(([l,t],i)=>link(l,t,i===0?'primary':'secondary')).join('')}</div></div>${nav}</article>`;
}
function brief(p,s){
 const incoming=[];
 for(const pg of pages) for(const st of pg.states) for(const edge of [...(st.actions||[]),...(st.events||[])]){
  if(url(edge[1])===url(p.id+'#'+s.id)||(s.id==='start'&&edge[1]===p.id))
   if(pg.id!==p.id || st.id!==s.id)incoming.push([pg.title+' · '+st.label,pg.id+'#'+st.id]);
 }
 const unique=[...new Map(incoming.map(pair=>[pair[1],pair])).values()];
 const ev=s.events||[];
 return `<aside class="brief" aria-label="설계 검토 도구"><div class="briefheading"><span class="eyebrow">설계 검토</span><code>${p.id} / ${esc(s.id)}</code></div>
 <label class="chooser">검토할 화면 상태<select id="variant">${p.states.map(st=>`<option value="${esc(st.id)}" ${st.id===s.id?'selected':''}>${esc(st.label)}</option>`).join('')}</select></label>
 <div class="spec"><h3>사용자와 진입 조건</h3><b>${esc(p.role)}</b><p>${esc(p.entry)}</p></div>
 ${ev.length?`<div class="spec eventbox"><h3>외부 사건 발생 시</h3><p>보드·서버·다른 주체의 결과를 시안에 적용합니다.</p>${ev.map(([l,t])=>`<a href="${esc(url(t))}">${esc(l)} <span>→ ${esc(destination(t))}</span></a>`).join('')}</div>`:''}
 <div class="spec"><h3>완료 기준</h3><p>${esc(p.accept)}</p></div>
 <details class="spec"><summary>기능·유즈케이스 원문 (${p.refs.length})</summary>${p.refs.map(k=>`<div class="ref"><a href="${esc(refs[k].source)}">${esc(k)} · ${esc(refs[k].title)}</a><p>${esc(refs[k].criteria)}</p></div>`).join('')}</details>
 ${unique.length?`<details class="spec"><summary>이 상태로 들어오는 경로 (${unique.length})</summary><div class="inlinks">${unique.map(([l,t])=>link(l,t)).join('')}</div></details>`:''}
 <small class="fixture">설계용 고정 데이터 · 입력값과 사건을 예시로 재현합니다. 실제 기기·계정·원장에 연결하지 않습니다.</small></aside>`;
}
function route(){
 const [id='index',state='start']=location.hash.slice(1).split(/[\/#]/);
 if(id==='main-content')return;
 if(!id||id==='index'){document.title='NU · 페이지별 화면 설계';indexView();return;}
 const p=byId.get(id), s=p?.states.find(st=>st.id===state);
 if(!p||!s){main.innerHTML='<section class="indexwrap"><h1>화면을 찾을 수 없습니다</h1><a href="#index">전체 페이지로</a></section>';return;}
 document.title=p.title+' · '+s.label+' | NU';
 main.innerHTML=`<div class="workspace"><header class="context"><div><a href="#index">전체 페이지</a><span> / </span>${esc(productOf.get(p.id).name)}</div><h1>${esc(p.title)}</h1><span class="context-meta">${esc(p.id)} · ${esc(kindLabel[p.kind])} · ${esc(p.role)}</span></header>
 <div class="workgrid"><div class="stage">${appPreview(p,s)}</div>${brief(p,s)}</div></div>`;
 document.querySelector('#variant').onchange=e=>{location.hash=url(p.id+'#'+e.target.value);};
}
window.addEventListener('hashchange',()=>{route();if(location.hash!=='#main-content'){main.focus({preventScroll:true});window.scrollTo(0,0);}});
document.querySelector('.skip').addEventListener('click',event=>{event.preventDefault();main.focus();});
route();

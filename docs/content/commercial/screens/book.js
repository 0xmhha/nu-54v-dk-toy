'use strict';
const {pages,refs} = JSON.parse(document.querySelector('#book-data').textContent,(_key,value)=>demoValue(value));
const byId = new Map(pages.map(p=>[p.id,p]));
const main = document.querySelector('main');
const sections = {
 W:['지갑·보드·클라우드','모바일 지갑','NU Wallet','개인 계정, 자산, 표시와 승인'],
 K:['매장·키오스크','가맹점 단말','여울 커피','매장 주문, POS와 근거리 픽업'],
 T:['여행·예약','모바일 여행','NU Travel','장소 탐색, 예약금과 여행 기록'],
 O:['가맹점 운영','운영 백오피스','NU Merchant','영업 준비, 지급·환불과 마감'],
 L:['테스트 시장','학습 서비스','NU Market','교환과 유동성, 가격과 포지션'],
 X:['체인 영수증','체인 탐색기','NU Explorer','지급 근거, 확정 범위와 재대사'],
 D:['SDK·릴리스','개발·운영 포털','NU Platform','호환성, 설치, 보안과 복구']
};
const toneIcon={normal:'○',success:'✓',warning:'◷',blocked:'⊘'};
const kindLabel={home:'홈',form:'정보 입력',review:'내용 확인',receipt:'결과 상세',list:'목록·탐색',monitor:'상태 조회'};
function esc(v){return String(v).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));}
function url(target){return '#'+target.replace('#','/');}
function destination(target){const [id,state='start']=target.split('#');const p=byId.get(id);return p.title+' · '+p.states.find(s=>s.id===state).label;}
function link(label,target,cls=''){return `<a class="${cls}" href="${esc(url(target))}">${esc(label)}</a>`;}
function productSurface(p){
 if((p.id.startsWith('L')||p.id.startsWith('W')) && /운영자|관리자|감사자|담당자/.test(p.role))return 'desk';
 if(p.id.startsWith('K') && /개인 폰|고객 폰|개인 장바구니|근거리 고객/.test(p.role+' '+p.title))return 'mobile';
 if(['W','T','L'].includes(p.id[0]))return 'mobile';
 return p.id[0]==='K'?'kiosk':'desk';
}
function choices(q='',scope=''){
 const text=q.trim().toLowerCase();
 return pages.filter(p=>(!scope||p.id[0]===scope)&&(!text||[p.id,p.title,p.role,p.area,...p.refs,...p.states.map(s=>s.label)].join(' ').toLowerCase().includes(text)));
}
function showList(q,scope){
 const items=choices(q,scope);
 const list=document.querySelector('#page-list');
 list.innerHTML=Object.entries(sections).map(([prefix,[name,,,desc]])=>{
  const found=items.filter(p=>p.id[0]===prefix);
  if(!found.length)return '';
  return `<section class="index-section"><div class="listhead"><h2>${esc(name)}</h2><span>${found.length}페이지</span></div><p>${esc(desc)}</p><div class="tiles">${found.map(p=>`<a class="tile" href="${url(p.id)}"><span class="pagecode">${p.id}</span><span><b>${esc(p.title)}</b><small>${esc(p.role)} · ${p.states.length}개 상태</small></span><span class="go">›</span></a>`).join('')}</div></section>`;
 }).join('');
 if(!items.length)list.innerHTML='<p class="empty">조건에 맞는 페이지가 없습니다. 다른 기능명이나 제품 ID로 찾아보세요.</p>';
 document.querySelector('#result-count').textContent=items.length+'페이지';
}
function indexView(){
 main.innerHTML=`<div class="indexwrap"><header class="indexhero"><div><p class="eyebrow">NU 하드월렛 · 상용 설계</p><h1>페이지별 화면 설계</h1><p>페이지를 열고 사용자 동작과 실패 시 복귀 경로를 따라가세요.</p></div><div class="sum"><strong>${pages.length}</strong><span>페이지</span><strong>${pages.reduce((n,p)=>n+p.states.length,0)}</strong><span>상태</span></div></header>
 <form class="filters" onsubmit="return false"><label>페이지 검색<input id="query" type="search" placeholder="예: 환불, P09.UC4, W10"></label><label>제품 영역<select id="scope"><option value="">전체 영역</option>${Object.entries(sections).map(([k,[n]])=>`<option value="${k}">${esc(n)}</option>`).join('')}</select></label><output id="result-count" aria-live="polite"></output></form><div id="page-list"></div></div>`;
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
function appPreview(p,s){
 const group=sections[p.id[0]], device=productSurface(p), tone=s.tone||'normal';
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
 ${identity}<h2 id="screen-title">${esc(s.headline)}</h2>${protectedReview?'':note}${getArtwork(p,s)}${fieldList(s)}
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
 main.innerHTML=`<div class="workspace"><header class="context"><div><a href="#index">전체 페이지</a><span> / </span>${esc(sections[p.id[0]][0])}</div><h1>${esc(p.title)}</h1><span class="context-meta">${esc(p.id)} · ${esc(kindLabel[p.kind])} · ${esc(p.role)}</span></header>
 <div class="workgrid"><div class="stage">${appPreview(p,s)}</div>${brief(p,s)}</div></div>`;
 document.querySelector('#variant').onchange=e=>{location.hash=url(p.id+'#'+e.target.value);};
}
window.addEventListener('hashchange',()=>{route();if(location.hash!=='#main-content'){main.focus({preventScroll:true});window.scrollTo(0,0);}});
document.querySelector('.skip').addEventListener('click',event=>{event.preventDefault();main.focus();});
route();

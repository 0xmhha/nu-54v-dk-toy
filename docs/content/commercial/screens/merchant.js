function retailHeader(mode){return `<header class="retailhead"><div class="row"><span class="brandglyph">${icon('coffee')}</span><span class="logo">여울 커피</span><span class="divide"></span><b>성수점</b></div><div class="row">${badge(mode,'',mode==='결제 전용'?'card':'bag')}${badge('StableNet 테스트넷','mint')}<span class="iconlabel small">${icon('globe')} 한국어</span></div></header>`}
function kiosk(){
 const menu=(n,name,price)=>`<article class="menucard">${photo(n,name)}<div class="info"><h3>${name}</h3><div class="between"><b>${price} <small>TestUSDC</small></b><span class="add">${icon('plus')}</span></div></div></article>`;
 const item=(n,name,price,option)=>`<div class="row th">${photo(n,name,'thumb')}<div><strong>${name}</strong><p class="muted small">${option} · 1개</p></div><b class="right">${price}</b></div>`;
 return `<div class="desk">${retailHeader('키오스크 모드')}<div class="ordergrid"><section>
 <div class="shophero">${photo(4,'여울 커피 성수점 입구')}<h1>메뉴를 골라주세요</h1><p>매장 픽업 · 가격 단위 TestUSDC</p></div>
 <div class="category"><span class="on">전체 메뉴</span><span>커피</span><span>베이커리</span></div>
 <div class="menugrid">${menu(1,'아이스 아메리카노','3.50')}${menu(2,'카페라테','4.50')}${menu(3,'크루아상','3.00')}</div>
 <div class="nearby between"><div class="row">${square('ble')}<div><b>내 폰에서 바로 주문</b><p class="muted small" style="margin-top:4px">다른 손님을 기다리지 않고 내 장바구니를 만들어요.</p></div></div><b>이용 방법　›</b></div></section>
 <aside class="cart"><div class="between"><h2>내 장바구니</h2>${badge('2개')}</div><div class="muted small" style="margin:8px 0 11px">주문 S-0142 · 매장 픽업</div>
 ${item(1,'아이스 아메리카노','3.50','기본 사이즈')}<div class="sep"></div>${item(3,'크루아상','3.00','기본')}<div class="carttotal"><div class="between"><b>합계</b>${badge('테스트 결제','mint')}</div>${amount('6.50','detailprice')}</div>${btn('결제 방법 선택','wide','card')}</aside></div></div>`;
}
function terminal(){
 return `<div class="desk">${retailHeader('결제 전용')}<div class="terminal">
 <section class="termprice">${contours()}${badge('POS 주문 S-0142','mint','receipt')}<div style="margin-top:39px"><h1>결제할 금액</h1>${amount('6.50')}<p style="color:#d3e4cd">여울 커피 성수점</p></div>${topgap}
 ${pair('네트워크','StableNet 테스트넷 · 8283')}${pair('가스 비용','매장 부담')}${pair('상태','지급자 선택 대기')}</section>
 <section class="connectbox"><div class="between"><h2>내 폰을 연결하세요</h2>${badge('미리보기','amber','clock')}</div><p class="muted" style="margin-top:8px">QR을 읽거나 앱에서 근처 단말을 찾으세요.</p>
 <div class="qrwrap"><img width="165" height="165" src="preview-qr.svg" alt="설계 시안용 예시 QR"></div><div class="sep"></div>
 <div style="margin-top:19px"><b class="t19">폰에 표시된 연결 표식</b><p class="muted small" style="margin-top:5px">같은 표식을 선택하면 해당 폰에서 금액을 확인해요.</p></div>
 <div class="box between" style="margin:16px 0"><div class="row">${square('phone')}<div><b>잎새 42</b><p class="muted small">선택 가능한 폰 1대</p></div></div>${icon('next')}</div>
 ${btn('이 표식의 폰 선택','wide','phone')}<p class="muted small" style="text-align:center;margin-top:12px">결제 승인은 연결된 하드월렛에서 진행해요.</p></section></div></div>`;
}
const records=[
 ['10:51','S-0143','확인 중','','4.50'],
 ['10:42','S-0142','지급 확정','mint','6.50'],
 ['10:29','S-0141','지급 확정','mint','4.50'],
 ['10:18','S-0140','부분 환불','rose','−1.50'],
 ['09:56','S-0140','지급 확정','mint','8.00'],
 ['09:34','S-0139','지급 확정','mint','3.00']
];
function operations(){
 const sn=(k,title,active=false)=>`<div class="sidenav ${active?'on':''}">${icon(k)}${title}</div>`;
 const metric=(title,value,sub)=>`<div class="metric"><small>${title}</small><strong>${value}</strong><small>${sub}</small></div>`;
 const rows=records.map(([at,id,status,c,n])=>`<tr class="${id==='S-0142'?'sel':''}"><td>${at}</td><td class="mono">${id}</td><td>${badge(status,c||'amber')}</td><td><b>${n}</b></td></tr>`).join('');
 return `<div class="desk ops"><aside class="sidebar">${brand('Merchant')}<div class="sidegroup">매장 관리</div>${sn('shop','지점 설정')}${sn('list','메뉴·재고')}${sn('bag','주문·픽업')}
 <div class="sidegroup">재무</div>${sn('chart','지급·정산',true)}${sn('recv','환불 관리')}${sn('calendar','영업일 마감')}
 ${topgap}<div class="sidenav">${icon('user')}점주 계정</div></aside>
 <section class="opsmain"><div class="between"><div class="crumb">여울 커피　/　성수점　/　재무</div>${badge('테스트넷','mint')}</div>
 <div class="between"><div><h1>지급·정산</h1><p class="muted" style="margin-top:8px">등록 주소의 실입금과 주문 원장을 대조합니다.</p></div>${btn('CSV 내보내기','outline','download')}</div>
 <div class="row" style="margin-top:22px">${badge('성수점 ⌄','white','shop')}${badge('11월 20일 · 서울 ⌄','white','calendar')}${badge('TestUSDC ⌄','white')}<span class="right small muted">갱신 11:05</span></div>
 <div class="metrics">${metric('확정 매출','22.00','지급 4건') }${metric('확정 환불','1.50','별도 재원 · 1건') }${metric('순매출','20.50','매출 − 환불') }${metric('대사 차이','0.00','TestUSDC')}</div>
 <div class="opslower"><section class="box"><div class="between"><h2>거래 원장</h2><span class="muted small">TestUSDC</span></div>
 <table class="ledger"><thead><tr><th>시각</th><th>주문 번호</th><th>상태</th><th>금액</th></tr></thead><tbody>${rows}</tbody></table><p class="muted small" style="margin-top:13px">확인 중인 거래는 확정 매출에서 제외합니다.</p></section>
 <aside class="box"><div class="between"><b>선택한 지급</b>${badge('지급 확정','mint','check')}</div><h2 style="margin-top:12px">S-0142</h2>${amount('6.50','detailprice')}<div class="sep"></div>
 ${pair('등록 주소 입금','+6.50 TestUSDC')}${pair('환불 누계','0.00 TestUSDC')}${pair('수령 주소','0x62c9…8e41')}${pair('확정 시각','10:42')}${btn('체인 증거 보기','soft wide','layers')}${btn('환불 신청','outline wide')}</aside></div></section></div>`;
}
function explorer(){return `<div class="desk"><header class="explorerhead">${brand('Explorer')}<div class="row">${badge('StableNet · 8283','mint','layers')}${badge('읽기 전용','white','eye')}</div></header><section class="explorerbody">
 <div class="crumb">탐색　/　StableNet 테스트넷　/　지급 영수증</div><h1>영수증 탐색기</h1>
 <div class="searchbar">${icon('search')}<b>성수점 · S-0142</b><span class="right small">주문 또는 트랜잭션 검색</span></div>
 <div class="exgrid"><section class="box"><div class="between"><b>지급 영수증</b>${badge('체인 확정','mint','check')}</div>
 ${amount('6.50','detailprice')}${pair('주문 참조','S-0142')}${pair('수령 주소','0x62c9…8e41')}${pair('자산','TestUSDC')}${pair('배포 프로필','StableNet 테스트넷 · v2')}${pair('이벤트','가맹점 직접 지급')}<small class="muted">조회 범위 · 공개 체인 증거</small></section>
 <aside class="box"><div class="row">${square('layers')}<h2>확정 근거</h2></div><div style="margin:17px 0"><small class="muted">트랜잭션 해시</small><div class="hash mono">${fixture.hash}</div></div>
 ${pair('포함 블록','25,810,240')}${pair('로그 인덱스','2')}${pair('마지막 관측 높이','25,810,251')}${pair('관측 시각','11월 20일 10:43')}<div class="softbox small">조회 시점의 정규 체인과 확정 블록을 대조했습니다.</div></aside></div></section></div>`}

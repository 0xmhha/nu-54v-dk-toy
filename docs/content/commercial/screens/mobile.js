function wallet(){
 const home=phone(`<div class="bar">${brand()}${badge('보드 연결','mint','ble')}</div>
 <div class="between"><h1>내 지갑</h1>${badge('대여 지갑 ⌄')}</div>
 <section class="homecard">${contours()}<div class="between"><b>결제 가능 예치금</b>${icon('eye')}</div>${amount('128.50')}<span class="sub">● &nbsp;StableNet 테스트넷 · 8283</span></section>
 <div class="pillbar">${btn('QR 결제','soft','qr')}${btn('결제 내역','outline','receipt')}</div>
 <div class="sectionlabel">내 자산<span>네트워크 관리 ›</span></div>
 <div class="assetrow"><span class="coin">$</span><div><b>TestUSDC</b><p>StableNet · 예치금</p></div><b class="right">128.50</b></div>
 <div class="sep"></div><div class="sectionlabel">확인할 결제<span>1건</span></div>
 <div class="box">${badge('검토 대기','amber','clock')}<div class="between" style="margin-top:11px"><b>여울 커피 성수점</b><b>6.50</b></div><div class="between muted small" style="margin-top:3px"><span>주문 S-0142</span><span>TestUSDC</span></div></div>
 ${topgap}<p class="muted small">마지막 동기화 · 오늘 10:41</p>`,'지갑');
 const review=phone(`${bar('매장 결제',badge('테스트넷'))}
 <div><div class="caption">주문 S-0142 · 가맹점 요청</div><h1 style="margin-top:6px">결제 내용을 확인하세요</h1></div>
 <div class="box"><div class="row">${square('coffee')}<div><b>여울 커피 성수점</b><p class="caption">검증된 가맹점</p></div></div><div class="sep" style="margin:14px 0"></div>${amount('6.50','samount')}</div>
 <div>${pair('네트워크','StableNet · 8283') }${pair('수령 주소','0x62c9…8e41　›') }${pair('가스 비용','매장 부담') }${pair('결제 원천','대여 지갑 예치금')}</div>
 ${topgap}<div class="securepanel"><div class="row">${square('chip')}<div><small>실물 승인 대기</small><strong>보드의 SW1을 누르세요</strong></div></div><p class="small" style="margin-top:9px">금액과 받는 곳이 맞으면 승인하세요.<br>보드에서 거절하려면 SW2를 누르세요.</p></div>
 ${btn('이 요청 거절','outline wide','close')}`);
 const pickup=phone(`${bar('주문 상세',badge('테스트넷'))}<div>${badge('결제 확정','mint','check')}<h1 style="margin-top:10px">주문을 접수했어요</h1></div>
 <div class="ticket"><b>내 픽업 번호</b><div class="bignum">A17</div><span>여울 커피 성수점</span></div>
 <div class="sectionlabel">픽업 상태 ${badge('준비 중','amber','clock')}</div>
 <p class="caption">메뉴가 준비되면 이 화면에서 알려드려요.</p>
 <div class="box"><div class="between"><b>결제 영수증</b>${icon('receipt')}</div><div class="sep" style="margin:10px 0 4px"></div>${pair('주문 번호','S-0142')}${pair('결제 금액','6.50 TestUSDC')}${pair('확정 시각','오늘 10:42')}</div>
 ${topgap}${btn('영수증 자세히','soft wide','receipt')}`,'결제','10:42');
 return mobile(home,review,pickup);
}
function mapArt(){return `<div class="map" role="img" aria-label="성수역과 여울 커피를 표시한 예시 지도"><svg viewBox="0 0 340 162" preserveAspectRatio="xMidYMid slice">
 <rect width="340" height="162" fill="#e6e8db"/><g fill="#cad7b7"><path d="M7 7h75v52L34 74 3 50z"/><path d="m234 87 69-40 35 37v75l-58 12z"/></g>
 <g fill="none" stroke="#fdfcf4" stroke-width="10"><path d="m-25 110 114-72 72 21 114-78 M-13 45l143 75 177-88 M31 180l104-224 M175 175 242-15 M-20 145l118 12 177-38 80-8"/></g>
 <path d="M-10 125 115 64 192 87 351 10" fill="none" stroke="#c2b89f" stroke-width="8"/><path d="M-10 125 115 64 192 87 351 10" fill="none" stroke="#fcf4d5" stroke-width="5"/>
 <g font-family="Apple SD Gothic Neo,sans-serif" font-size="10" fill="#5f6d58"><text x="18" y="34">성수동</text><text x="39" y="120">성수역</text></g><path d="M165 53a13 13 0 0 1 26 0c0 9-13 22-13 22s-13-13-13-22" fill="#1c4938"/><circle cx="178" cy="53" r="4" fill="#d4f59c"/><rect x="191" y="40" width="60" height="20" rx="8" fill="white"/><text x="198" y="54" font-size="10" font-family="Apple SD Gothic Neo,sans-serif" fill="#1b4134">여울 커피</text>
 </svg>${badge('예시 지도','white')}</div>`}
function discover(){
 const setup=phone(`${bar('기기 설정',badge('1 / 4'))}<div><div class="caption">첫 연결</div><h1>내 보드를 연결하세요</h1></div>
 <div class="boardimg"><img alt="NU-54V-DK 보드 참고 사진" src="../../assets/nu-54v-dk-official.jpg"></div>
 <div class="group">${statusline('check','보드에 전원을 연결하세요.') }${statusline('ble','보드의 페어링 모드를 켜세요.') }${statusline('phone','내 보드를 선택해 등록하세요.')}</div>
 <div class="softbox"><b>PIN은 보드에서 입력해요</b><p class="caption" style="margin-top:5px">지금은 기기를 찾는 단계예요. PIN 설정은 연결을 확인한 뒤 시작해요.</p></div>
 ${topgap}${btn('근처 보드 찾기','wide','ble')}${btn('기기 없이 연습','outline wide','book')}`);
 const travel=phone(`<div class="bar">${brand()}${icon('search')}</div><div class="between"><h1>둘러보기</h1>${badge('지역 직접 선택')}</div>
 <div class="softbox row" style="padding:11px">${icon('pin')}<b>서울 성수동</b><span class="right">변경 ›</span></div>
 ${mapArt()}<div class="sectionlabel">이 지역의 장소<span>목록 보기 ›</span></div>
 ${photo(4,'햇살이 비치는 여울 커피 매장 입구','shopimg')}<div><div class="between"><h2>여울 커피 성수점</h2>${badge('영업 중','mint')}</div><p class="caption">카페 · 사전 주문 및 픽업 가능</p></div>
 <div class="pillbar">${btn('메뉴 미리 주문','')}${btn('좌석 예약','outline')}</div>`,'둘러보기');
 const sub=phone(`${bar('구독 상세',badge('테스트넷'))}<div><div class="caption">11월 청구서 · 3회차</div><h1>커피 정기권</h1></div>
 <div class="homecard">${contours()}<b>이번 회차 청구액</b>${amount('12.00')}<span class="sub">여울 커피 · StableNet 테스트넷</span></div>
 <div>${pair('청구 기간','11월 1일 – 11월 30일')}${pair('납부 기한','11월 30일 18:00')}${pair('약관','버전 1.2　›')}</div>
 <div class="softbox"><div class="row">${icon('chip')}<b>이번 회차만 승인해요</b></div><p class="caption" style="margin-top:6px">내 보드에서 승인해야 결제돼요. 알림을 열어도 돈이 나가지 않아요.</p></div>
 ${topgap}${btn('이번 회차 검토','wide','receipt')}${btn('다음 회차부터 해지','outline wide')}`,'결제');
 return mobile(setup,travel,sub);
}
function permissions(){
 const app=phone(`${bar('DApp 연결')}<div class="largeicon">${icon('globe')}</div><div><div class="caption">서비스에서 연결을 요청했어요</div><h1>계정을 연결할까요?</h1></div>
 <div class="box"><b class="mono t16">travel.example</b>${pair('공개할 계정','여행용 01')}${pair('네트워크','Base · 테스트넷')}${pair('허용 범위','선택한 주소 조회')}${pair('세션 만료','오늘 18:00')}</div>
 <p class="caption">로그인 서명과 자산 사용은 별도로 확인해요.</p>${topgap}${btn('계정 연결 허용','wide')}${btn('거절','outline wide')}`);
 const risk=phone(`${bar('요청 검토',badge('서명 차단','rose','lock'))}<div class="largeicon">${icon('shield')}</div><div><div class="caption">토큰 사용 권한 요청</div><h1>보호된 표시 수단이<br>필요해요</h1></div>
 <div class="box"><b>토큰 사용 한도 변경</b>${pair('요청 출처','travel.example')}${pair('현재 표시 수단','일반 휴대폰 화면')}</div>
 <div class="notice">${icon('info')}<span>허용 대상과 금액을 독립된 화면에서 확인해야 해요. 지금은 보드 서명이 차단돼 있어요.</span></div>${topgap}${btn('지원 장치 연결 방법','soft wide','chip')}${btn('요청 닫기','outline wide')}`);
 const unknown=phone(`${bar('주문 상세',badge('테스트넷'))}<div class="largeicon" style="background:var(--amberbg);color:var(--amber)">${icon('clock')}</div><div><div class="caption">주문 S-0142</div><h1>결제 확인 중</h1></div>
 <p class="caption">거래를 보냈어요. 체인에서 결과를 확인하면 이 주문에 표시해요.</p><div class="box">${pair('받는 곳','여울 커피 성수점')}${pair('보낸 금액','6.50 TestUSDC')}${pair('마지막 조회','오늘 10:41:56')}</div>
 <div class="softbox"><b>이 화면을 닫아도 괜찮아요</b><p class="caption" style="margin-top:6px">결제 탭에서 같은 주문을 다시 열 수 있어요.</p></div>${topgap}${btn('기존 거래 상태 조회','wide','refresh')}`,'결제');
 return mobile(app,risk,unknown);
}

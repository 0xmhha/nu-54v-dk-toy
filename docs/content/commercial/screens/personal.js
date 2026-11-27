function personal(){
 const token=(symbol,color)=>`<span class="tokenicon" style="background:${color}">${symbol}</span>`;
 const home=phone(`<div class="bar">${brand()}${badge('보드 연결','mint','ble')}</div>
 <div class="between"><h1>내 지갑</h1>${badge('개인 하드월렛 ⌄')}</div>
 <section class="homecard"><div class="between"><b>기본 계정 · 사용 가능 잔액</b>${icon('eye')}</div>${contours()}
 <div class="amount">0.4200<span class="unit">ETH</span></div><span class="sub">● &nbsp;Ethereum Sepolia · 테스트넷</span></section>
 <div class="pillbar">${btn('받기','soft','recv')}${btn('보내기','outline','send')}</div>
 <div class="sectionlabel">선택한 네트워크<span>변경 ›</span></div>
 <div class="assetrow">${token('Ξ','#616f9a')}<div><b>Ethereum</b><p>Sepolia · 계정 01</p></div>${icon('next')}</div><div class="sep"></div>
 <div class="sectionlabel">최근 거래<span>모두 보기 ›</span></div><div>${line('recv','ETH 받기','오늘 09:18 · 확정','<b>+0.1000</b>')}${line('send','ETH 보내기','어제 16:24 · 확정','<b>−0.0200</b>')}</div>
 ${topgap}<p class="muted small">마지막 동기화 · 오늘 10:41</p>`,'지갑');
 const chain=(symbol,color,n,sub,active=false)=>`<div class="assetrow netrow">${token(symbol,color)}<div><b>${n}</b><p>${sub}</p></div><span class="right">${active?badge('선택됨','mint','check'):icon('next')}</span></div>`;
 const switcher=phone(`${bar('네트워크 선택',badge('테스트넷','mint'))}<div><h1>사용할 네트워크</h1><p class="caption" style="margin-top:5px">선택한 네트워크의 계정과 자산을 표시해요.</p></div>
 <div class="seg"><span>메인넷</span><b>테스트넷</b></div>
 <div class="chainlist">${chain('Ξ','#616f9a','Ethereum','Sepolia',true)}${chain('₿','#a96723','Bitcoin','Bitcoin 테스트 네트워크')}${chain('S','#604a88','Solana','Devnet')}${chain('X','#39413f','XRP Ledger','Testnet')}${chain('B','#315dad','Base','Base Sepolia')}${chain('N','#347256','StableNet','체인 ID 8283')}</div>
 ${topgap}${btn('커스텀 네트워크 추가','outline wide','plus')}`);
 const custom=phone(`${bar('네트워크 추가',badge('EVM'))}<div><h1>연결 정보 확인</h1><p class="caption" style="margin-top:5px">입력한 체인과 RPC 응답을 대조해요.</p></div>
 <div class="group"><div class="field"><span>네트워크 이름</span><b>내 테스트 체인</b></div><div class="field"><span>요청한 체인 ID</span><b class="mono">77702</b></div><div class="field"><span>RPC 주소</span><b class="mono">https://rpc.example</b></div></div>
 <div class="notice">${icon('info')}<div><b>체인 ID가 일치하지 않아요</b><p style="margin-top:5px">이 예시 RPC의 응답은 8283이에요. 주소와 체인 ID를 다시 확인하세요.</p></div></div>
 ${topgap}${btn('연결 정보 수정','wide','settings')}<p class="caption" style="text-align:center">검증을 마치면 네트워크를 추가할 수 있어요.</p>`);
 return mobile(home,switcher,custom);
}

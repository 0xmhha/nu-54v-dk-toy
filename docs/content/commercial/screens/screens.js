const pages={
 personal:['모바일 · 개인 하드월렛',personal],
 wallet:['모바일 · 지갑과 결제',wallet],
 discover:['모바일 · 연결과 여행, 구독',discover],
 permissions:['모바일 · 권한 요청과 복구',permissions],
 kiosk:['키오스크 · 주문 모드',kiosk],
 terminal:['키오스크 · 결제 전용',terminal],
 operations:['백오피스 · 지급과 정산',operations],
 explorer:['탐색기 · 지급 영수증',explorer]
};
const selector=document.getElementById('pages');
const canvas=document.getElementById('canvas');
for(const [id,[label]] of Object.entries(pages))selector.add(new Option(label,id));
function render(){
 const id=Object.hasOwn(pages,location.hash.slice(1))?location.hash.slice(1):'personal';
 selector.value=id;
 canvas.innerHTML=pages[id][1]();
 canvas.setAttribute('aria-label',pages[id][0]);
 document.title='NU · '+pages[id][0];
}
selector.addEventListener('change',()=>{location.hash=selector.value});
window.addEventListener('hashchange',render);
render();

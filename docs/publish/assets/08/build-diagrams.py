"""Original bilingual code diagrams for article 08; no mock app screenshots."""
from pathlib import Path
import importlib.util
from PIL import Image, ImageDraw
OUT=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('series_style',OUT.parent/'06'/'build-diagrams.py')
style=importlib.util.module_from_spec(spec);spec.loader.exec_module(style)
text,arrow,footer=style.text,style.arrow,style.footer
BG,INK,MUTED,TEAL,PALE,GOLD,LINE=style.BG,style.INK,style.MUTED,style.TEAL,style.PALE,style.GOLD,style.LINE

def base(title,sub):
 im=Image.new('RGB',(1600,1100),BG);d=ImageDraw.Draw(im)
 text(d,65,38,'NU-54V-DK / 08',24,TEAL)
 text(d,65,92,title,42,height=70)
 text(d,65,170,sub,27,MUTED,height=80)
 return im,d

def panel(d,rect,title,body,fill=PALE,size=26):
 x,y,x2,y2=rect;d.rounded_rectangle(rect,18,fill=fill,outline=LINE,width=2)
 text(d,x+22,y+17,title,28,TEAL,width=x2-x-44,height=80)
 text(d,x+22,y+82,body,size,width=x2-x-44,height=y2-y-95)

def flow(en):
 im,d=base('Display, button approval, then settlement' if en else '내용 표시, 버튼 승인, 정산 완료를 나눕니다',
   'Final-outcome path · early refusals and cancellation still need phone updates' if en else '최종 결과 전달 경로 · 조기 거절·취소의 폰 알림은 보완이 필요합니다')
 for x,label in [(255,'KIOSK' if en else '키오스크'),(800,'DEVICE' if en else '결제 기기'),(1345,'PHONE' if en else '폰 앱')]:
  text(d,x-115,270,label,32,TEAL,width=280,height=50)
  d.line((x,330,x,970),fill=LINE,width=3)
 arrow(d,(255,395),(800,395));text(d,285,343,'1  payment.prepare',29,width=490,height=45)
 text(d,290,420,'Device validates the order' if en else '기기가 주문 검사',25,MUTED,width=475,height=45)
 arrow(d,(800,500),(1345,500));text(d,830,448,'2  confirm.show',29,width=490,height=45)
 text(d,830,520,'Fields from the pending request' if en else '승인 대기 요청의 값 전달',25,MUTED,width=670,height=45)
 panel(d,(580,590,1020,740),'3  SW1 / SW2','Short press: approve / refuse' if en else '짧게 누름: 승인 / 거절',size=25)
 panel(d,(1100,590,1535,740),'DISPLAY ONLY' if en else '화면 확인','No payment approval button' if en else '폰에는 결제 승인 버튼 없음',fill=GOLD,size=25)
 arrow(d,(800,790),(255,790));text(d,285,742,'payment.result',29,width=290,height=45)
 text(d,285,812,'Device approval ≠ settlement' if en else '기기 승인 ≠ 정산 완료',26,MUTED,width=850,height=45)
 d.rounded_rectangle((65,868,620,938),14,fill=PALE,outline=LINE,width=2)
 text(d,85,882,'4  Submit → finalized event' if en else '4  제출 → 확정 이벤트 확인',27,width=515,height=45)
 arrow(d,(640,905),(800,905));arrow(d,(800,905),(1345,905))
 text(d,840,858,'5  payment.outcome',28,width=620,height=45)
 text(d,840,929,'Forward final outcome to the phone' if en else '최종 결과를 폰에 전달',25,MUTED,width=620,height=45)
 footer(d,'Original diagram · current code · sequence only, not measured time or an app screenshot' if en else '직접 작성 · 현재 코드 기준 · 순서 개념도이며 실측 시간·앱 화면 캡처가 아닙니다')
 return im

def precision(en):
 im,d=base('Two decimal places omit part of the amount' if en else '소수점 두 자리 표시는 금액의 일부를 생략합니다',
   'Existing test: 5,009,999 base units · token decimals = 6' if en else '기존 시험 값: 최소 단위 5,009,999 · 토큰 소수점 자릿수 6')
 text(d,70,275,'EXACT REQUEST' if en else '실제 요청 금액',30,TEAL)
 digits=['5','.','0','0','9','9','9','9'];start=240;cw=135
 for i,v in enumerate(digits):
  x=start+i*cw;fill=PALE if i<4 else GOLD
  d.rounded_rectangle((x,345,x+115,465),14,fill=fill,outline=LINE,width=2)
  text(d,x+36,360,v,57,width=80,height=90)
 d.line((start,505,start+3*cw+115,505),fill=TEAL,width=4)
 d.line((start+4*cw,505,start+7*cw+115,505),fill='#9A7031',width=4)
 text(d,start,525,'Displayed: 5.00' if en else '화면에 표시: 5.00',30,TEAL,width=525,height=55)
 text(d,start+4*cw,525,'Omitted: 9999' if en else '생략되는 자리: 9999',30,'#9A7031',width=590,height=55)
 panel(d,(65,650,750,870),'DISPLAY' if en else '표시 문자열','5.00\ntruncatedAmount(5_009_999n, 6)',size=28)
 panel(d,(800,650,1535,870),'PAYMENT AMOUNT' if en else '결제 요청 금액','5,009,999 base units\nExact request stays unchanged' if en else '최소 단위 5,009,999\n원래 요청 금액은 바뀌지 않습니다',fill=GOLD,size=28)
 text(d,75,910,'Another tested case: 999 base units → displayed as 0.00' if en else '같은 시험의 작은 금액: 최소 단위 999 → 화면에는 0.00',29,MUTED,width=1450,height=65)
 footer(d,'Original diagram · source code + existing test values · each cell is one character, not token quantity' if en else '직접 작성 · 코드·기존 시험 값 기준 · 칸은 자릿수이며 토큰 수량의 비례 그래프가 아닙니다')
 return im

def timeouts(en):
 im,d=base('A screen timeout does not cancel a payment' if en else '폰 화면의 시간 초과는 결제 취소가 아닙니다',
   'Configured waits · each row has its own start event · not measured latency' if en else '코드의 대기 설정 · 각 행의 시작 사건이 다름 · 실측 지연이 아닙니다')
 x0,x1=485,1490;scale=(x1-x0)/120
 for sec in [0,10,30,60,90,120]:
  x=x0+sec*scale;d.line((x,315,x,810),fill=LINE,width=1)
  text(d,x-20,270,str(sec),24,MUTED,width=80,height=45)
 text(d,75,268,'Seconds' if en else '초',27,MUTED,width=350,height=50)
 text(d,70,360,'KIOSK' if en else '키오스크',30,TEAL,width=375,height=50)
 text(d,70,425,'Send payment.prepare\nand wait for a result' if en else 'payment.prepare 전송\n기기 결과 대기',26,width=375,height=100)
 d.rounded_rectangle((x0,370,x0+10*scale,430),10,fill=TEAL)
 text(d,x0+10*scale+25,371,'10 s → no result: session.cancel' if en else '10초 → 결과가 없으면 session.cancel',28,TEAL,width=825,height=55)
 text(d,x0,475,'No response is different from an explicit USER_REJECTED' if en else '응답 없음은 USER_REJECTED와 다릅니다',26,MUTED,width=1000,height=60)
 text(d,70,610,'PHONE' if en else '폰 앱',30,TEAL,width=375,height=50)
 text(d,70,675,'Receive confirm.show\nand display details' if en else 'confirm.show 수신\n결제 내용 표시',26,width=375,height=100)
 d.rounded_rectangle((x0,620,x1,680),10,fill=PALE,outline=TEAL,width=2)
 text(d,x0+25,630,'120 s without an outcome → waiting screen' if en else '결과 없이 120초 → 대기 화면으로 복귀',28,TEAL,width=950,height=45)
 text(d,x0,730,'No cancellation is sent by this screen timer' if en else '이 화면 타이머는 취소를 보내지 않습니다',27,MUTED,width=1000,height=60)
 d.rounded_rectangle((65,855,1535,975),18,fill=GOLD)
 text(d,90,873,'After a signature: Checking → recheck the same signature' if en else '서명을 받은 뒤: Checking → 같은 서명으로 결과 재확인',30,width=1410,height=55)
 text(d,90,927,'The submission budget also starts at the request; it is not an extra 10 seconds' if en else '제출 판정 예산도 요청 시점부터 계산하며, 추가 10초를 주는 것이 아닙니다',25,MUTED,width=1410,height=45)
 footer(d,'Original diagram · bar lengths proportional to 10 / 120 seconds · different start events' if en else '직접 작성 · 막대 길이는 10초·120초에 비례 · 각 행은 서로 다른 사건에서 시작합니다')
 return im

if __name__=='__main__':
 for en in [False,True]:
  for stem,fn in [('01-display-and-approval',flow),('02-amount-precision',precision),('03-timeout-boundaries',timeouts)]:
   p=OUT/f'{stem}{"-en" if en else ""}.png';fn(en).save(p);print(p.name)

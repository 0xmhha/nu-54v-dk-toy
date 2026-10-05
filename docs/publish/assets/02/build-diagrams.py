from PIL import Image, ImageDraw, ImageFont
from pathlib import Path
import math
out=Path('docs/publish/assets/02')
W,H=1600,1000
bg='#F7F6F1';ink='#182B30';muted='#52686D';teal='#087C75';pale='#DEF1EC';gold='#F4E7C4';line='#B9C9C8';gray='#E8ECEC';orange='#AF5D32'
font='/System/Library/Fonts/AppleSDGothicNeo.ttc'
def f(n):return ImageFont.truetype(font,n)
def canvas(title,sub):
 im=Image.new('RGB',(W,H),bg);d=ImageDraw.Draw(im)
 d.text((65,50),'NU-54V-DK / 02',font=f(24),fill=teal)
 d.text((65,97),title,font=f(48),fill=ink)
 d.text((65,163),sub,font=f(27),fill=muted)
 return im,d

def text(d,xy,s,size=31,color=ink):
 d.multiline_text(xy,s,font=f(size),fill=color,spacing=13)

def box(d,rect,title,body,fill=pale):
 d.rounded_rectangle(rect,22,fill=fill,outline=line,width=2)
 x,y,x2,y2=rect;text(d,(x+26,y+22),title,32,teal);text(d,(x+26,y+86),body,29)

def arrow(d,p1,p2,color=teal):
 d.line([p1,p2],fill=color,width=5)
 a=math.atan2(p2[1]-p1[1],p2[0]-p1[0]);r=16
 d.polygon([p2,(p2[0]-r*math.cos(a-.5),p2[1]-r*math.sin(a-.5)),(p2[0]-r*math.cos(a+.5),p2[1]-r*math.sin(a+.5))],fill=color)

def foot(d,s):
 d.line([(65,910),(1535,910)],fill=line,width=2);text(d,(65,932),s,23,muted)

for en in (False,True):
 suf='-en' if en else ''
 im,d=canvas('Secure elements have different jobs' if en else '보안칩이 맡는 일은 제품마다 다릅니다','Two documented architectures, simplified' if en else '공식 설명에 나온 두 구조를 단순화했습니다')
 d.rounded_rectangle((65,230,745,865),24,fill='white',outline=line,width=2)
 d.rounded_rectangle((855,230,1535,865),24,fill='white',outline=line,width=2)
 text(d,(100,265),'Ledger',39);text(d,(890,265),'Trezor Safe 3 / 5',39)
 box(d,(100,345,710,650),'Secure element' if en else '보안칩','Generate / store private keys\nSign requests\nControl device display' if en else '개인 키 생성·보관\n서명 처리\n기기 확인 화면 제어')
 text(d,(100,714),'Keys and signing in the SE' if en else '키와 서명의 중심을 보안칩에 둡니다',29)
 box(d,(890,345,1500,525),'Secure element' if en else '보안칩','PIN verification / protected secret' if en else 'PIN 검증 / 비밀값 보호')
 arrow(d,(1195,525),(1195,610));text(d,(1225,549),'With PIN' if en else 'PIN과 함께',25,muted)
 box(d,(890,610,1500,795),'Main chip' if en else '메인 칩','Encrypted private keys' if en else '암호화한 개인 키',gray)
 foot(d,'Source: Ledger / Trezor official explanations • conceptual, not a circuit diagram' if en else '출처: Ledger·Trezor 공식 설명 • 개념도이며 실제 회로도가 아닙니다')
 im.save(out/f'02-secure-element-roles{suf}.png')

 im,d=canvas('Register a request. Approve. Sign inside.' if en else '요청을 등록하고, 버튼으로 승인하고, 내부에서 서명합니다','Target design • secure-world implementation still to be verified' if en else '목표 설계 • 보안 구역 구현과 시험은 후속 작업입니다')
 box(d,(65,250,490,470),'Before approval' if en else '승인 전','Receive kiosk request\nShow details in phone app' if en else '키오스크 요청 수신\n폰 앱에 확인 내용 표시',gray)
 d.rounded_rectangle((550,225,1535,750),24,fill='white',outline=teal,width=3)
 text(d,(575,245),'nRF54L15 / TrustZone',30)
 box(d,(590,325,965,630),'Non-secure world' if en else '일반 구역','Communication\nRequest processing\nLED guidance' if en else '통신\n결제 요청 처리\nLED 안내',gray)
 box(d,(1090,325,1495,630),'Secure world' if en else '보안 구역','TF-M key service\nPrivate key / signing\nApproval button input' if en else 'TF-M 키 서비스\n개인 키 / 서명\n승인 버튼 입력')
 arrow(d,(490,405),(590,405))
 arrow(d,(965,390),(1090,390));text(d,(980,331),'Request' if en else '요청 등록',22)
 arrow(d,(1090,560),(965,560));text(d,(980,586),'Signature' if en else '서명 결과',22)
 box(d,(1090,785,1495,880),'Physical approval' if en else '사람의 버튼 승인','',gold)
 arrow(d,(1292,785),(1292,630))
 arrow(d,(775,630),(775,790));text(d,(545,812),'Return signature to kiosk' if en else '서명 결과를 키오스크로 전달',29)
 foot(d,'No private key export • phone display / signed-request consistency is a separate issue' if en else '개인 키 내보내기 금지 • 폰 표시와 서명 요청의 일치 여부는 별도 문제입니다')
 im.save(out/f'03-trustzone-signing-flow{suf}.png')

 im,d=canvas('A protected key does not prove what you approved' if en else '개인 키를 지켜도, 확인한 결제와 서명할 결제는 다를 수 있습니다','Key confidentiality, physical approval, and trusted display are separate checks' if en else '키 보호 · 버튼 입력 · 화면과 서명 내용의 일치를 각각 확인해야 합니다')
 box(d,(65,245,715,490),'Phone display' if en else '폰 화면','Payment A\nWhat the user sees' if en else '결제 A\n사용자가 확인한 내용',gray)
 box(d,(885,245,1535,490),'Signing request' if en else '서명 요청','Payment B\nWhat the device is asked to sign' if en else '결제 B\n기기에 전달된 서명 대상',gold)
 text(d,(756,331),'≠',72,orange)
 text(d,(65,530),'A compromised phone or non-secure app may create a mismatch.' if en else '폰 앱이나 일반 구역이 침해되면 두 내용이 달라질 수 있습니다.',33,orange)
 box(d,(65,625,535,850),'Key protection' if en else '개인 키 보호','Limits access to key material' if en else '키에 대한 접근을 제한합니다')
 box(d,(565,625,1035,850),'Button approval' if en else '버튼 승인','Confirms a physical press' if en else '실제 버튼 입력을 확인합니다')
 box(d,(1065,625,1535,850),'Display consistency' if en else '표시 내용의 일치','Needs separate assurance' if en else '별도의 보장이 필요합니다',gold)
 foot(d,'A / B are illustrative labels, not real payment records • conceptual diagram' if en else 'A·B는 설명용 개념 표기이며 실제 결제 기록이 아닙니다 • 직접 작성')
 im.save(out/f'04-approval-and-display{suf}.png')

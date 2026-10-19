"""Original bilingual editorial diagrams for installment 04."""
from pathlib import Path
import math
from PIL import Image, ImageDraw, ImageFont

OUT = Path(__file__).resolve().parent
W, H = 1600, 1000
BG, INK, MUTED = '#F7F6F1', '#182B30', '#52686D'
TEAL, PALE, GOLD, LINE, GRAY = '#087C75', '#DEF1EC', '#F4E7C4', '#B9C9C8', '#E8ECEC'
FONT = '/System/Library/Fonts/AppleSDGothicNeo.ttc'


def font(size):
    return ImageFont.truetype(FONT, size)


def text(d, xy, value, size=30, color=INK):
    d.multiline_text(xy, value, font=font(size), fill=color, spacing=12)


def wrapped(d, xy, value, width, size=29, color=INK, max_height=None):
    lines = []
    for para in value.split('\n'):
        line = ''
        for word in para.split(' '):
            trial = f'{line} {word}'.strip()
            if d.textlength(trial, font=font(size)) > width and line:
                lines.append(line)
                line = word
            else:
                line = trial
        lines.append(line)
    value = '\n'.join(lines)
    bounds = d.multiline_textbbox(xy, value, font=font(size), spacing=12)
    assert bounds[2] - xy[0] <= width + 2, value
    if max_height is not None:
        assert bounds[3] - xy[1] <= max_height, value
    text(d, xy, value, size, color)


def canvas(title, subtitle):
    im = Image.new('RGB', (W, H), BG)
    d = ImageDraw.Draw(im)
    text(d, (65, 42), 'NU-54V-DK / 04', 24, TEAL)
    wrapped(d, (65, 94), title, 1470, 46, max_height=66)
    wrapped(d, (65, 168), subtitle, 1470, 27, MUTED, max_height=65)
    return im, d


def card(d, rect, title, body, fill=PALE, title_size=32, body_size=29):
    d.rounded_rectangle(rect, 22, fill=fill, outline=LINE, width=2)
    x, y, x2, y2 = rect
    wrapped(d, (x+25, y+23), title, x2-x-50, title_size, TEAL, max_height=85)
    wrapped(d, (x+25, y+122), body, x2-x-50, body_size, max_height=y2-y-145)


def arrow(d, p1, p2):
    d.line([p1, p2], fill=TEAL, width=5)
    a = math.atan2(p2[1]-p1[1], p2[0]-p1[0])
    d.polygon([p2, (p2[0]-17*math.cos(a-.5), p2[1]-17*math.sin(a-.5)),
               (p2[0]-17*math.cos(a+.5), p2[1]-17*math.sin(a+.5))], fill=TEAL)


def footer(d, value):
    d.line([(65, 916), (1535, 916)], fill=LINE, width=2)
    wrapped(d, (65, 935), value, 1470, 23, MUTED, max_height=48)


def observed_run(en):
    """Time uses a common elapsed axis; block icons show order, not distance."""
    im = Image.new('RGB', (1600, 1100), BG)
    d = ImageDraw.Draw(im)
    text(d, (65, 35), 'NU-54V-DK / 04', 24, TEAL)
    text(d, (65, 83), 'From the request to the payment result' if en else '결제 요청에서 결과 확인까지', 44)
    text(d, (65, 151), 'October 5, 2026 · one observed run · testnet PoC' if en else '2026년 10월 5일 · 한 번의 실행 기록 · 테스트넷 PoC', 27, MUTED)

    # Aligned elapsed-time axis: approval is approximate, result is observed at 7.1 s.
    x0, xe, y = 120, 1450, 442
    xa = round(x0 + (xe-x0)*5.5/7.1)
    d.rounded_rectangle((x0, 355, xa, 455), 18, fill=PALE)
    d.rectangle((xa, 355, xe, 455), fill=GRAY)
    d.line((x0, y, xe, y), fill=TEAL, width=7)
    for second in range(1, 8):
        x = x0 + (xe-x0)*second/7.1
        d.line((x, 458, x, 468), fill=LINE, width=2)
        text(d, (x-8, 477), str(second), 21, MUTED)
    for x, label, num in [(x0, 'Request' if en else '결제 요청', '0 s' if en else '0초'),
                           (xa, 'SW1 approval' if en else 'SW1 승인', '≈ 5.5 s' if en else '약 5.5초'),
                           (xe, 'App result' if en else '앱 결과', '7.1 s' if en else '7.1초')]:
        d.line((x, 310, x, 448), fill=TEAL, width=2)
        d.ellipse((x-13, y-13, x+13, y+13), fill=TEAL, outline=BG, width=3)
        tw=d.textlength(label,font=font(28));text(d,(x-tw/2,222),label,28)
        tw=d.textlength(num,font=font(50));text(d,(x-tw/2,266),num,50,TEAL)
    # Clock icon and human approval cue anchor the meaning of the long wait.
    d.ellipse((155, 365, 225, 435), outline=TEAL, width=4)
    d.line((190, 379, 190, 401, 205, 414), fill=TEAL, width=4)
    text(d,(250,378),'Waiting for human approval' if en else '사람이 버튼을 누를 때까지',31,TEAL)
    d.line((xa, 330, xa, 353),fill=TEAL,width=4)
    d.rounded_rectangle((xa-20,323,xa+20,344),5,fill=TEAL)
    wrapped(d,(xa+22,365),'Following steps\nnot timed separately' if en else '이후 처리 단계\n별도 시간 측정 없음',xe-xa-32,25,MUTED,max_height=82)
    text(d,(120,524),'Elapsed time from request (s) · approval time is approximate' if en else '요청부터 경과 시간(초) · 버튼 승인 시점은 근삿값입니다',25,MUTED)
    d.line((x0,590,xe,590),fill=INK,width=3)
    for x in (x0,xe):d.line((x,577,x,602),fill=INK,width=3)
    note='7.1 s total includes the wait for the person' if en else '전체 7.1초에는 사람이 승인하기까지의 대기가 포함됩니다'
    tw=d.textlength(note,font=font(29));text(d,((1600-tw)/2,614),note,29)

    # One token pictogram communicates the amount; connected blocks explain finality.
    d.rounded_rectangle((65,702,1535,995),22,fill='#FFFFFF',outline=LINE,width=2)
    d.ellipse((100,765,235,900),fill=GOLD,outline='#CCB36E',width=3)
    tw=d.textlength('1',font=font(80));text(d,(167-tw/2,779),'1',80)
    text(d,(108,914),'tUSDC',26,TEAL)
    wrapped(d,(270,751),'Payment amount\nMatching PaymentSettled' if en else '이번 결제 금액\n요청과 맞는 PaymentSettled',350,29,max_height=150)
    arrow(d,(620,835),(680,835))
    # Deliberate ellipsis: these two block heights are not a time scale or count.
    for bx in (705,1180):
        d.rounded_rectangle((bx,775,bx+290,900),14,fill=PALE if bx==705 else GOLD,outline=TEAL,width=3)
    text(d,(725,803),'21463022',43,TEAL)
    text(d,(1200,803),'21463049',43,TEAL)
    text(d,(722,730),'Payment block' if en else '결제 포함 블록',28)
    text(d,(1180,730),'Finalized at check' if en else '확인 시 확정 블록',28)
    d.line((998,835,1175,835),fill=LINE,width=4)
    for cx in (1054,1085,1116):d.ellipse((cx-4,831,cx+4,839),fill=MUTED)
    text(d,(725,930),'Block order shown; spacing is not proportional' if en else '블록의 순서만 표시 · 블록 간 간격은 비례하지 않습니다',24,MUTED)
    d.line((65,1030,1535,1030),fill=LINE,width=2)
    text(d,(65,1050),'Created from the run record · not a benchmark or a separate finality-delay measurement' if en else '직접 작성 · 단일 실행 관찰값이며 성능 보장이나 별도로 측정한 확정 지연이 아닙니다',24,MUTED)
    return im


for en in (False, True):
    suffix = '-en' if en else ''
    im, d = canvas('From button approval to testnet settlement' if en else '보드 버튼에서 키오스크, 테스트넷 정산까지',
                   'October 5, 2026 / one real-hardware payment / testnet PoC' if en else '2026년 10월 5일 / 실제 기기를 사용한 결제 1건 / 테스트넷 PoC')
    card(d, (65, 260, 505, 760), '1. NU-54V-DK', 'Human presses SW1\nBoard signs authorization' if en else '사람이 SW1으로 승인\n보드가 결제에 서명', body_size=28)
    d.rounded_rectangle((135, 515, 435, 705), 15, fill='#202C2F')
    for n in range(4):
        x = 170 + n*68
        d.ellipse((x, 548, x+24, 572), fill='#62C6AB', outline=LINE)
        d.rounded_rectangle((x-2, 615, x+28, 645), 4,
                            fill=GOLD if n == 0 else '#C4CDCE', outline=LINE)
    text(d, (161, 657), 'SW1', 22, GOLD)
    card(d, (585, 260, 1025, 760), '2. Android kiosk' if en else '2. Android 키오스크',
         'Checks device signature\nSubmits transaction' if en else '기기 서명 확인\n트랜잭션 제출', GRAY, body_size=28)
    d.rounded_rectangle((685, 515, 925, 705), 18, fill='#202C2F')
    d.rounded_rectangle((701, 533, 909, 687), 8, fill=BG)
    text(d, (729, 564), '1 tUSDC', 36, TEAL)
    text(d, (725, 632), 'TESTNET', 24, MUTED)
    card(d, (1105, 260, 1535, 760), '3. Settlement' if en else '3. 정산 확인',
         'PaymentSettled\nMatching request\nFinalized block' if en else 'PaymentSettled\n요청 내용 일치\n확정 블록 확인', GOLD, body_size=29)
    arrow(d, (505, 510), (585, 510))
    arrow(d, (1025, 510), (1105, 510))
    wrapped(d, (65, 810), 'Screenless board: four LEDs + four buttons / schematic app screen' if en else '화면 없는 보드: LED 4개 + 버튼 4개 / 앱 화면도 설명용 도식입니다', 1470, 29, MUTED)
    footer(d, 'Original conceptual diagram / not a board photo / no private keys or real addresses' if en else '직접 작성한 개념도 / 실제 보드 사진이 아니며 개인 키와 실제 주소는 생략했습니다')
    im.save(OUT/f'01-hero{suffix}.png')

    im, d = canvas('How we tested the payment' if en else '소프트웨어 서명에서 직접 버튼을 누르기까지',
                   'Software signing, debugger control, and a real button press' if en else '세 시험에서 사용한 서명 방식과 승인 방법')
    columns = [
        ('Software signing\nPart 03' if en else '소프트웨어로 서명\n03편',
         'October 1\nSoftware signing key\nKiosk submission\nSettlement verification' if en else '10월 1일\n소프트웨어 키 서명\n키오스크 제출\n정산 확인', PALE),
        ('Debugger control\nOctober 1' if en else '디버거로 버튼 제어\n10월 1일',
         'Firmware 0.2.0\nBoard-key signature\nDebugger controls pin\nKiosk module on Mac' if en else '펌웨어 0.2.0\n보드 키로 서명\n디버거가 버튼 핀 제어\nMac 키오스크 모듈', GRAY),
        ('A person presses SW1\nOctober 5' if en else '직접 버튼을 눌러 승인\n10월 5일',
         'Firmware 0.3.0\nPerson presses SW1\nAndroid kiosk app\nSecure payment channel' if en else '펌웨어 0.3.0\nSW1을 직접 눌러 승인\nAndroid 키오스크 앱\n결제 보안 채널', GOLD)
    ]
    for n, (title, body, fill) in enumerate(columns):
        x = 65+n*515
        card(d, (x, 265, x+440, 735), title, body, fill, body_size=28)
    wrapped(d, (65, 800), 'Each test used a different signing or approval method.' if en else '서명 방식과 버튼을 누르는 방법을 바꾸며 시험했습니다.', 1470, 31)
    footer(d, 'Created by the author from the October 1 and October 5 test records' if en else '직접 작성 / 10월 1일·5일 시험 기록')
    im.save(OUT/f'02-test-scope{suffix}.png')

    observed_run(en).save(OUT/f'03-observed-run{suffix}.png')

    im, d = canvas('A payment flow verified; protection work still remained' if en else '결제 흐름의 성공과 남은 보호 경계를 나눕니다',
                   'State of the October 5 development build / not a claim about today' if en else '10월 5일 개발 빌드의 상태 / 현재 구현 전체의 평가가 아닙니다')
    card(d, (65, 265, 780, 780), 'Verified in this run' if en else '이번 실행에서 확인했습니다',
         'Board-key payment signature\nPerson presses the approval button\nAndroid kiosk submission\nMatching finalized event\nSecure payment channel' if en else '보드 키의 결제 서명\n사람이 누른 승인 버튼\nAndroid 키오스크 제출\n요청과 맞는 확정 이벤트\n결제 보안 채널', PALE, body_size=31)
    card(d, (820, 265, 1535, 780), 'Not established by this run' if en else '이 실행으로 입증하지 않았습니다',
         'TF-M key isolation\nSecure-world button boundary\nRenter phone confirmation screen\nAgreement between display and signing\nProduction rental setup' if en else 'TF-M 키 격리\n보안 구역의 승인 버튼 경계\n대여자 폰 확인 화면\n표시 내용과 서명 대상의 일치\n운영용 대여 셋업', GOLD, body_size=31)
    wrapped(d, (65, 820), 'Physical approval and informed approval require separate verification.' if en else '버튼을 눌렀는지와 올바른 내용을 보고 눌렀는지는 별도 검증입니다.', 1470, 31)
    footer(d, 'Original diagram / historical test scope / not a product security certification' if en else '직접 작성 / 당시 시험 범위 / 제품 전체의 보안 인증 결과가 아닙니다')
    im.save(OUT/f'04-verified-and-remaining{suffix}.png')

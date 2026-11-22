"""Original bilingual illustrations for article 06; conceptual flows, no invented metrics."""
from pathlib import Path
import math
from PIL import Image, ImageDraw, ImageFont

OUT = Path(__file__).resolve().parent
W, H = 1600, 1100
BG, INK, MUTED = '#F7F6F1', '#182B30', '#52686D'
TEAL, PALE, GOLD, LINE = '#087C75', '#DEF1EC', '#F4E7C4', '#B9C9C8'
FONT = '/System/Library/Fonts/AppleSDGothicNeo.ttc'


def font(n):
    return ImageFont.truetype(FONT, n)


def text(d, x, y, value, size=30, color=INK, width=1470, height=120):
    lines = []
    for para in value.split('\n'):
        line = ''
        for word in para.split():
            trial = (line + ' ' + word).strip()
            if line and d.textlength(trial, font=font(size)) > width:
                lines.append(line)
                line = word
            else:
                line = trial
        lines.append(line)
    result = '\n'.join(lines)
    b = d.multiline_textbbox((x, y), result, font=font(size), spacing=10)
    assert b[2] <= x + width + 2, result
    assert b[3] <= y + height, result
    d.multiline_text((x, y), result, font=font(size), fill=color, spacing=10)


def base(title, sub):
    im = Image.new('RGB', (W, H), BG)
    d = ImageDraw.Draw(im)
    text(d, 65, 38, 'NU-54V-DK / 06', 24, TEAL)
    text(d, 65, 92, title, 46, height=70)
    text(d, 65, 170, sub, 28, MUTED, height=75)
    return im, d


def box(d, rect, title, body, fill=PALE, size=29):
    x, y, x2, y2 = rect
    d.rounded_rectangle(rect, radius=22, fill=fill, outline=LINE, width=2)
    text(d, x+25, y+22, title, 32, TEAL, width=x2-x-50, height=88)
    text(d, x+25, y+110, body, size, width=x2-x-50, height=y2-y-126)


def arrow(d, a, b, color=TEAL, both=False):
    d.line((a, b), fill=color, width=5)
    for p, q in [(a, b)] + ([(b, a)] if both else []):
        angle = math.atan2(q[1]-p[1], q[0]-p[0])
        d.polygon([q, (q[0]-17*math.cos(angle-.5), q[1]-17*math.sin(angle-.5)),
                   (q[0]-17*math.cos(angle+.5), q[1]-17*math.sin(angle+.5))], fill=color)


def footer(d, label):
    d.line((65, 1006, 1535, 1006), fill=LINE, width=2)
    text(d, 65, 1026, label, 23, MUTED, height=60)


def links(en):
    im, d = base('Two links, different connection policies' if en else '폰은 본딩하고, 키오스크는 결제 채널을 엽니다',
                 'Current architecture · testnet PoC' if en else '현재 연결 구조 · 테스트넷 PoC')
    # Screenless board illustration: exactly four LEDs and four switches.
    d.rounded_rectangle((625, 300, 975, 725), 24, fill=INK)
    text(d, 650, 325, 'NU-54V-DK', 34, '#FFFFFF', width=300)
    for i in range(4):
        x = 658 + i*79
        d.ellipse((x, 405, x+28, 433), fill='#77D6AB', outline=PALE, width=2)
        d.rounded_rectangle((x-9, 500, x+37, 548), 5, fill='#839492', outline='#FFFFFF', width=2)
    text(d, 650, 605, 'No screen\n4 LEDs / 4 buttons' if en else '화면 없음\nLED 4개 / 버튼 4개',
         27, '#FFFFFF', width=300, height=92)
    box(d, (65, 300, 520, 725), 'Renter’s phone' if en else '대여자의 폰 앱',
        'Bond after setup\nPasskey + LE Secure Connections\n\nStatus and confirmation' if en else
        '셋업 후 패스키 본딩\nLE Secure Connections\n\n기기 상태·결제 내용 확인', size=28)
    box(d, (1080, 300, 1535, 725), 'Shop kiosk' if en else '가게의 키오스크 앱',
        'No pairing\nMerchant verification\nApplication-layer encryption' if en else
        '페어링 없음\n가맹점 서명 확인\n응용 계층에서 본문 암호화', fill=GOLD, size=28)
    arrow(d, (537, 480), (608, 480), both=True)
    arrow(d, (992, 480), (1063, 480), both=True)
    d.rounded_rectangle((65, 780, 1535, 955), 22, outline=LINE, width=2)
    text(d, 95, 800, 'Different protection at each layer' if en else '연결마다 보호하는 계층이 다릅니다', 31, TEAL)
    text(d, 95, 861, 'Phone: Bluetooth link encryption   /   Kiosk: session AES-GCM' if en else
         '폰: 블루투스 링크 암호화   /   키오스크: 세션 AES-GCM', 29, width=1400, height=65)
    footer(d, 'Original diagram · current code + protocol · board drawing is schematic' if en else
           '직접 작성 · 현재 코드·규격 기준 · 보드 외형은 개략도입니다')
    return im


def handshake(en):
    im, d = base('Verify the merchant, then encrypt the session' if en else '가맹점을 확인한 뒤 결제 채널을 엽니다',
                 'Current flow · attestation validity is checked later at payment.identify' if en else
                 '현재 흐름 · 증명서 유효 기간은 이후 payment.identify에서 확인')
    text(d, 95, 270, 'KIOSK' if en else '키오스크', 31, TEAL)
    text(d, 1270, 270, 'DEVICE' if en else '결제 기기', 31, TEAL, width=265)
    d.line((180, 325, 180, 950), fill=LINE, width=3)
    d.line((1400, 325, 1400, 950), fill=LINE, width=3)
    arrow(d, (180, 410), (1400, 410))
    text(d, 255, 325, '1  session.open · plaintext' if en else '1  session.open · 평문', 31, width=1050)
    text(d, 255, 440, 'Temporary public key + attestation + merchant signature' if en else
         '임시 공개키 + 가맹점 증명서 + 가맹점 서명', 27, MUTED, width=1060, height=45)
    d.rounded_rectangle((720, 505, 1370, 625), 18, fill=PALE)
    text(d, 745, 522, 'Check operator / merchant / curve point' if en else
         '운영자·가맹점 서명과 곡선 위 점 검사', 28, TEAL, width=600, height=85)
    arrow(d, (1400, 710), (180, 710))
    text(d, 255, 640, '2  session.open.ok · plaintext' if en else '2  session.open.ok · 평문', 31, width=1060)
    text(d, 255, 735, 'Device temporary public key + device nonce' if en else
         '기기의 임시 공개키 + 기기 난수', 27, MUTED, width=1060, height=45)
    d.rounded_rectangle((260, 797, 1320, 862), 14, fill=GOLD)
    text(d, 285, 808, '3  ECDH → HKDF-SHA256 → 128-bit session key' if en else
         '3  ECDH → HKDF-SHA256 → 128비트 통신용 키', 28, width=1010, height=45)
    arrow(d, (180, 925), (1400, 925), both=True)
    text(d, 360, 944, '4  Encrypted bodies + authentication tags' if en else
         '4  본문 암호문 + 인증 태그', 28, TEAL, width=950, height=45)
    footer(d, 'Original diagram · message order is conceptual; vertical spacing is not elapsed time' if en else
           '직접 작성 · 순서를 나타낸 개념도이며 세로 간격은 시간에 비례하지 않습니다')
    return im


def protection(en):
    im, d = base('Direction and message number make the IV' if en else '방향과 메시지 번호로 IV를 구분합니다',
                 '12-byte IV · current C code uses a 64-bit counter' if en else
                 '12바이트 IV · 현재 C 코드는 64비트 카운터 사용')
    for i in range(12):
        x = 80 + i*120
        d.rounded_rectangle((x, 315, x+110, 425), 10,
                            fill=TEAL if i == 0 else GOLD if i < 4 else PALE, outline=LINE, width=2)
        text(d, x+18, 333, 'D' if i == 0 else '0' if i < 4 else 'N', 37,
             '#FFFFFF' if i == 0 else INK, width=75, height=60)
        text(d, x+8, 440, str(i), 22, MUTED, width=90, height=45)
    # Byte widths and group spans are proportional to actual bytes.
    d.line((200, 495, 1510, 495), fill=TEAL, width=3)
    text(d, 210, 508, '11-byte counter field · 3 zero bytes + 8 counter bytes in C' if en else
         '11바이트 번호 필드 · C 구현은 앞 3바이트 0 + 뒤 8바이트 번호', 27, width=1290, height=50)
    text(d, 80, 260, 'D: direction   N: counter bytes   Each cell = 1 byte' if en else
         'D: 방향   N: 카운터 바이트   칸 하나 = 1바이트', 28, MUTED)
    box(d, (65, 615, 750, 925), 'Authentication succeeds' if en else '인증 성공',
        'Expected direction + message number\nDeliver body → advance receive counter' if en else
        '기대한 방향·번호로 인증\n본문 처리 → 수신 번호 증가', size=28)
    box(d, (800, 615, 1535, 925), 'Authentication fails' if en else '인증 실패',
        'Altered / out of order / wrong direction\nEncrypted BAD_FRAME → close session' if en else
        '변조·순서 변경·다른 방향\n암호화한 BAD_FRAME → 세션 종료', fill=GOLD, size=28)
    footer(d, 'Original diagram · IV cells are proportional; process boxes are conceptual, not counts' if en else
           '직접 작성 · 바이트 칸은 비례 표시, 아래 처리 상자는 개념도이며 횟수가 아닙니다')
    return im


if __name__ == '__main__':
    for en in (False, True):
        for stem, draw in [('01-two-links', links), ('02-channel-handshake', handshake), ('03-message-protection', protection)]:
            path = OUT / f'{stem}{"-en" if en else ""}.png'
            draw(en).save(path)
            print(path.name)

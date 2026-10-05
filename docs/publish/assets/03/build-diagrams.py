"""Create original bilingual diagrams for installment 03; no image editing."""
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont
import math

OUT = Path(__file__).resolve().parent
W, H = 1600, 1000
BG, INK, MUTED = '#F7F6F1', '#182B30', '#52686D'
TEAL, PALE, GOLD, LINE, GRAY = '#087C75', '#DEF1EC', '#F4E7C4', '#B9C9C8', '#E8ECEC'
FONT = '/System/Library/Fonts/AppleSDGothicNeo.ttc'

def font(size):
    return ImageFont.truetype(FONT, size)

def text(d, xy, value, size=30, color=INK):
    d.multiline_text(xy, value, font=font(size), fill=color, spacing=13)

def canvas(title, subtitle):
    im = Image.new('RGB', (W, H), BG)
    d = ImageDraw.Draw(im)
    text(d, (65, 45), 'NU-54V-DK / 03', 24, TEAL)
    text(d, (65, 95), title, 46)
    text(d, (65, 165), subtitle, 27, MUTED)
    return im, d

def box(d, rect, title, body='', fill=PALE, title_size=32, body_size=28):
    d.rounded_rectangle(rect, 22, fill=fill, outline=LINE, width=2)
    x, y, _, _ = rect
    text(d, (x+25, y+23), title, title_size, TEAL)
    text(d, (x+25, y+90), body, body_size)

def arrow(d, p1, p2):
    d.line([p1, p2], fill=TEAL, width=5)
    a = math.atan2(p2[1]-p1[1], p2[0]-p1[0])
    d.polygon([p2, (p2[0]-17*math.cos(a-.5), p2[1]-17*math.sin(a-.5)),
               (p2[0]-17*math.cos(a+.5), p2[1]-17*math.sin(a+.5))], fill=TEAL)

def footer(d, value):
    d.line([(65, 915), (1535, 915)], fill=LINE, width=2)
    text(d, (65, 935), value, 23, MUTED)

for en in (False, True):
    suffix = '-en' if en else ''
    im, d = canvas('A signature is the start, not payment completion' if en else '서명은 시작입니다. 정산 완료까지 확인합니다',
                   'Software-signed payment / testnet proof of concept' if en else '소프트웨어 서명 결제 / 테스트넷 PoC')
    labels = [('Sign', 'Payment authorization'), ('Submit', 'Kiosk transaction'), ('Settle', 'Checks + ledger update'), ('Confirm', 'Event + finalized block')] if en else [('서명', '결제 승인 메시지'), ('제출', '키오스크 트랜잭션'), ('정산', '검사와 장부 변경'), ('확인', '이벤트와 확정 블록')]
    for n, (title, body) in enumerate(labels):
        x = 65+n*380
        d.ellipse((x+95, 300, x+195, 400), fill=TEAL)
        text(d, (x+131, 320), str(n+1), 42, 'white')
        box(d, (x, 445, x+330, 645), title, body, GOLD if n==3 else PALE, body_size=25)
        if n<3:
            arrow(d, (x+330, 545), (x+375, 545))
    text(d, (65, 730), '4.5 tUSDC', 63, TEAL)
    text(d, (65, 810), 'Verified October 1, 2026' if en else '2026년 10월 1일 확인', 30)
    footer(d, 'Conceptual diagram / no private keys or real addresses' if en else '직접 작성한 개념도 / 개인 키와 실제 주소는 표시하지 않습니다')
    im.save(OUT/f'01-hero{suffix}.png')

    im, d = canvas('Two signatures, one settlement flow' if en else '결제 승인 서명과 제출 트랜잭션은 다릅니다',
                   'This test used a software key, not the board button' if en else '이번 시험은 소프트웨어 키를 사용했습니다. 보드 버튼 시험이 아닙니다')
    box(d, (65, 270, 505, 530), 'Software key' if en else '소프트웨어 키',
        'Sign payment authorization\nEIP-712 structured message' if en else '결제 승인 메시지 서명\nEIP-712 구조화된 메시지', body_size=26)
    box(d, (585, 270, 1025, 530), 'Kiosk submission' if en else '키오스크 제출',
        'Sign the transaction\nSubmit / pay gas' if en else '제출 트랜잭션 서명\n전송 / 가스 부담', GRAY)
    box(d, (1105, 270, 1535, 530), 'Settlement contract' if en else '정산 컨트랙트',
        'Validate request\nCheck balance and limits' if en else '결제 요청 검사\n잔액과 한도 확인', body_size=26)
    arrow(d, (505, 400), (585, 400)); arrow(d, (1025, 400), (1105, 400))
    arrow(d, (1320, 530), (1320, 645))
    box(d, (865, 645, 1535, 860), 'Internal ledger update' if en else '컨트랙트 내부 장부 변경',
        'Device balance down / merchant balance up\nEmit PaymentSettled' if en else '기기 잔액 감소 / 가맹점 잔액 증가\nPaymentSettled 이벤트 기록', GOLD, body_size=26)
    text(d, (65, 680), 'Withdrawal is separate.' if en else '토큰 출금은 별도 흐름입니다.', 31)
    text(d, (65, 745), 'No external-wallet transfer per payment' if en else '결제마다 외부 지갑에 보내지 않습니다.', 25, MUTED)
    footer(d, 'Created from the project design and October 1 verification record' if en else '직접 작성 / 프로젝트 설계와 10월 1일 검증 기록 기준')
    im.save(OUT/f'02-settlement-flow{suffix}.png')

    im, d = canvas('Balances after the verified payment' if en else '정산 뒤 기록한 잔액을 확인합니다',
                   'October 1 record / token base units' if en else '10월 1일 기록 / 토큰의 기본 단위')
    box(d, (65, 250, 780, 535), 'Device account' if en else '기기 계정', '45,500,000', body_size=66)
    box(d, (820, 250, 1535, 535), 'Merchant balance' if en else '가맹점 잔액', '4,500,000', GOLD, body_size=66)
    text(d, (65, 580), 'Payment: 4,500,000 = 4.5 tUSDC' if en else '결제 금액: 4,500,000 = 4.5 tUSDC', 35)
    box(d, (65, 675, 780, 865), 'totalOwed', '50,000,000', GRAY, body_size=45)
    box(d, (820, 675, 1535, 865), 'surplus', '0', GRAY, body_size=45)
    footer(d, 'Recorded post-settlement values / internal ledger, not a withdrawal' if en else '직접 작성 / 실측 정산 후 값 / 내부 장부이며 출금 결과가 아닙니다')
    im.save(OUT/f'03-settlement-balances{suffix}.png')

    im, d = canvas('Three checks before calling the payment complete' if en else '결제 완료를 말하기 전에 세 가지를 확인합니다',
                   'Receipt + matching event + finalized block' if en else '실행 영수증 + 결제 이벤트 일치 + 확정 블록')
    box(d, (65, 250, 510, 590), '1. Execution' if en else '1. 실행 성공',
        'Receipt status: 1\nGas used: 120,888\nBlock: 21129166' if en else '실행 영수증 status: 1\n사용 가스: 120,888\n결제 블록: 21129166', body_size=26)
    box(d, (580, 250, 1025, 590), '2. Payment event' if en else '2. 결제 내용 일치',
        'PaymentSettled\nMerchant / order / device\nmatch signed values\nAmount: 4,500,000' if en else 'PaymentSettled\n가맹점 / 주문 / 기기\n서명 값과 일치\n금액: 4,500,000', body_size=26)
    box(d, (1095, 250, 1535, 590), '3. Finalized block' if en else '3. 확정 블록 확인',
        '21129178 ≥ 21129166\nPayment-block hash\nmatches receipt blockHash' if en else '21129178 ≥ 21129166\n결제 블록 해시와\n영수증 blockHash 일치', GOLD, body_size=25)
    arrow(d, (510, 420), (580, 420)); arrow(d, (1025, 420), (1095, 420))
    box(d, (65, 670, 1535, 865), 'Result' if en else '판정',
        'Settlement criteria met / not an independent consensus audit' if en else '정산 확인 조건 통과 / 합의 알고리즘 자체의 독립 검증은 아닙니다', GRAY)
    footer(d, 'October 1 observation / addresses and block hashes omitted' if en else '직접 작성 / 10월 1일 관찰값 / 실제 주소와 블록 해시는 생략했습니다')
    im.save(OUT/f'04-finality-checks{suffix}.png')

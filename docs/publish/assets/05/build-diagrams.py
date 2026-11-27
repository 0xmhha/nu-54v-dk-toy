"""Original bilingual diagrams for article 05; no measured security score is implied."""
from pathlib import Path
import math
from PIL import Image, ImageDraw, ImageFont

OUT = Path(__file__).resolve().parent
W, H = 1600, 1100
BG, INK, MUTED = '#F7F6F1', '#182B30', '#52686D'
TEAL, PALE, GOLD, LINE = '#087C75', '#DEF1EC', '#F4E7C4', '#B9C9C8'
FONT = '/System/Library/Fonts/AppleSDGothicNeo.ttc'


def font(size):
    return ImageFont.truetype(FONT, size)


def label(d, xy, value, size=30, color=INK):
    d.multiline_text(xy, value, font=font(size), fill=color, spacing=10)


def wrap(d, xy, value, width, size=30, color=INK, height=180):
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
    value = '\n'.join(lines)
    bounds = d.multiline_textbbox(xy, value, font=font(size), spacing=10)
    assert bounds[2] <= xy[0] + width + 2, value
    assert bounds[3] <= xy[1] + height, value
    label(d, xy, value, size, color)


def base(title, sub):
    im = Image.new('RGB', (W, H), BG)
    d = ImageDraw.Draw(im)
    label(d, (65, 38), 'NU-54V-DK / 05', 24, TEAL)
    wrap(d, (65, 93), title, 1470, 46, height=70)
    wrap(d, (65, 168), sub, 1470, 28, MUTED, height=75)
    return im, d


def box(d, rect, title, body, fill=PALE, title_size=32, body_size=28):
    x, y, x2, y2 = rect
    d.rounded_rectangle(rect, radius=22, fill=fill, outline=LINE, width=2)
    wrap(d, (x+25, y+22), title, x2-x-50, title_size, TEAL, height=95)
    wrap(d, (x+25, y+115), body, x2-x-50, body_size, height=y2-y-133)


def arrow(d, p1, p2):
    d.line([p1, p2], fill=TEAL, width=5)
    a = math.atan2(p2[1]-p1[1], p2[0]-p1[0])
    d.polygon([p2, (p2[0]-17*math.cos(a-.5), p2[1]-17*math.sin(a-.5)),
               (p2[0]-17*math.cos(a+.5), p2[1]-17*math.sin(a+.5))], fill=TEAL)


def footer(d, value):
    d.line((65, 1006, 1535, 1006), fill=LINE, width=2)
    wrap(d, (65, 1026), value, 1470, 23, MUTED, height=60)


def boundary(en):
    im, d = base('The boundary we still need to build' if en else '보안 구역으로 옮겨야 할 경계',
                 'Current code and planned architecture · testnet PoC' if en else '현재 코드와 구현할 구조 · 테스트넷 PoC')
    label(d, (70, 265), 'CURRENT · application-managed approval' if en else '현재 코드 · 앱이 승인 절차를 관리', 30)
    rows = [('Button event', 'main.c → work queue'), ('Session checks', 'Pending payment → sign callback'),
            ('PSA Crypto', 'CRACEN + Zephyr Secure storage')] if en else [
                ('버튼 입력', 'main.c → 작업 큐'), ('세션 확인', '결제 승인 대기 → 서명 콜백'),
                ('PSA Crypto', 'CRACEN + Zephyr Secure storage')]
    for i, (t, b) in enumerate(rows):
        x = 70 + i*505
        box(d, (x, 320, x+450, 515), t, b, body_size=27)
        if i < 2:
            arrow(d, (x+457, 417), (x+492, 417))
    label(d, (70, 555), 'PLANNED · not implemented in this checkout' if en else '목표 설계 · 현재 체크아웃에는 미구현', 30)
    box(d, (70, 615, 510, 930), 'Application' if en else '일반 앱',
        'Message handling\nPhone confirmation fields\nRegister digest + purpose' if en else
        '메시지 처리\n폰 확인용 필드\n요약값·목적 등록', fill='#E8ECEC')
    arrow(d, (525, 770), (585, 770))
    d.rounded_rectangle((605, 615, 1530, 930), 24, outline=TEAL, width=4)
    label(d, (632, 638), 'Secure world · TF-M service' if en else '보안 구역 · TF-M 키 서비스', 31, TEAL)
    box(d, (630, 702, 1055, 903), 'Button approval' if en else '버튼 승인',
        'Secure interrupt\nOne request / one use' if en else '보안 구역 인터럽트\n한 요청에 한 번 사용', body_size=27)
    box(d, (1080, 702, 1505, 903), 'Key operations' if en else '키 처리',
        'Check approval → sign\nProtected state' if en else '승인 검사 → 서명\n보호할 상태 저장', body_size=27)
    footer(d, 'Original diagram · source code + design · the lower row is a target, not a test result' if en else
           '직접 작성 · 코드와 설계 기준 · 아래쪽은 목표이며 실기 시험 결과가 아닙니다')
    return im


def approval(en):
    im, d = base('One approval belongs to one request' if en else '버튼 한 번의 승인을 요청 한 건에 묶기',
                 'Target rule for the secure service · not yet implemented or tested' if en else
                 '보안 키 서비스의 목표 규칙 · 아직 구현·시험하지 않았습니다')
    items = [('1 · Register', 'digest + purpose\nOne pending request'),
             ('2 · Approve', 'Secure button interrupt\nIssue a one-use token'),
             ('3 · Sign and clear', 'Match request + token\nConsume approval')] if en else [
                 ('1 · 등록', '요약값 + 목적\n대기 요청 한 건'), ('2 · 승인', '보안 구역의 버튼 인터럽트\n일회용 토큰 발급'),
                 ('3 · 서명과 폐기', '요청·토큰 일치 확인\n사용한 승인 정보 삭제')]
    for i, (t, b) in enumerate(items):
        x = 70 + i*505
        box(d, (x, 310, x+450, 610), t, b)
        if i < 2:
            arrow(d, (x+458, 460), (x+492, 460))
    d.rounded_rectangle((70, 670, 1530, 950), 22, fill=GOLD)
    label(d, (100, 697), 'REFUSE' if en else '거절해야 할 요청', 31, TEAL)
    for i, v in enumerate(['Different digest', 'Different purpose', 'Reused token'] if en else
                          ['다른 요약값', '다른 목적', '이미 사용한 토큰']):
        x = 105+i*475
        d.ellipse((x, 768, x+52, 820), outline=TEAL, width=3)
        d.line((x+12, 780, x+40, 808), fill=TEAL, width=3)
        d.line((x+40, 780, x+12, 808), fill=TEAL, width=3)
        wrap(d, (x+72, 779), v, 355, 28, height=70)
    wrap(d, (105, 870), 'Cancel / disconnect / timeout → clear pending approval' if en else
         '취소 · 연결 해제 · 시간 초과 → 대기 중인 승인 정보 삭제', 1380, 29, height=60)
    footer(d, 'Original diagram · design sequence only · secure-partition refusal tests remain' if en else
           '직접 작성 · 설계 순서를 표현한 도식 · 보안 파티션의 거절 시험은 남아 있습니다')
    return im


def verification(en):
    im, d = base('What the host tests actually cover' if en else '호스트 시험으로 확인한 범위',
                 'Actual run · October 19, 2026 · no TF-M hardware test' if en else
                 '2026년 10월 19일 실제 실행 · TF-M 실기 시험은 미실행')
    label(d, (70, 265), '8 test groups passed' if en else '시험 8개 묶음 통과', 34)
    labels = [('Frames', 'frame'), ('Buttons', 'button'), ('PIN entry', 'pin_entry'), ('Link routing', 'links'),
              ('EIP-712', '9 vectors'), ('Frame vectors', 'frame_vectors'), ('CBOR', 'cbor_vectors'),
              ('Sessions', '38 scenarios')] if en else [
                  ('메시지 조각', 'frame'), ('버튼', 'button'), ('PIN 입력', 'pin_entry'), ('링크 판정', 'links'),
                  ('EIP-712', '벡터 9개'), ('조각 벡터', 'frame_vectors'), ('CBOR', 'cbor_vectors'),
                  ('세션', '시나리오 38개')]
    for i, (t, b) in enumerate(labels):
        x, y = 70+(i%4)*370, 335+(i//4)*225
        d.rounded_rectangle((x, y, x+345, y+195), 20, fill=PALE, outline=LINE, width=2)
        d.ellipse((x+20, y+20, x+63, y+63), fill=TEAL)
        d.line((x+31, y+42, x+40, y+51, x+54, y+33), fill=BG, width=4)
        wrap(d, (x+22, y+83), t, 303, 29, height=45)
        wrap(d, (x+22, y+137), b, 303, 25, MUTED, height=45)
    d.rounded_rectangle((70, 825, 1530, 955), 20, fill=GOLD)
    label(d, (95, 845), 'Still unverified on the board' if en else '보드에서 아직 검증하지 않은 것', 29, TEAL)
    wrap(d, (95, 895), 'TF-M key isolation · secure GPIO approval · NVM inspection' if en else
         'TF-M 키 격리 · 보안 구역 GPIO 승인 · 비휘발성 메모리 검사', 1410, 28, height=50)
    footer(d, 'Original infographic · 9 vectors and 38 scenarios are included in the 8 groups, not extra groups' if en else
           '직접 작성 · 벡터 9개와 시나리오 38개는 위 8개 묶음에 포함되며 별도 묶음이 아닙니다')
    return im


if __name__ == '__main__':
    for en in (False, True):
        suffix = '-en' if en else ''
        for name, draw in [('01-security-boundary', boundary), ('02-approval-binding', approval),
                           ('03-verification-scope', verification)]:
            path = OUT / (name+suffix+'.png')
            draw(en).save(path)
            print(path.name)

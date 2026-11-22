"""Bilingual article 07 diagrams; reuse the established series drawing helpers."""
from pathlib import Path
import importlib.util
from PIL import Image, ImageDraw

OUT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('series_style', OUT.parent / '06' / 'build-diagrams.py')
style = importlib.util.module_from_spec(spec)
spec.loader.exec_module(style)
text, box, arrow, footer = style.text, style.box, style.arrow, style.footer
BG, INK, MUTED = style.BG, style.INK, style.MUTED
TEAL, PALE, GOLD, LINE = style.TEAL, style.PALE, style.GOLD, style.LINE


def base(title, sub):
    im = Image.new('RGB', (1600, 1100), BG)
    d = ImageDraw.Draw(im)
    text(d, 65, 38, 'NU-54V-DK / 07', 24, TEAL)
    text(d, 65, 92, title, 43, height=70)
    text(d, 65, 170, sub, 28, MUTED, height=75)
    return im, d


def lifecycle(en):
    im, d = base('Close the account before resetting the device' if en else '계정을 먼저 닫고 기기를 초기화합니다',
                 'Current code flow · integrated hardware return and payout remain to be tested' if en else
                 '현재 코드 흐름 · 실기 반납부터 지급까지의 통합 시험은 남아 있습니다')
    text(d, 70, 265, 'ACCOUNT AND DEVICE' if en else '계정과 기기 정리', 29, TEAL)
    steps = [('1 · Close account', 'closeAccount\nFurther settlement blocked'),
             ('2 · Confirm finality', 'AccountClosed\nCheck finalized block range'),
             ('3 · Reset device', 'Signed device.reset\nCheck acceptance')] if en else [
                 ('1 · 계정 종료', 'closeAccount\n이후 결제 정산 차단'),
                 ('2 · 확정 확인', 'AccountClosed\n확정 블록 범위에서 확인'),
                 ('3 · 기기 초기화', '서명된 device.reset\n기기 수락 여부 확인')]
    for i, (title, body) in enumerate(steps):
        x = 65 + i*505
        box(d, (x, 320, x+450, 595), title, body, size=27)
        if i < 2:
            arrow(d, (x+460, 450), (x+495, 450))
    arrow(d, (275, 610), (275, 700))
    text(d, 320, 633, 'The payout clock starts at closure execution' if en else
         '잔액 지급의 지연은 계정 종료 실행 시각부터 계산', 27, MUTED, width=1160)
    box(d, (65, 715, 750, 955), 'WAIT FOR PAYOUT' if en else '잔액 지급 대기',
        'Closure timestamp + 3,600 seconds' if en else '계정 종료 블록 시각 + 3,600초', fill=GOLD, size=28)
    arrow(d, (760, 830), (790, 830))
    box(d, (805, 715, 1535, 955), 'EXECUTE WITHDRAWAL' if en else '별도로 출금 실행',
        'Call executeWithdrawal\nPay only the registered return address' if en else
        'executeWithdrawal 호출\n등록된 반환 주소로만 지급', fill=GOLD, size=28)
    footer(d, 'Original diagram · code + deployment configuration · arrows indicate dependencies, not measured time' if en else
           '직접 작성 · 코드·배포 설정 기준 · 화살표는 의존 관계이며 실제 소요 시간이 아닙니다')
    return im


def timeline(en):
    im, d = base('The same delay, different account states' if en else '같은 출금 지연이어도 계정 상태는 다릅니다',
                 'Each row starts when its function executes · configured delay: 3,600 seconds' if en else
                 '각 행의 시작은 해당 함수 실행 시각 · 설정된 출금 지연: 3,600초')
    x0, x1 = 345, 1490
    y = 320
    d.line((x0, y, x1, y), fill=TEAL, width=4)
    for i, sec in enumerate([0, 900, 1800, 2700, 3600]):
        x = x0 + (x1-x0)*sec/3600
        d.line((x, y-12, x, y+15), fill=TEAL, width=3)
        text(d, x-38, y-65, f'{sec:,}', 25, width=105, height=45)
    text(d, 70, 270, 'Seconds' if en else '초', 27, MUTED, width=200)
    rows = [(410, 'WITHDRAWAL\nREQUEST' if en else '일반 출금 요청',
             'Valid payments may still settle' if en else '유효한 결제는 계속 정산될 수 있습니다', PALE),
            (600, 'ACCOUNT\nCLOSURE' if en else '계정 종료',
             'Further settlement is blocked' if en else '이후 결제 정산을 막습니다', '#F1DDCF')]
    for top, label, body, fill in rows:
        text(d, 70, top+35, label, 29, TEAL, width=245, height=100)
        d.rounded_rectangle((x0, top, x1, top+135), 14, fill=fill, outline=LINE, width=2)
        text(d, x0+25, top+45, body, 31, width=x1-x0-50, height=70)
    d.rounded_rectangle((65, 815, 1535, 955), 20, fill=GOLD)
    text(d, 95, 834, 'After the delay: someone must call executeWithdrawal' if en else
         '지연이 끝나도 executeWithdrawal을 호출해야 지급됩니다', 31, width=1390, height=60)
    text(d, 95, 897, 'Time passing alone does not transfer tokens' if en else
         '시간이 지나는 것만으로 토큰이 송금되지는 않습니다', 27, MUTED, width=1390, height=45)
    footer(d, 'Original diagram · proportional configured time axis · eligibility is not a measured payout completion time' if en else
           '직접 작성 · 설정 시간에 비례한 축 · 출금 가능 시점을 실제 지급 완료 시간으로 표시하지 않았습니다')
    return im


def recovery(en):
    im, d = base('Resume from the step that remains unfinished' if en else '반납이 멈춘 위치에 따라 이어서 처리합니다',
                 'Current code + tests · lost reset acknowledgment still needs reconciliation' if en else
                 '현재 코드·시험 기준 · 초기화 응답 유실은 별도 확인 절차가 필요합니다')
    rows = [('Closure failed / not final', 'No device reset is sent', PALE),
            ('Closure final / BLE unavailable', 'Reset=false; ResetPending\nBring the device close and rerun', PALE),
            ('Connected to another device', 'Do not send the reset command', PALE),
            ('Reset acknowledgment missing', 'No automatic completion check\nInspect the device state separately', GOLD)] if en else [
                ('계정 종료 실패 / 확정 미확인', '기기 초기화 명령을 보내지 않습니다', PALE),
                ('종료 확정 / BLE 연결 불가', 'Reset=false, ResetPending\n기기를 가까이 두고 재실행합니다', PALE),
                ('다른 기기에 연결', '초기화 명령을 보내지 않습니다', PALE),
                ('초기화 응답 유실', '자동 완료 판정이 없습니다\n기기 상태를 따로 확인해야 합니다', GOLD)]
    for i, (cause, action, fill) in enumerate(rows):
        y = 280 + i*160
        d.rounded_rectangle((65, y, 665, y+130), 16, fill='#E8ECEC', outline=LINE, width=2)
        text(d, 90, y+35, cause, 29, width=550, height=85)
        arrow(d, (680, y+65), (730, y+65))
        d.rounded_rectangle((745, y, 1535, y+130), 16, fill=fill, outline=LINE, width=2)
        text(d, 770, y+22, action, 28, width=740, height=100)
    text(d, 75, 942, 'An already closed account skips the closure transaction on rerun' if en else
         '재실행할 때 이미 닫힌 계정은 종료 트랜잭션을 건너뜁니다', 28, TEAL, width=1450, height=48)
    footer(d, 'Original diagram · code + existing tests · these are recovery states, not measured failure rates' if en else
           '직접 작성 · 코드·기존 시험 기준 · 실패율이 아니라 중단 위치별 처리 흐름입니다')
    return im


if __name__ == '__main__':
    for en in (False, True):
        for stem, draw in [('01-return-lifecycle', lifecycle), ('02-withdrawal-timeline', timeline), ('03-return-recovery', recovery)]:
            path = OUT / f'{stem}{"-en" if en else ""}.png'
            draw(en).save(path)
            print(path.name)

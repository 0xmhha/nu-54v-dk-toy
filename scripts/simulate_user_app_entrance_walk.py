"""Taps through the route of the user app that needs no registration at a store. Run by simulate_user_app.py --walk --route entrance.

The store finder, a store page, its menu, the checkout with the board's account proof, the payment, the order and its
pickup QR, and the questions before a board reset. Each flow keeps a screenshot of every screen and state it reaches,
and beside it the words of that screen in a text file of the same name.
A flow checks what the app's code says it shows; where the app does something else the flow fails and says what.

The walk has two parts, each in a world of its own because one world lives 43 minutes: `stores` needs no board, and
`orders` starts with one.
"""

import html
import re
import time

from simulate_user_app_walk import PACKAGE, TABS, Walk

# The stores of a run with a customer entrance, as the fixture names them (order_flow_anvil_test.go seedFinder).
CAFE, HANOK, SEOCHON, NIGHT = "Order flow cafe", "Hanok tea house 한옥 찻집", "Seochon bakery 서촌 빵집", "Night bar 심야 술집"
BUSY, GARDEN, ALLEY = "Busy noodle 바쁜 국수", "Garden brunch 가든 브런치", "Back alley 골목 가게"
STORES = (CAFE, HANOK, SEOCHON, NIGHT, BUSY, GARDEN, ALLEY)
# What each sells, in won and in cents.
PRICES = {"아메리카노": (4_500, 450), "치즈케이크": (6_500, 650), "스콘": (3_800, 380)}
PARTS = {
    "stores": ("agree", "home_before_board", "store_finder", "store_pages", "shortcut_qr", "finder_problems", "menu_without_board"),
    "orders": ("agree", "set_up", "balances_in_this_build", "reset_without_orders", "shortfall", "checkout", "checkout_problems", "pay", "order_status",
               "pickup", "order_end", "reset_with_open_order", "former_board"),
}


class EntranceWalk(Walk):
    NEEDED = ("agree", "set_up")

    def __init__(self, adb, step, evidence, token="usd", mark="", part="stores"):
        super().__init__(adb, step, evidence, token, mark)
        self.part = part
        self.sale = ""  # the order the phone paid last
        self.wordless = False

    def flows(self):
        return tuple(getattr(self, name) for name in PARTS[self.part])

    # ---- amounts ----
    def price(self, won, cents):
        """An amount as a screen of this store shows it: whole won, or dollars to the cent."""
        return f"{won:,}원" if self.won else f"{cents // 100}.{cents % 100:02d} TestUSDC"

    def of(self, *products):
        """The sum of products, each named once for every one in the basket."""
        return self.price(sum(PRICES[name][0] for name in products), sum(PRICES[name][1] for name in products))

    # ---- the screen ----
    def shown(self, text):
        return any(text in word for word in self.words())

    def top(self):
        for _ in range(8):
            self.swipe(False)

    def page(self):
        """Every word of the page in front, read from its top to its end."""
        self.top()
        seen, last = [], None
        for _ in range(30):
            now = self.words()
            if now == last:
                break
            seen += [word for word in now if word not in seen]
            last = now
            self.swipe()
        self.top()
        return seen

    def shot(self, name, words=True):
        """The screenshot and, beside it, the words on that screen as the phone reads them: the wording without the picture."""
        super().shot(name)
        if not words or self.wordless:
            return
        self.shell("rm", "-f", "/sdcard/nu54-ui.xml")
        self.shell("uiautomator", "dump", "/sdcard/nu54-ui.xml")  # once: a screen that does not rest is left without words
        dumped = self.adb("exec-out", "cat", "/sdcard/nu54-ui.xml").stdout.decode(errors="replace")
        seen = []
        for node in re.findall(r"<node [^>]*>", dumped):
            for kind in ("text", "content-desc"):
                found = re.search(rf' {kind}="([^"]*)"', node)
                word = html.unescape(found[1]) if found else ""
                if word and word not in TABS and word not in seen[-2:]:
                    seen.append(word)
        if seen:
            (self.evidence / f"{name}.txt").write_text("\n".join(seen) + "\n")

    def pressed_late(self, label, name):
        self.wordless = True  # the board waits for its button: the screen is not read meanwhile
        try:
            super().pressed_late(label, name)
        finally:
            self.wordless = False

    def tapped_then_shot(self, label, name, after=0.4):
        """What the screen shows right after a button: nothing is read first, so a state that passes is still caught."""
        node = self.find(label)
        assert node and node["enabled"], f"no button {label}"
        left, upper, right, lower = node["box"]
        self.shell("input", "tap", str((left + right) // 2), str((upper + lower) // 2))
        time.sleep(after)
        self.shot(name, words=False)
        time.sleep(1.2)

    def long_press(self, text):
        node = next(node for node in self.nodes() if text in node["words"])
        left, upper, right, lower = node["box"]
        x, y = str((left + right) // 2), str((upper + lower) // 2)
        self.shell("input", "swipe", x, y, x, y, "900")
        time.sleep(1.5)

    def in_front(self):
        """Whether the app is the one in front, and not another that a link opened."""
        focus = [line for line in self.shell("dumpsys", "window").splitlines() if b"mCurrentFocus" in line]
        return any(PACKAGE.encode() in line for line in focus)

    def come_back(self):
        """Leaves the app a link opened, the way its owner would: Back, until this app is in front again."""
        for _ in range(4):
            if self.in_front():
                return
            self.back()
        self.shell("am", "start", "-n", PACKAGE + "/.MainActivity")
        time.sleep(2)
        assert self.in_front(), "the app did not come back to the front"

    # ---- what the phone sent to the entrance ----
    def entrance(self, **change):
        """The last requests, after setting what the phone can reach."""
        return self.step("entrance", change=change)["sent"]

    def answered(self, kind):
        """How many requests of one kind got one answer so far: 'proof 200', 'order.status 404', 'stores 429'."""
        return self.step("entrance", change={})["answered"].get(kind, 0)

    def last_wait(self):
        """The Retry-After of the last request the server refused for its rate."""
        refused = [call for call in self.entrance() if call.get("status") == 429]
        assert refused and refused[-1].get("retryAfter", 0) >= 1, f"no Retry-After reached the phone: {refused}"
        return refused[-1]["retryAfter"]

    # ---- the ways through the app ----
    def finder(self):
        self.tap("둘러보기")
        self.tap("매장 찾기")
        self.sees("모든 매장")

    def on_store_page(self, name=None, seconds=40, menu=True):
        """Waits for a store page: its order button is there, and the finder's search field is not. A store with no
        menu on sale has no such button (`menu` false): its page is known by the reason it gives."""
        deadline = time.monotonic() + seconds
        while True:
            seen = self.words()
            there = "메뉴 보고 주문" in seen or "메뉴만 보기" in seen if menu else any("주문을 받지 않습니다" in word for word in seen)
            if "매장 이름" not in seen and there and (name is None or name in seen):
                return
            assert time.monotonic() < deadline, f"no store page: {' | '.join(seen)[:600]}"
            time.sleep(1)

    def store_page(self, name, menu=True):
        """The store page, found by name the way a customer finds it."""
        self.finder()
        self.type("매장 이름", name.split()[0])
        self.tap(name)
        self.on_store_page(name, menu=menu)

    def menu(self, name=CAFE, button="메뉴 보고 주문", empty=False):
        """The menu of a store. The app keeps what a customer left in a basket while they are away from the store, so
        a flow that starts anew at a store where the flow before left something first takes that out (`empty`)."""
        self.store_page(name)
        self.tap(button)
        self.sees(f"{name} 메뉴")
        if empty:
            self.empty_basket()

    def empty_basket(self):
        """Takes out every product of the basket, from the top of the menu to its end, and comes back to the top."""
        last = None
        for _ in range(40):
            nodes = self.nodes()
            out = next((node for node in nodes if node["words"][1].endswith(" 빼기") and node["enabled"]
                        and 60 < (node["box"][1] + node["box"][3]) // 2 < self.height * 88 // 100), None)
            if out:
                self.touch(out)
                continue
            seen = [(node["words"], node["box"]) for node in nodes]
            if seen == last:
                break
            last = seen
            self.swipe()
        self.top()

    def add(self, product, count=1):
        for _ in range(count):
            self.tap(f"{product} 추가")

    def ask_for_order(self):
        """The basket's button is below the menu and what it says is drawn at the top of the page: the page brings
        the answer into view itself, so nothing here scrolls to it."""
        self.tap("주문 접수 요청")
        time.sleep(2)

    def held(self):
        self.sees("주문이 연결되었습니다", 30)

    def review(self):
        self.tap("이 주문의 결제 내용 검토")
        self.sees("결제 내용을 확인하세요", 30)

    def approve(self, checked, waiting):
        self.tap("내 보드 확인")
        self.sees("오늘 남은 한도")
        self.shot(checked)
        self.pressed_late("보드로 승인", waiting)
        self.sees("주문이 접수되었습니다", 60)

    def newest_order(self):
        rows = self.step("orders")
        assert rows, "the kitchen has no order"
        return rows[-1]["saleRef"]

    def open_order(self):
        """The order's own page, from the home tab."""
        self.tap("홈")
        deadline = time.monotonic() + 40
        while True:
            node = next((node for node in self.nodes() if node["words"][1].startswith(f"{CAFE} 주문번호")), None)
            if node:
                break
            assert time.monotonic() < deadline, "the home tab shows no order"
            time.sleep(2)
        self.touch(node)
        self.sees("주문 단계")

    def ask_again(self):
        """Presses "상태 다시 확인" when it is on the screen. An order that just ended no longer has the button, so
        one that went away between two looks at the screen is no failure."""
        node = next((node for node in self.nodes() if "상태 다시 확인" in node["words"] and node["tap"] and node["enabled"]
                     and 60 < (node["box"][1] + node["box"][3]) // 2 < self.height * 88 // 100), None)
        if node:
            self.touch(node)

    def refresh(self, text, seconds=60):
        """The order page asks again by itself; the customer can ask too."""
        deadline = time.monotonic() + seconds
        while not self.shown(text):
            assert time.monotonic() < deadline, f"not on the screen: {text}: {' | '.join(self.words())[:600]}"
            self.ask_again()
            time.sleep(3)

    def reset_page(self):
        self.tap("설정")
        self.tap("내 보드")
        self.tap("공장 초기화")
        self.sees("앱 없이 버튼으로 초기화", 60)
        self.top()

    # ---- the flows of `stores` ----
    def home_before_board(self):
        """A build of this route asks for no store registration: four steps, and no fifth."""
        self.tap("홈")
        self.sees("보드 연결 시작")
        steps = [word for word in self.words() if word in ("보드 연결과 PIN 설정", "서명 시험", "반납받을 주소 등록", "매장 등록", "원래 지갑에서 입금")]
        assert steps == ["보드 연결과 PIN 설정", "서명 시험", "반납받을 주소 등록", "원래 지갑에서 입금"], steps
        self.shot("001-home-before-board")
        self.tap("둘러보기")
        self.sees("이름으로 매장을 찾아 매장 화면을 보고 미리 주문합니다")
        self.shot("002-explore")

    def store_finder(self):
        self.finder()
        self.sees("더 있습니다")
        seen = self.words()
        assert "매장 3곳을 불러왔습니다. 더 있습니다." in seen and "휴무" in seen and CAFE not in seen, seen
        assert not any("최근 매장" in word for word in seen), "a phone that opened no store has recent stores"
        self.shot("003-finder-first-page")
        self.tap("매장 더 보기")
        self.sees(SEOCHON)
        seen = self.page()
        for note in ("바쁨", "오늘 쉽니다", "판매 중인 메뉴 없음", "주문 잠시 멈춤"):
            assert any(word.endswith(" · " + note) for word in seen), f"no row says {note}: {seen}"
        assert ALLEY not in seen, "a store that is not listed is in the list"
        assert "매장 더 보기" not in seen and not any("더 있습니다" in word for word in seen), "the whole list still says that more follows"
        self.shot("004-finder-all-stores")
        self.swipe()
        self.shot("005-finder-all-stores-lower")
        self.top()
        self.type("매장 이름", "order")
        self.sees(CAFE)
        assert BUSY not in self.words(), "the search keeps a store with another name"
        self.shot("006-finder-search-hit")
        self.clear("매장 이름")
        self.type("매장 이름", "zzz")
        self.sees("이 이름의 매장을 찾지 못했습니다.")
        self.shot("007-finder-search-no-hit")
        # The search reads the pages that follow by itself: a store of the second page is found from the first.
        self.tap("홈")
        self.finder()
        self.sees("더 있습니다")
        self.type("매장 이름", "seochon")
        self.sees(SEOCHON)
        self.shot("008-finder-search-reads-next-page")

    def store_pages(self):
        self.store_page(CAFE)
        seen = self.words()
        assert "메뉴 보고 주문" in seen and "02-555-0142" in seen and "직접 볶은 원두로 내리는 동네 카페입니다." in seen, seen
        self.shot("009-store-open")
        seen = self.page()
        for text in ("평일", "08:00~22:00", "토·일", "매달 첫째 월요일", "보드 결제 · " + self.label, "운영자 NU54 운영팀 help@nu54.example",
                     "주소를 길게 누르면 복사할 수 있습니다.", "매장 주소가 지도 앱으로 갑니다. 내 위치는 보내지 않습니다."):
            assert text in seen, f"the store page does not say {text}: {seen}"
        self.swipe()
        self.shot("010-store-open-hours")
        self.swipe()
        self.shot("011-store-open-payment-and-contact")
        # The address is text that can be selected, and the map row hands the address to a map app.
        self.top()
        self.find("지도 앱으로 열기")
        self.long_press("서울 종로구 자하문로 12")
        self.shot("012-store-address-long-press")
        self.back()
        self.tap("지도 앱으로 열기")
        time.sleep(4)
        left = not self.in_front()
        self.shot("013-store-map-row-pressed")
        self.come_back()
        if not left:  # a phone without a map app: the page says so, below everything else
            assert "지도 앱을 열지 못했습니다. 주소를 길게 눌러 복사하세요." in self.page(), "the map row did nothing"
        self.top()
        self.on_store_page(CAFE)
        # No order now, each with its own reason. A store that only stopped taking orders, for the day or for a
        # while, shows its menu to read: the products and their prices, and nothing to put in a basket.
        for number, name, line, reason, notice in (
                (14, HANOK, "오늘 쉽니다", "오늘은 쉬는 날이라 주문을 받지 않습니다. 메뉴는 볼 수 있습니다.", "오늘은 매장이 쉬는 날입니다"),
                (16, SEOCHON, "주문 잠시 멈춤 · 15:30쯤 다시 받습니다", "매장이 주문을 잠시 멈췄습니다. 15:30쯤 다시 받는다고 알렸습니다. 메뉴는 볼 수 있습니다.",
                 "매장이 주문을 잠시 멈췄습니다")):
            self.store_page(name)
            seen = self.words()
            assert reason in seen and "메뉴만 보기" in seen and "메뉴 보고 주문" not in seen, f"{name}: {seen}"
            assert any(line in word for word in seen), f"{name} does not say {line}: {seen}"
            self.shot(f"{number:03d}-store-{name.split()[0].lower()}-no-orders")
            self.tap("메뉴만 보기")
            self.sees(f"{name} 메뉴")
            self.sees(notice)
            menu = self.page()
            product = "아메리카노 · " + self.of("아메리카노")
            assert any(product in word for word in menu), f"the store page says the menu of {name} can be read, and it shows no product: {[word for word in menu if word not in TABS][2:]}"
            self.find("아메리카노 · " + self.of("아메리카노")) if product in menu else self.swipe()
            self.shot(f"{number + 1:03d}-menu-{name.split()[0].lower()}-read-only")
            self.top()
            offered = [word for word in menu if word.endswith(" 추가") or word in ("주문 접수 요청", "옵션 라떼 옵션 고르기")]
            assert not offered, f"{name} takes no order and its menu offers {offered}"
            assert not any("가능" in word or "1000" in word for word in menu), f"the menu of {name} tells a count: {menu}"
            self.tap("매장 화면으로")
            self.on_store_page(name)
        # A store with nothing on sale. This walk first expected its page to say "메뉴는 볼 수 있습니다." and to open a
        # menu that held one sentence; a page must not promise a menu that is not there, so the page now says only
        # why no order is taken, and has no button to a menu.
        self.store_page(NIGHT, menu=False)
        seen = self.page()
        assert "지금 판매 중인 메뉴가 없어 주문을 받지 않습니다." in seen, f"{NIGHT}: {seen}"
        promised = [word for word in seen if "메뉴는 볼 수 있습니다" in word or word in ("메뉴만 보기", "메뉴 보고 주문")]
        assert not promised, f"{NIGHT} has nothing on sale and its page promises a menu: {promised}"
        self.shot("018-store-night-no-menu")
        self.store_page(BUSY)
        self.sees("바쁨, 약 15분")
        assert "메뉴 보고 주문" in self.words(), "a busy store takes orders"
        self.shot("020-store-busy")

    def shortcut_qr(self):
        # The store's shortcut QR holds its reference alone; this store is in no list.
        self.step("shortcut", store="alley")
        self.finder()
        self.tap("매장 바로가기 QR 찍기")
        self.sees(ALLEY)
        self.sees("바로가기 QR로만 여는 작은 가게.")
        self.shot("021-store-opened-by-shortcut-qr")
        self.tap("매장 찾기로")
        self.sees("모든 매장")
        # A QR of something else, and a store that does not exist.
        self.step("shortcut", text="https://example.com/menu")
        self.tap("매장 바로가기 QR 찍기")
        self.sees("이 QR로는 매장을 열 수 없습니다")
        self.shot("022-shortcut-qr-not-a-store")
        self.step("shortcut", text="nu54://shop/0fbc54ef-2827-4ab5-bfcd-3a27611c39e0")
        self.tap("다시 찍기")
        self.sees("이 매장을 찾지 못했습니다")
        self.shot("023-shortcut-qr-unknown-store")
        self.tap("매장 찾기로")
        self.sees("최근 매장")
        seen = self.words()
        recent = seen[seen.index("최근 매장") + 1:seen.index("최근 매장 지우기")]
        assert ALLEY in recent and len({word for word in recent if word in STORES}) == 3, recent
        self.shot("024-finder-recent-stores")
        self.tap("최근 매장 지우기")
        time.sleep(2)
        assert not self.shown("최근 매장"), "the recent stores stay after they were cleared"
        self.shot("025-finder-recent-cleared")

    def finder_problems(self):
        # No connection to the store server.
        self.tap("홈")
        self.entrance(entrance=False)
        self.finder()
        self.sees("매장 목록을 불러오지 못했습니다. 연결을 확인하고 다시 시도하세요.")
        self.shot("026-finder-no-connection")
        self.entrance(entrance=True)
        self.tap("다시 불러오기")
        self.sees("더 있습니다")
        self.entrance(entrance=False)
        self.tap(BUSY)
        self.sees("매장 화면을 불러오지 못했습니다. 연결을 확인하고 다시 시도하세요.")
        self.shot("027-store-page-no-connection")
        self.entrance(entrance=True)
        self.tap("다시 불러오기")
        self.on_store_page(BUSY)
        # The server says "too many requests" itself: its limit of public reads is made small for a moment.
        self.tap("홈")
        self.step("limit", kind="read", perMinute=4, burst=1)  # one read, then one every 15 seconds
        try:
            self.finder()
            self.tap("홈")
            self.finder()
            self.sees("요청이 많습니다. 잠시 뒤 다시 시도하세요.")
            self.shot("028-finder-busy")
            wait = self.last_wait()
        finally:
            self.step("limit", kind="read")
        time.sleep(wait + 1)  # the phone sends nothing until the time the server named
        self.tap("다시 불러오기")
        self.sees("더 있습니다")

    def menu_without_board(self):
        """The menu, its choices and the basket need no board and no proof; the counts of what is left are not told."""
        self.menu()
        self.sees("메뉴는 누구나 볼 수 있습니다. 주문할 때 보드로 본인을 확인합니다.")
        self.sees(f"아메리카노 · {self.of('아메리카노')}")
        self.shot("029-menu-public")
        seen = self.page()
        assert "품절" in seen and not any("가능" in word or "1000" in word for word in seen), f"the public menu tells a count: {seen}"
        assert not self.find("스콘 추가")["enabled"], "a sold out product can be added"
        self.shot("030-menu-public-sold-out")
        self.tap("옵션 라떼 옵션 고르기")
        self.sees("온도")
        self.shot("031-options")
        self.tap("온도 차갑게")
        self.tap(f"크기 크게 +{self.money(1)}")
        self.shot("032-options-picked")
        self.tap(f"담기 · {self.money(9)}")
        self.sees(f"{CAFE} 메뉴")
        assert any("차갑게 · 크게" in word for word in self.page()), "the combination that was picked is not in the menu's list"
        self.add("아메리카노", 2)
        self.sees("선택 2")
        self.shot("033-menu-basket-counts")
        self.find("주문 접수 요청")
        self.sees("합계")
        self.shot("034-basket-without-board")
        asked = self.step("entrance", change={})["answered"]
        assert not [kind for kind in asked if kind.split()[0] in ("challenge", "proof") or "." in kind.split()[0]], f"reading the menu asked a board or a session: {asked}"
        # Asking for the order without a board account: the answer is drawn above the menu, and the page shows it
        # where the customer is left after the press.
        self.tap("주문 접수 요청")
        time.sleep(2)
        self.shot("035-order-asked-without-board-as-left")
        assert self.shown("주문하려면 먼저 지갑 탭에서 보드를 연결하고 서명 시험을 마치세요."), \
            f"after '주문 접수 요청' the screen shows no answer: {[word for word in self.words() if word not in TABS][:6]}"
        self.top()
        self.sees("주문하려면 먼저 지갑 탭에서 보드를 연결하고 서명 시험을 마치세요.")
        self.shot("036-order-needs-board")
        self.tap("내 보드 열기")
        self.sees("LED 신호 보는 법")
        self.shot("037-board-page-from-order")
        seen = self.page()
        assert any(label in seen for label in ("보드 연결 시작", "설정 열기", "보드 연결·상태 확인")), \
            f"a customer without a board is sent to a page with no way to connect one: {[word for word in seen if word not in TABS][:6]}"

    # ---- the flows of `orders` ----
    def set_up(self):
        self.tap("홈")
        self.connect_board("101-register", "102-return-address")
        self.sees("입금 안내 보기")
        seen = self.words()
        assert not any("매장 등록" in word or "매장에 보드 등록" in word for word in seen), f"this build asks for a store registration: {seen}"
        self.shot("103-setup-without-store-registration")

    def balances_in_this_build(self):
        """What the board account holds, where this build shows it: less than one won, and a part of a won."""
        small, part = (99, 299_900) if self.won else (9_999, 2_990_000)  # 0.99 won, then 2,999.99 won; 0.009999, then 2.999999 dollars
        self.step("deposit", amount=str(small * self.unit))
        self.tap("입금 안내 보기")
        if self.won:
            self.sees(self.label + "는 StableNet 테스트넷에만 있습니다")
            self.shot("104-deposit-network-first")
            self.tap("확인했습니다. 주소 보기")
        self.sees("입금 주소 QR")
        self.shot("105-deposit-address")
        self.tap("지갑")
        self.sees("토큰 추가")
        time.sleep(6)
        self.shot("106-wallet-below-one-unit")
        self.menu()
        self.add("아메리카노")
        self.find("주문 접수 요청")
        self.sees("보드 잔액 · " + self.price(0, 0), 60)
        self.shot("107-basket-balance-below-one-unit")
        self.step("deposit", amount=str(part * self.unit))
        deadline = time.monotonic() + 60
        while not self.shown("보드 잔액 · " + self.price(2_999, 299)):
            assert time.monotonic() < deadline, f"the basket does not show the balance: {self.words()}"
            self.tap("잔액 다시 확인")
            time.sleep(2)
        self.sees("모자란 금액 · " + self.price(1_501, 151))
        self.shot("108-basket-balance-part-of-a-unit")
        # The wallet tab: the token a board of this build pays in, and what the account holds of it, cut off.
        held = f"{self.label} 2,999원" if self.won else "TestUSDC 2.99"
        self.tap("지갑")
        self.sees("토큰 추가")
        self.sees(held, 60)
        self.shot("109-wallet-part-of-a-unit")
        rows = [word for word in self.words() if word.startswith(self.label + " ")]
        assert rows and all(word.startswith(held + ",") or word == held for word in rows), f"the wallet shows more than is held, or a part of a unit: {rows}"
        # Sending, test tokens and the return go through a store's registered connection, which this build does not
        # set up. Each page says so in place of its action, and what the customer can do instead.
        for number, button, name, said, dead in (
                (110, "보내기", "send", "이 앱에서는 아직 보낼 수 없습니다", ("다시 확인", "다음")),
                (111, "시험 토큰", "test-tokens", "이 앱에서는 아직 시험 토큰을 받을 수 없습니다", ("다시 확인",)),
                (112, "반납", "return", "이 앱에서는 아직 반납할 수 없습니다", ("반납 시작", "잔액 없이 초기화로 진행"))):
            self.tap("지갑")
            self.tap(button)
            self.sees(said, 60)
            self.shot(f"{number:03d}-{name}-not-in-this-build")
            seen = self.page()
            offered = [label for label in dead if label in seen]
            assert not offered, f"{button} cannot work in this build and its page offers {offered}"
            assert not any("쓸 수 없는 기능" in word for word in seen), f"{button}: the page says only that something cannot be used: {seen}"
            if button == "반납":
                assert any(self.price(2_999, 299) in word for word in seen), f"the return page does not say what the account holds: {seen}"
                assert not any("돌려받을 금액" in word for word in seen), f"the return page promises money back: {seen}"
                self.tap("돌아가기")
            else:
                self.back()

    def reset_without_orders(self):
        """The questions before a factory reset, for an account that never ordered."""
        asked = self.answered("proof 200")
        self.reset_page()
        self.sees("앱에서 초기화 계속", 60)
        self.sees(f"남은 돈 {self.price(2_999, 299)}을 잃습니다.")
        assert not self.shown("끝나지 않은 주문이 있"), "an account without an order is held back"
        self.shot("113-reset-no-open-order")
        # Nothing in this build sends the money back, so the page does not say to return it first.
        seen = self.page()
        assert any("이 앱에서는 아직 반납할 수 없어, 남은 돈을 원래 지갑으로 돌려받을 방법이 없습니다." in word for word in seen), f"the reset page does not say the money cannot be returned: {seen}"
        assert "먼저 반납하기" not in seen and not any("먼저 반납하세요" in word for word in seen), "the reset page says to return first, which this build cannot do"
        assert self.answered("proof 200") > asked, "the entrance was not asked how many orders of the account are unfinished"
        # The entrance cannot be asked: not known, never "none".
        self.tap("홈")
        self.entrance(entrance=False)
        self.reset_page()
        self.sees("끝나지 않은 주문이 있는지 알 수 없습니다", 60)
        self.sees("매장 서버에 확인하지 못했습니다. 끝나지 않은 주문이 있으면 그 환불금을 영영 받을 수 없습니다.")
        assert "앱에서 초기화 계속" not in self.page(), "a reset is offered while the server could not be asked"
        self.shot("114-reset-server-not-asked")
        self.entrance(entrance=True)
        # The board is away, so its account cannot be proven to the entrance.
        self.tap("홈")
        self.step("around", change={"board": False})
        self.reset_page()
        self.sees("보드가 가까이 없어 매장 서버에 물어보지 못했습니다. 보드를 켜고 가까이 둔 뒤 다시 확인하세요.", 60)
        self.shot("115-reset-board-away")
        self.step("around", change={"board": True})
        self.tap("다시 확인")
        self.sees("앱에서 초기화 계속", 60)
        self.shot("116-reset-asked-again")

    def shortfall(self):
        """A basket the board account cannot pay: what is missing, the way to top up, and the basket after it."""
        self.menu(empty=True)
        self.add("아메리카노")
        self.find("주문 접수 요청")
        self.sees("모자란 금액 · " + self.price(1_501, 151), 60)
        self.sees("보드 잔액이 합계보다 적어 주문할 수 없습니다. 지갑에서 충전한 뒤 다시 확인하세요.")
        assert not self.find("주문 접수 요청")["enabled"], "an order is offered for a basket that cannot be paid"
        self.shot("117-basket-shortfall")
        self.tap("충전하러 가기")
        self.sees("보드 설정")
        self.shot("118-top-up-leads-to-the-board-page")
        self.step("deposit", amount=str(90_000_000 * self.unit))
        self.tap("결제")
        self.sees(f"{CAFE} 메뉴")
        self.shot("119-menu-after-top-up")
        kept = any(word.startswith("선택 1") for word in self.page())
        if not kept:
            self.add("아메리카노")
        self.find("주문 접수 요청")
        self.sees("보드 잔액 · " + self.price(902_999, 9_299), 60)  # 900,000 won more, or 90 dollars
        assert not self.shown("모자란 금액"), "the basket still says money is missing"
        self.shot("120-basket-after-top-up")
        assert kept, "the way to the top-up and back emptied the basket: the same basket cannot be ordered after topping up"

    def checkout(self):
        """The board proves its account once, with no button; the menu is read again with what is left; the cart is held."""
        self.menu(empty=True)
        self.add("아메리카노")
        self.add("치즈케이크", 2)
        self.find("주문 접수 요청")
        self.sees("합계 · " + self.of("아메리카노", "치즈케이크", "치즈케이크"))
        self.shot("121-basket")
        # Another customer takes two of the three cheesecakes; the public menu does not say so.
        self.step("stock", sku="cheesecake", count=1)
        prompts, proofs, carts = self.step("state")["prompts"], self.answered("proof 200"), self.answered("order.create 200")
        self.tapped_then_shot("주문 접수 요청", "122-order-asked-board-proving")
        time.sleep(4)
        self.find("바뀐 장바구니 확인 후 주문 접수")
        self.sees("확보 못한 품목 · 치즈케이크 ×1", 30)
        self.sees("합계 · " + self.of("아메리카노", "치즈케이크"))
        self.shot("123-basket-reconciled")
        self.top()
        self.sees("선택 1 · 가능 1")
        self.shot("124-menu-with-counts")
        assert self.answered("proof 200") == proofs + 1 and self.answered("order.create 200") == carts, "the basket that could not be filled was held, or the board was not asked once"
        assert self.step("state")["prompts"] == prompts, "the account proof asked for a button"
        self.tap("바뀐 장바구니 확인 후 주문 접수")
        self.held()
        self.top()
        self.sees("금액 " + self.of("아메리카노", "치즈케이크"))
        self.shot("125-order-held")
        # A customer knows the order by its store, its amount and its number: the card shows no reference of it.
        coded = [word for word in self.page() if re.search(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", word)]
        assert not coded, f"the card of a held order shows a reference: {coded}"
        assert self.answered("proof 200") == proofs + 1, "the board was asked again for the same store"
        # Giving the order up releases its stock at the store.
        self.tap("주문 그만두기")
        self.sees("주문을 그만두었습니다. 이전 주문은 취소되어 잡아 둔 재고가 풀렸습니다.")
        self.shot("126-order-given-up")
        assert "합계" not in " ".join(self.page()), "the basket stays after the order was given up"
        self.step("stock", sku="cheesecake", count=3)

    def checkout_problems(self):
        """What the checkout says when the store's server is busy, cannot be reached, released the cart or takes no board order."""
        self.menu()
        self.add("아메리카노")
        self.ask_for_order()
        self.held()
        # Too many requests: the session's own limit, made small. The server answers 429 with a Retry-After.
        self.step("limit", kind="session", perMinute=20, burst=1)
        try:
            self.tap("이 주문의 결제 내용 검토")
            self.sees("요청이 많습니다. 잠시 뒤 다시 시도하세요.")
            self.shot("127-checkout-busy")
            wait = self.last_wait()
        finally:
            self.step("limit", kind="session")
        time.sleep(wait + 1)  # the phone sends nothing until the time the server named
        # The connection is lost.
        self.entrance(entrance=False)
        self.tap("이 주문의 결제 내용 검토")
        time.sleep(3)
        self.top()
        self.shot("128-checkout-no-connection")
        lost = self.words()
        self.entrance(entrance=True)
        self.tap("주문 그만두기")
        self.sees("주문을 그만두었습니다.")
        unsaid = "" if any("연결을 확인" in word or "연결하지 못했습니다" in word for word in lost) else lost[1]
        # The answer to a cart is lost on its way back, and the store's hold of that cart ends meanwhile.
        self.add("아메리카노")
        self.entrance(loseReply="order.create")
        self.ask_for_order()
        self.sees("저장한 주문 다시 확인")
        self.shot("129-order-answer-lost")
        ended = self.step("endHold")["carts"]
        assert ended == 1, f"{ended} carts were held"
        time.sleep(4)  # the store's expiry loop closes it
        self.tap("저장한 주문 다시 확인")
        self.sees("이전 주문은 결제되지 않은 채 매장에서 취소되었거나 결제 시간이 지나 닫혔습니다. 잡아 둔 재고는 없습니다. 장바구니를 확인하고 다시 주문하세요.", 30)
        self.shot("130-cart-released")
        self.find("주문 접수 요청")
        self.sees("합계 · " + self.of("아메리카노"))
        # No board near when the order is asked for. The store's session of before still stands, so a new store is asked.
        self.menu(HANOK, "메뉴만 보기")
        self.menu(GARDEN)
        self.add("아메리카노")
        self.step("around", change={"board": False})
        self.ask_for_order()
        self.sees("결제할 보드를 찾지 못했습니다. 보드를 켜고 휴대전화 가까이 둔 뒤 다시 시도하세요.", 60)
        self.shot("131-checkout-board-away")
        self.step("around", change={"board": True})
        # A store with a number in the order contract and no way to be paid by a board: asking again does not help.
        self.ask_for_order()
        self.sees("이 매장은 지금 보드 결제 주문을 받을 수 없습니다. 다시 시도해도 접수되지 않습니다. 매장이나 운영자에게 문의하세요.", 30)
        self.shot("132-store-takes-no-board-orders")
        assert any(word.startswith("선택 1") for word in self.page()), "the basket was emptied"
        # A store without a number in the order contract.
        self.menu(BUSY)
        self.sees("지금은 약 15분 걸립니다")
        self.add("아메리카노")
        self.ask_for_order()
        self.sees("이 매장은 아직 앱에서 주문 결제를 받지 않습니다.")
        self.shot("133-store-not-in-the-order-contract")
        assert not unsaid, f"a connection lost at the payment review is not said as one; the page says: {unsaid}"

    def pay(self):
        self.menu(empty=True)
        self.add("아메리카노")
        self.add("치즈케이크")
        total = self.of("아메리카노", "치즈케이크")
        self.ask_for_order()
        self.held()
        self.review()
        self.sees(total)
        self.shot("134-payment-review")
        sessions = self.answered("proof 200")
        self.approve("135-payment-board-checked", "136-payment-approve")
        self.sees(f"{CAFE} 주문번호")
        self.sees("결제 금액 " + total)
        self.shot("137-order-placed")
        self.sale = self.newest_order()
        assert self.answered("order.lock 200") >= 1, "the store's server took no lock"
        assert self.answered("proof 200") == sessions + 1, "the paid order got no session of its own, or the board was asked more than once"
        self.tap("확인")
        self.sees(f"{CAFE} 메뉴")
        seen = self.page()
        assert "주문 그만두기" not in seen and "새로 주문하기" not in seen, "a paid order can be given up"

    def order_status(self):
        """The paid order on its own page, until the store has it ready."""
        self.open_order()
        self.sees("매장이 주문을 받았습니다.")
        self.shot("138-order-accepted")
        assert "수령 QR 받기" not in self.page(), "an order that is not ready offers a pickup QR"
        self.step("cook", sale=self.sale, targets=["preparing"])
        self.refresh("주문 단계: 준비 중")
        self.shot("139-order-preparing")
        self.step("cook", sale=self.sale, targets=["ready"])
        self.refresh("준비되었습니다. 카운터에서 수령 QR을 보여 주세요.")
        self.sees("수령 QR 받기")
        self.shot("140-order-ready")
        deadline = time.monotonic() + 40
        while "주문한 상품이 준비되었습니다" not in self.shell("dumpsys", "notification", "--noredact").decode(errors="replace"):
            assert time.monotonic() < deadline, "no notification that the order is ready"
            time.sleep(1)

    def pickup(self):
        """The pickup QR: shown after the paying board proved its account, renewed, not renewed without the board, scanned by the store."""
        self.open_order()
        prompts, proofs = self.step("state")["prompts"], self.answered("proof 200")
        self.tap("수령 QR 받기")
        self.sees("이 QR을 쓸 수 있는 시간", 30)
        self.shot("141-pickup-qr")
        assert self.step("state")["prompts"] == prompts and self.answered("proof 200") == proofs + 1, "the pickup QR asked for a button, or not for one proof"
        # The QR lives a minute; then the board proves again and a new one is shown.
        time.sleep(62)
        self.sees("이 QR을 쓸 수 있는 시간", 40)
        assert self.answered("proof 200") >= proofs + 2, "the QR was not renewed"
        self.shot("142-pickup-qr-renewed")
        # Without the board the QR runs out and is not renewed.
        self.step("around", change={"board": False})
        time.sleep(70)
        self.shot("143-pickup-qr-ran-out-board-away")
        ran_out = self.words()
        self.step("around", change={"board": True})
        # The store does not take the QR that ran out: its scanner reads what the screen held last.
        late = self.step("scan", sale=self.sale)
        assert late["status"] == 400 and late["body"]["reason"] == "expired", late
        assert "결제한 보드가 필요합니다" in ran_out and "다시 확인하기" in ran_out, f"a QR that ran out without the board does not say so: {ran_out}"
        self.tap("다시 확인하기")
        self.sees("이 QR을 쓸 수 있는 시간", 60)
        self.shot("144-pickup-qr-again")
        # The store's operator scans it on the order's page of the operations web.
        scanned = self.step("scan", sale=self.sale)
        assert scanned["status"] == 200 and scanned["body"]["saleRef"] == self.sale, scanned
        self.sees("매장이 전달", 60)
        self.shot("145-pickup-scanned")
        assert "수령 QR 받기" not in self.page(), "an order that was handed over offers a pickup QR"

    def order_end(self):
        """The customer says the order was received, and the order ends: paid out to the store."""
        self.open_order()
        self.refresh("받았어요", 90)
        self.shot("146-order-handed-over")
        self.tap("받았어요")
        self.sees("확인하면 매장에 대금이 지급되고 취소할 수 없습니다. 물건을 받았나요?")
        self.shot("147-order-received-question")
        before = self.step("entrance", change={})["answered"]
        self.tap("네, 받았어요")
        self.sees("'받았어요' 답을 확인하고 있습니다", 30)
        self.shot("148-order-answer-sent")
        deadline, done = time.monotonic() + 100, "받았다고 확인해서 매장에 지급되었습니다"
        while not self.shown(done) and time.monotonic() < deadline:
            self.ask_again()
            time.sleep(5)
        self.shot("149-order-after-the-answer")
        seen, after = self.words(), self.step("entrance", change={})["answered"]
        self.tap("홈")
        time.sleep(3)
        self.shot("150-home-after-the-answer")
        home = self.words()
        since = {kind: after.get(kind, 0) - before.get(kind, 0) for kind in ("order.answer 200", "order.status 200", "order.status 404", "proof 200")}
        assert any(done in word for word in seen), (
            f"100 seconds after the answer the order page does not say that the store was paid: {[word for word in seen if '답' in word or '단계' in word]}; "
            f"since the answer the entrance answered {since}")
        # The order's own session read its end: the server turned no status read away, and the board proved its account
        # at most once more (for the answer itself, when the session was not there).
        assert since["order.status 404"] == 0 and since["proof 200"] <= 1, f"the phone lost the order's session while it waited for the end: {since}"
        offered = [label for label in ("상태 다시 확인", "받았어요", "못 받았어요", "수령 QR 받기") if label in seen]
        assert not offered, f"an order that ended still offers {offered}"
        listed = [word for word in home if word.startswith(f"{CAFE} 주문번호")]
        assert not listed, f"the order that ended is still in progress on the home tab: {listed}"

    def reset_with_open_order(self):
        """An order that is paid and not handed over holds the board's key: no return and no reset from the app."""
        self.menu()
        self.add("아메리카노")
        self.ask_for_order()
        self.held()
        self.review()
        self.approve("151-second-payment-board-checked", "152-second-payment-approve")
        self.tap("확인")
        self.sale = self.newest_order()
        self.reset_page()
        self.sees("끝나지 않은 주문이 있습니다", 60)
        self.shot("153-reset-open-order")
        seen = self.page()
        assert "앱에서 초기화 계속" not in seen and "보드 초기화" not in seen, "a reset is offered over an open order"
        self.tap("진행 중인 주문 보기")
        time.sleep(4)
        self.shot("154-open-orders-from-reset")
        led_to = self.words()
        self.tap("설정")
        self.tap("반납과 초기화")
        self.sees("끝나지 않은 주문이 있습니다", 60)
        self.shot("155-return-open-order")
        count = next((word for word in seen if word.startswith("주문 ") and "먼저 끝내세요" in word), "")
        wrong = []
        if not count.startswith("주문 1건"):
            wrong.append(f"one order is open at the store (the other ended), and the page says: {count}")
        if not any("주문번호" in word for word in led_to):
            wrong.append(f"'진행 중인 주문 보기' shows no order: {[word for word in led_to if word not in TABS][:4]}")
        assert not wrong, "; ".join(wrong)

    def former_board(self):
        """The board is reset with its own buttons and registered again: the orders of its former account stay, to be read."""
        account = self.step("state")["account"]
        deadline = time.monotonic() + 150  # the board refuses while the payment it approved last can still be sent
        while not self.step("factoryReset"):
            assert time.monotonic() < deadline, "the board refused its own factory reset"
            time.sleep(10)
        # The phone learns of it when it next asks the board, and forgets the account whose key is gone.
        self.tap("설정")
        self.tap("내 보드")
        self.tap("보드 연결·상태 확인")
        self.tap("개인 기기 찾기")
        self.tap("등록 상태 확인")
        self.sees("보드가 초기화되어 있어 이 휴대전화의 이전 보드 정보를 지웠습니다. 이전 주소로 보내지 말고 다시 등록하세요.", 60)
        self.shot("156-board-found-reset")
        self.register_board("157-register-again", "158-return-address-again")
        assert self.step("state")["account"] != account, "the board kept its account"
        self.tap("홈")
        self.sees("이전 보드 · ", 60)
        self.shot("159-home-orders-of-former-board")
        # The board was just asked for its limits: the home tab says them in the token of this build's order contract.
        limits = [word for word in self.words() if "오늘 더 승인할 수 있는 금액" in word or "하루에" in word or word == "확인 안 됨"]
        assert any("하루에" in word for word in limits) and not any("확인 안 됨" in word for word in limits), \
            f"the home tab does not say the board's limits in the token of this build: {limits}"
        self.touch(next(node for node in self.nodes() if node["words"][1].startswith(f"{CAFE} 주문번호")))
        self.sees("이전 보드의 주문", 60)
        self.shot("160-order-of-former-board")
        seen = self.page()
        for text in ("지금 이 휴대전화에 연결된 보드 계정이 아니라, 초기화했거나 바꾼 이전 보드 계정으로 결제한 주문입니다.",
                     "주문 상태 확인, 받았다는 답, 이의, 수령 QR은 결제한 보드 계정의 확인이 필요해서 이 앱에서는 할 수 없습니다. 위 상태는 새로 확인한 것이 아닙니다.",
                     "환불되면 이전 보드 계정으로 가서 되찾을 수 없습니다. 매장이나 운영자에게 문의하세요."):
            assert text in seen, f"the order of a former board does not say: {text}: {seen}"
        assert not {"받았어요", "못 받았어요", "수령 QR 받기", "상태 다시 확인"} & set(seen), f"an order of a former board offers what needs its key: {seen}"
        proofs = self.answered("proof 200")
        time.sleep(20)  # the home tab keeps asking about open orders; a former board's are not asked about
        assert self.answered("proof 200") == proofs, "the new board was asked to prove the account of the former one"

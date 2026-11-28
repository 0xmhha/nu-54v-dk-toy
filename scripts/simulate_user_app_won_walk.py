"""Taps through the wallet of a won store on the route with a registration. Run by simulate_user_app.py --walk --part won-wallet --token won.

Won are whole on a screen: what a part of a won and less than one won look like in the wallet, that a transfer is
typed in whole won, and what is said where money stays that no screen shows. The build that orders without a
registration has no transfer, so these screens are walked where they are.
"""

import time

from simulate_user_app_walk import Walk

WHOLE_WON_ONLY = "원화는 1원 단위로만 보낼 수 있습니다. 소수점 없이 숫자만 입력하세요."
UNDER_ONE_WON_LEFT = "1원보다 적은 금액이 남습니다. 보내기로는 보낼 수 없고, 반납할 때 함께 보냅니다."
UNDER_ONE_WON_RETURNED = "1원보다 적은 금액이 남아 있습니다. 화면에는 0원으로 보이지만 남은 금액을 그대로 보냅니다."
UNDER_ONE_WON_LOST = "1원보다 적은 돈이 남아 있습니다. 초기화하면 잃습니다."


class WonWalletWalk(Walk):
    NEEDED = ("agree", "set_up", "part_of_a_won")

    def flows(self):
        return (self.agree, self.set_up, self.part_of_a_won, self.decimal_refused, self.whole_won_sent, self.below_one_won)

    def shown(self, text):
        return any(text in word for word in self.words())

    def field(self, label):
        """What an input field holds now."""
        node = self.find(label)
        assert node, f"no field {label}"
        return node["words"][0]

    def to_send(self):
        self.tap("지갑")
        self.tap("보내기")
        self.sees("오늘 남은 횟수")

    def part_of_a_won(self):
        """12,999.99 won on the board account reads 12,999 won: cut off, never rounded up."""
        self.step("deposit", amount=str(1_299_999 * self.unit))
        self.tap("입금 안내 보기")
        self.sees(self.label + "는 StableNet 테스트넷에만 있습니다")
        self.tap("확인했습니다. 주소 보기")
        self.sees("입금 주소 QR")
        self.tap("지갑")
        self.sees(f"{self.label} 12,999원", 60)
        self.shot("03-wallet-part-of-a-won")
        seen = self.words()
        assert not any("12,999.99" in word or "13,000원" in word for word in seen), f"the wallet shows the part of a won, or rounds it up: {seen}"
        self.to_send()
        self.sees("12,999원")
        self.shot("04-send-with-part-of-a-won")

    def decimal_refused(self):
        """A won amount takes digits alone: a decimal point is refused with a sentence at the field, and the sentence
        stays while the customer types on, so that 1000.5 never becomes 10,005 won without a word."""
        self.to_send()
        self.type("받는 주소", "0x" + "f1" * 20)
        self.type("금액", "1000.")
        self.sees(WHOLE_WON_ONLY)
        assert self.field("금액") == "1000", f"the field took the decimal point: {self.field('금액')}"
        field, said = self.find("금액")["box"], next(node["box"] for node in self.nodes() if WHOLE_WON_ONLY in node["words"][0])
        assert 0 <= said[1] - field[3] < 80, f"the sentence is not directly below the amount field: field {field}, sentence {said}"
        self.shot("05-send-decimal-point-refused")
        # The customer goes on typing the amount they meant, 1000.5: the field now reads a number nobody typed.
        self.type("금액", "5")
        after, said = self.field("금액"), self.shown(WHOLE_WON_ONLY)
        self.shot("06-send-after-the-refused-point")
        assert said and not self.find("다음")["enabled"], (
            f"1000.5 was typed and the field reads {after}; the sentence {'stays' if said else 'is gone'} and the next page can be opened")
        # Taking a character out is the customer's own change: the sentence goes and the page goes on with what the field holds.
        self.touch(self.find("금액"))
        self.shell("input", "keyevent", "123")  # to the end of the field
        self.shell("input", "keyevent", "67")
        if b"mInputShown=true" in self.shell("dumpsys", "input_method"):
            self.back()
        assert self.field("금액") == "1000" and not self.shown(WHOLE_WON_ONLY), f"after a character was deleted: {self.field('금액')}, {self.words()}"
        self.shot("07-send-after-a-character-was-deleted")
        self.tap("다음")
        self.sees("처음 보내는 주소입니다")
        self.sees("1,000원")
        self.shot("08-send-review-after-the-correction")
        self.tap("고치기")
        self.tap("지갑")

    def whole_won_sent(self):
        """A whole amount goes out as typed; what stays below one won is said before the board is asked."""
        before = int(self.step("state")["balance"])
        self.to_send()
        self.type("받는 주소", "0x" + "f2" * 20)
        self.clear("금액")
        self.type("금액", "12999")
        self.tap("다음")
        self.sees("처음 보내는 주소입니다")
        self.sees("12,999원")
        self.sees(UNDER_ONE_WON_LEFT)
        self.shot("09-send-review-whole-won")
        seen = self.words()
        assert "0원" in seen, f"what is left after the transfer is not shown as 0 won: {seen}"
        self.tap("이 내용으로 보내기")
        self.sees("보드에는 화면이 없습니다")
        if not self.find("보드로 승인"):
            self.tap("내 보드 찾기")
        self.pressed_late("보드로 승인", "10-send-approve")
        self.sees("거래가 확정되었습니다", 90)
        assert int(self.step("state")["balance"]) == before - 1_299_900 * self.unit, "the transfer moved another amount than 12,999 won"
        self.shot("11-sent")
        self.tap("확인")
        self.back()

    def below_one_won(self):
        """0.99 won stays: the wallet shows 0 won, and each page that sends or loses it says so."""
        assert int(self.step("state")["balance"]) == 99 * self.unit, "the account does not hold 0.99 won"
        self.tap("지갑")
        self.sees(f"{self.label} 0원", 60)
        self.shot("12-wallet-below-one-won")
        self.tap("반납")
        self.sees(UNDER_ONE_WON_RETURNED, 60)
        self.sees("돌려받을 금액 0원")
        self.shot("13-return-below-one-won")
        self.tap("돌아가기")
        self.tap("설정")
        self.tap("내 보드")
        self.tap("공장 초기화")
        self.sees(UNDER_ONE_WON_LOST, 60)
        self.shot("14-reset-below-one-won")

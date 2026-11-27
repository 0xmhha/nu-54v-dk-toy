#!/usr/bin/env python3
"""The host board without an owner, through restarts: what it keeps in storage and what it starts with.

Usage: restart_test.py ORDER_BOARD

Needs nothing but the board program. The flows with an owner are in ../app (scripts/test_order_flow.py).
"""

import subprocess
import sys
import time

PHONE = "5a" * 32
POLICY = " ".join(["policy", "11" * 20, "22" * 20, "33" * 20, "44" * 20, "55" * 32, PHONE])
ZERO = "00" * 20
EPOCH = "e1" * 16
MANAGEMENT, MANAGE, CLOCK = 1, 1, 8  # the route and two commands of packages/protocol/v2/transport.md


class Board:
    def __init__(self, program):
        self.child = subprocess.Popen([program], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True, bufsize=1)
        self.clock = (0, 0)
        self.waits = False

    def until(self, *answers):
        """The first line that starts with one of the answers; what the board says before it is kept."""
        while True:
            line = self.child.stdout.readline().strip()
            if not line:
                raise SystemExit(f"FAIL the board ended (status {self.child.wait()}) before {answers}")
            word, *rest = line.split(" ")
            if word == "clock":
                self.clock = (int(rest[0]), int(rest[1]))
            elif word == "owner-wait":
                self.waits = rest[0] == "1"
            if word in answers:
                return line

    def ask(self, command, *answers):
        self.child.stdin.write(command + "\n")
        self.child.stdin.flush()
        return self.until(*(answers or ("ok",)))

    def boot(self):
        """Power cycle. Returns the device, the account and the generation of the start line."""
        self.waits = False
        return self.ask("boot", "booted").split(" ")[1:]

    def grant(self, life, ahead):
        return self.ask(f"grant {life} {ahead}", "ok", "refused") == "ok"

    def connects(self, period=""):
        opened = self.ask(f"connect {'e0' * 16} {PHONE}{period}", "open", "denied") == "open"
        if opened:
            self.ask("close")
        return opened

    def asks(self, command, payload, expires):
        """One request of the phone in a session of its own (packages/protocol/v2/transport.md): "out" or "ended"."""
        if self.ask(f"connect {EPOCH} {PHONE}", "open", "denied") != "open":
            return "denied"
        body = bytes([command]) + bytes([command, 9] + [0] * 14) + expires.to_bytes(8, "big") + payload
        frame = bytes.fromhex(EPOCH) + (0).to_bytes(4, "big") + bytes([0, 1]) + body
        answer = self.ask("data " + (bytes([MANAGEMENT]) + len(frame).to_bytes(2, "big") + frame).hex(), "out", "ended")
        if answer != "ended":
            self.ask("close")
        return answer.split(" ")[0]


def check(what, passed):
    if not passed:
        raise SystemExit(f"FAIL {what}")
    print(f"ok   {what}")


def main(program):
    board = Board(program)
    device, account, generation = board.until("booted").split(" ")[1:]
    check("a new board has no wallet key and no owner", account == ZERO and generation == "0")
    check("it advertises for its owner by itself", board.waits)
    check("no grant and no time", board.clock == (0, 0))
    check("no session without the operator's grant", not board.connects())
    board.ask(POLICY)
    check("the operator's grant", board.grant(3000, 2))
    earliest, latest = board.clock
    check("the time of the grant, four seconds wide", latest - earliest == 4 and abs(time.time() - 2 - earliest) < 2)
    check("a session under the grant", board.connects())
    # The time command is for the registered owner's phone. This board has none, so the phone the grant names for
    # the registration is answered when it asks for the status, and is not when it tells the time.
    now = int(time.time())
    check("the phone of the grant is answered", board.asks(MANAGE, bytes([3]), now + 60) == "out")
    check("a board without an owner does not take the time", board.asks(CLOCK, bytes([0x81]) + (now + 5).to_bytes(8, "big"), now + 95) == "ended")
    check("and its time is what it was", board.clock == (earliest, latest))

    time.sleep(2.2)
    again, account, generation = board.boot()
    check("the same board after the restart, still without a key and waiting", again == device and account == ZERO and board.waits)
    check("the grant is kept through the restart", board.connects())
    # The time is saved once an hour, so the two seconds since the grant are lost: the clock is late.
    check("the restarted board goes on from the time it saved", board.clock == (earliest, latest))
    now = int(time.time())
    check("a certificate that has ended is refused", not board.connects(f" 1 {now - 3600}"))
    check("only a registered owner comes in with a certificate that is not valid yet", not board.connects(f" {now + 3600} {now + 7200}"))

    check("a grant that took a minute or more is not taken", not board.grant(3000, 60))
    check("a grant no longer than its own uncertainty is not taken", not board.grant(60, 58))
    # 58 seconds to sign: the latest possible time is a minute ahead, so this grant ends two seconds later.
    check("a slow grant", board.grant(62, 58) and board.clock[1] - board.clock[0] == 60)
    check("a session while it runs", board.connects())
    board.boot()
    check("the slow grant is kept through the restart, as wide as it was", board.clock[1] - board.clock[0] == 60 and board.connects())
    # A fresh grant is the authority's word about the time and sets both ends. Before the audit of 2026-11-27 the
    # latest possible time never came down, and this check asked for a width of 58 seconds or more.
    check("a quick grant after a restart brings the latest possible time down",
          board.grant(3000, 2) and board.clock[1] - board.clock[0] == 4 and board.connects())
    check("a slow grant in the same start", board.grant(3000, 58) and board.clock[1] - board.clock[0] == 60 and board.connects())
    check("a quick grant in the same start brings it down as well",
          board.grant(3000, 2) and board.clock[1] - board.clock[0] == 4 and board.connects())
    # The slow one that ends two seconds after its start is tried again and left to end.
    check("the slow grant again", board.grant(62, 58))
    time.sleep(4.5)
    check("no session after its end", not board.connects())
    board.boot()
    check("a grant that ended is not taken up at the restart", board.clock == (0, 0) and not board.connects())
    check("a quick grant after it is four seconds wide again", board.grant(3000, 2) and board.clock[1] - board.clock[0] == 4 and board.connects())

    board.ask("nogrant")
    check("no session once the grant in use is gone", not board.connects())
    board.boot()
    check("the stored grant comes back with the restart", board.connects())
    board.child.stdin.close()
    check("the board ends with its input", board.child.wait(timeout=5) == 0)


if __name__ == "__main__":
    main(*sys.argv[1:])

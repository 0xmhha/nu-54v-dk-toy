"""app/led-patterns.json against status_led.c and its callers.

With --phone it reports where the app's board guide and that list differ. The report never fails:
the app and the design book change on their own.
"""

import json
from pathlib import Path
import re
import sys
import unittest

PRODUCT = Path(__file__).resolve().parents[1]
SOURCES = PRODUCT / "app/src"
WALLET = PRODUCT.parent / "p02-user-app/src/wallet"
BOOK = PRODUCT.parents[1] / "docs/content/commercial/screens/board.json"
LIST = json.loads((PRODUCT / "app/led-patterns.json").read_text())
PATTERNS = LIST["patterns"]


def c_patterns():
    found = re.findall(r"^PATTERN\((\w+), (true|false), ([\d, ]+)\);$", (SOURCES / "status_led.c").read_text(), re.MULTILINE)
    return {name: ([int(step) for step in steps.split(",")], repeat == "true") for name, repeat, steps in found}


def frames(text, name):
    """On and off times of one drawing loop in BoardStepPanel.tsx."""
    body = re.search(rf"const {name}: \[number, number\]\[\] = (\[\[.*?\]\]);", text, re.DOTALL)
    times = []
    for lit, ms in re.findall(r"\[([01]), (\d+)\]", body[1]):
        if times and times[-1][0] == lit:
            times[-1][1] += int(ms)
        else:
            times.append([lit, int(ms)])
    return [ms for _, ms in times]


def guides():
    """LED text per step, from the app and from the design book."""
    app = {}
    text = (WALLET / "boardSteps.ts").read_text()
    steps = text[text.index("export const boardSteps"):text.index("export const boardFlows")]
    for step, body in re.findall(r"^  (\w+): \{(label: .*?)\},\n(?=  \w+: \{label|\};)", steps, re.MULTILINE | re.DOTALL):
        led = re.search(r"\bled: '([^']*)'", body)
        app[step] = led[1] if led else None
    book = {step: value.get("led") for step, value in json.loads(BOOK.read_text())["steps"].items()}
    return app, book


def signal_report(app):
    """The rows of the LED list in HomeParts.tsx."""
    block = re.search(r"const signals: .*?= \[\n(.*?)\n\];", (WALLET / "HomeParts.tsx").read_text(), re.DOTALL)
    if not block:
        return ["no LED list in HomeParts.tsx"]
    rows = [(meaning, text or app[step]) for meaning, text, step in
            re.findall(r"\['([^']*)', (?:'([^']*)'|boardSteps\.(\w+)\.led!)\]", block[1])]
    lines = []
    for name, pattern in PATTERNS.items():
        expected, text = pattern["signal"]
        found = [meaning for meaning, shown in rows if re.search(rf"(?<![\d.]){re.escape(text)}", shown)
                 and not (name == "P_PIN_TAP" and shown != text)]
        if not found:
            lines.append(f"{name}: no row in the LED list")
        elif not pattern["spending"]:
            lines.append(f"{name}: row {found} is never shown by the spending profile: {pattern['why']}")
        else:
            lines.append(f"{'ok' if any(expected in row for row in found) else 'MEANING'} list {name}: {found}; firmware: {pattern['meaning']}")
    return lines


def phone_report():
    app, book = guides()
    lines = signal_report(app)
    for step in sorted(app.keys() | book.keys()):
        if app.get(step) != book.get(step):
            lines.append(f"step {step}: app and design book differ: {app.get(step)!r} / {book.get(step)!r}")
    for name, pattern in PATTERNS.items():
        steps = [step for step, text in app.items() if text and pattern["phoneText"] in text]
        if pattern["spending"] and not steps:
            lines.append(f"{name} (spending profile: {pattern['meaning']}): no step describes it")
        elif not pattern["spending"] and steps:
            lines.append(f"{name}: described in {', '.join(steps)}, but the spending profile never shows it: {pattern['why']}")
        elif pattern["spending"]:
            lines.append(f"ok {name}: {', '.join(steps)}")
    panel = (WALLET / "BoardStepPanel.tsx").read_text()
    step = {name: PATTERNS[name]["steps"] for name in PATTERNS}
    start, tap, keep, locate = step["P_PIN_START"], step["P_PIN_TAP"][0], step["P_PIN_KEEP"][0], step["P_LOCATE"]
    drawn = {"blinkFrames": frames(panel, "blinkFrames"), "pinLed": frames(panel, "pinLed"), "locateFrames": frames(panel, "locateFrames")}
    expected = {"blinkFrames": drawn["blinkFrames"] == step["P_WAITING"],
                "pinLed": drawn["pinLed"][:3] == start[:3] and drawn["pinLed"][4::2] == [tap, tap, keep],
                "locateFrames": drawn["locateFrames"][:-1] == locate[:-1]}
    lines += [f"{'ok' if same else 'MISMATCH'} drawing {name}: {drawn[name]}" for name, same in expected.items()]
    return lines


class LedPatternTest(unittest.TestCase):
    def test_every_pattern_is_listed_with_its_timing(self):
        self.assertEqual(c_patterns(), {name: (value["steps"], value["repeat"]) for name, value in PATTERNS.items()})

    def test_patterns_are_told_apart(self):
        shown = [(tuple(value["steps"]), value["repeat"]) for value in PATTERNS.values() if value["spending"]]
        self.assertEqual(len(shown), len(set(shown)))

    def test_a_pattern_that_cannot_show_says_why(self):
        self.assertEqual([], [name for name, value in PATTERNS.items() if value["spending"] == ("why" in value)])

    def test_situations_use_exactly_the_patterns_of_the_profile(self):
        used = {name for entry in LIST["situations"] for name in entry["patterns"]}
        self.assertEqual({name for name, value in PATTERNS.items() if value["spending"]}, used)

    def test_the_pin_waits_as_long_as_the_press_shows(self):
        shown = re.search(r"^#define STATUS_LED_PRESSED_MS (\d+)$", (SOURCES / "status_led.h").read_text(), re.MULTILINE)
        self.assertEqual(PATTERNS["P_PRESSED"]["steps"][0], int(shown[1]))

    def test_every_caller_is_listed(self):
        calls = {f"{path.name} {call}" for path in SOURCES.glob("*.c") if path.name != "status_led.c"
                 for call in re.findall(r"\b(status_led_\w+)\(", path.read_text())}
        listed = set(LIST["endsAPattern"]).union(*(value["callers"] for value in PATTERNS.values()))
        self.assertEqual(calls, listed)

    def test_nothing_else_drives_the_lamp(self):
        others = {path.name for path in SOURCES.glob("*.c")
                  if path.name not in ("status_led.c", "board_io.c") and "board_led_" in path.read_text()}
        self.assertEqual(set(LIST["outsideStatusLed"]), others)

    def test_the_header_names_each_pattern_once(self):
        comment = (SOURCES / "status_led.h").read_text().split("*/")[0]
        for text in ("a short blip every 2 s", "fast blinking (0.2 s)", "on for 0.6 s", "two long flashes / one short / one 0.5 s",
                     "five very short flashes", "very fast blinking (0.08 s)", "two long 1.5 s flashes", "three short flashes"):
            self.assertIn(text, comment)


if __name__ == "__main__":
    if sys.argv[1:] == ["--phone"]:
        print("\n".join(phone_report()))
    else:
        unittest.main()

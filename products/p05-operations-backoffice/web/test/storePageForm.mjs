import assert from "node:assert/strict";
import { hoursLines, storePageForm, storePageProblem, storePageWire } from "../src/storePageForm.ts";

const day = { opens: "10:00", closes: "21:00" };
const saved = { listed: true, kind: "카페", intro: "직접 볶은 원두", address: "시험시 시험로 1", directions: "", phone: "02-555-0100",
  amenities: "무선 인터넷", parking: "", closedNote: "매주 일요일", hours: [day, day, day, day, day, { opens: "11:00", closes: "20:00" }, null] };

// A saved page goes to the form and back unchanged.
const form = storePageForm(saved);
assert.equal(form.hoursGiven, true);
assert.deepEqual(form.days[6], { open: false, opens: "10:00", closes: "21:00" });
assert.equal(storePageProblem(form), undefined);
assert.deepEqual(storePageWire(form), saved);

// A page never saved: no hours are sent until the store gives them, and text is trimmed.
const empty = storePageForm({ listed: false, kind: "", intro: "", address: "", directions: "", phone: "", amenities: "", parking: "", closedNote: "", hours: null });
assert.equal(empty.hoursGiven, false);
assert.equal(storePageProblem(empty), "매장 종류를 적어 주세요. 예: 카페");
assert.deepEqual(storePageWire({ ...empty, kind: " 카페 ", intro: " 소개 " }).hours, null);
assert.equal(storePageWire({ ...empty, kind: " 카페 " }).kind, "카페");
assert.deepEqual(storePageWire({ ...empty, kind: "카페", hoursGiven: true }).hours, [null, null, null, null, null, null, null]);

// What cannot be saved, said before the save.
const broken = (change) => storePageProblem({ ...form, ...change });
assert.equal(broken({ intro: "가".repeat(201) }), "200자 안으로 줄여 주세요.");
assert.equal(broken({ intro: "가".repeat(200) }), undefined);
assert.equal(broken({ address: "a\nb" }), "줄바꿈은 넣을 수 없습니다.");
assert.equal(broken({ phone: "전화 주세요" }), "전화번호는 숫자, 빈칸, 하이픈으로 적어 주세요.");
assert.equal(broken({ phone: "+82 2-555-0100" }), undefined);
assert.equal(broken({ days: form.days.map((d, i) => (i === 0 ? { ...d, closes: "10:00" } : d)) }), "월요일의 여는 시간과 닫는 시간이 같습니다.");
assert.equal(broken({ days: form.days.map((d, i) => (i === 2 ? { ...d, opens: "" } : d)) }), "수요일의 시간을 확인해 주세요.");
// A closed day's leftover times are not checked, and hours that are not given are not checked at all.
assert.equal(broken({ days: form.days.map((d, i) => (i === 6 ? { open: false, opens: "", closes: "" } : d)) }), undefined);
assert.equal(broken({ hoursGiven: false, days: form.days.map((d) => ({ ...d, opens: "" })) }), undefined);

// Neighbouring weekdays with the same hours read together.
assert.deepEqual(hoursLines(saved.hours), [{ days: "월~금", time: "10:00~21:00" }, { days: "토", time: "11:00~20:00" }, { days: "일", time: "쉼" }]);
assert.deepEqual(hoursLines([day, day, day, day, day, day, day]), [{ days: "월~일", time: "10:00~21:00" }]);

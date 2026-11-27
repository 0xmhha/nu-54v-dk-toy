#!/usr/bin/env node
"use strict";

const assert = require("node:assert/strict");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const [origin, branch, sale, refund, hash, stage, cert, key, checkerCert, checkerKey] =
  process.argv.slice(2);

async function openFinance(browser, certPath, keyPath) {
  const page = await browser.newPage({
    ignoreHTTPSErrors: true,
    clientCertificates: [{ origin, certPath, keyPath }],
    viewport: { width: 390, height: 760 },
  });
  page.setDefaultTimeout(12000);
  await page.goto(origin + "/ops/");
  return page;
}

async function selectBranch(page) {
  await page.getByRole("button", { name: "정산과 이행" }).click();
  await page.getByLabel("지점 참조").fill(branch);
}

async function manualLookup(page) {
  await selectBranch(page);
  await page.getByLabel("환불 대상 원주문").fill(sale);
  await page.getByRole("button", { name: "원금과 환불 상태 조회" }).click();
  await page.getByRole("heading", { name: "원주문 환불 이력" }).waitFor();
}

function nextReply(page, operation) {
  return page.waitForResponse((response) => response.url().endsWith("/" + operation));
}

async function openFromQueue(page, id, expected) {
  await selectBranch(page);
  const reader = nextReply(page, "refund-queue");
  await page.getByRole("button", { name: "환불 대기열 조회", exact: true }).click();
  const reply = await reader;
  assert.equal(reply.status(), 200);
  const { items } = await reply.json();
  const entry = items.find((row) => row.refundId === id);
  assert.ok(entry && entry.saleId === sale);
  assert.deepEqual(Object.keys(entry).sort(), [
    "amount", "asset", "next", "principal", "refundId", "saleId", "state",
  ]);
  assert.equal(entry.next, expected);
  const open = page.getByRole("button", { name: `환불 ${id} 원주문 열기` });
  const read = nextReply(page, "refund-status");
  await open.click();
  const response = await read;
  assert.equal(response.status(), 200);
  const model = await response.json();
  assert.equal(model.saleId, sale);
  assert.equal(model.payer, undefined);
  const records = model.refunds.filter((row) => row.id === id);
  assert.equal(records.length, 1);
  const card = page.locator("article").filter({ hasText: id });
  await card.waitFor();
  return { record: records[0], card };
}

async function pendingReview(browser, maker) {
  await manualLookup(maker);
  await maker.getByLabel("환불 금액 (TestUSDC)").fill("0.55");
  await maker.getByRole("button", { name: "환불 신청 검토" }).click();
  const submit = maker.getByRole("button", { name: "같은 환불 요청 기록" });
  let resume;
  let captured;
  const pause = new Promise((resolve) => { resume = resolve; });
  const arrived = new Promise((resolve, reject) => { captured = { resolve, reject }; });
  await maker.route("**/v2/finance/refund-reserve", async (route) => {
    try {
      const reply = await route.fetch({ timeout: 10000 });
      assert.equal(reply.status(), 200);
      const { refundId } = await reply.json();
      await reply.dispose();
      captured.resolve({ refundId, requestKey: route.request().postDataJSON().idempotencyKey });
      await pause;
    } catch (error) {
      captured.reject(error);
    } finally {
      await route.abort("connectionreset");
    }
  }, { times: 1 });
  const dropped = maker.waitForEvent("requestfailed", (r) => r.url().endsWith("/refund-reserve"));
  await submit.click();
  let first;
  const held = maker.getByText("보낸 신청의 결과를 확인한 뒤 금액을 수정할 수 있습니다.", { exact: true });
  try {
    first = await arrived;
    await held.waitFor();
    assert.ok(await submit.isDisabled());
    assert.equal(await maker.getByLabel("환불 금액 (TestUSDC)").count(), 0);
  } finally {
    resume();
  }
  await dropped;
  await maker.reload();
  const own = await openFromQueue(maker, first.refundId, "requester_cannot_decide");
  assert.equal(await own.card.getByRole("button", { name: "별도 승인 검토" }).count(), 0);
  await held.waitFor();
  const replay = nextReply(maker, "refund-reserve");
  await submit.click();
  const reply = await replay;
  assert.equal(reply.status(), 200);
  assert.equal(reply.request().postDataJSON().idempotencyKey, first.requestKey);
  assert.equal((await reply.json()).refundId, first.refundId);
  const checker = await openFinance(browser, checkerCert, checkerKey);
  const checked = await openFromQueue(checker, first.refundId, "decide");
  await checked.card.getByRole("button", { name: "별도 승인 검토" }).click();
  await checker.getByRole("heading", { name: "별도 승인 · 0.55 TestUSDC" }).waitFor();
}

async function retryReview(page) {
  const next = { "pre-final": "await_chain", final: "read_result", maker: "requester_cannot_decide", approve: "decide", treasury: "review_funds" };
  const { record, card } = await openFromQueue(page, refund, next[stage]);
  const approve = card.getByRole("button", { name: "확정 실패 후 새 승인 검토" });
  const submit = card.getByRole("button", { name: "재원과 제출 검토" });
  if (stage === "pre-final" || stage === "final" || stage === "maker") {
    assert.equal(await approve.count(), 0);
    assert.equal(await submit.count(), 0);
  }
  if (stage === "pre-final") {
    assert.equal(record.state, "inflight");
    assert.equal(record.reviewRevision, 0);
    assert.ok(!record.previousFailure);
    return;
  }
  if (stage === "final") {
    assert.equal(record.state, "final");
    assert.equal(record.transaction, hash);
    await card.getByText("확정 거래 " + hash, { exact: true }).waitFor();
    return;
  }
  assert.equal(record.reviewRevision, 1);
  assert.equal(record.previousFailure, hash);
  await card.getByText("이전 전송 실패 확정 " + hash, { exact: false }).waitFor();
  if (stage === "maker") return;
  const checking = stage === "approve";
  await (checking ? approve : submit).click();
  const confirm = page.getByRole("region", { name: "환불 결재 확인" });
  await confirm.getByText(refund, { exact: false }).waitFor();
  const response = nextReply(page, checking ? "refund-approve" : "refund-submit");
  await confirm.getByRole("button", { name: checking ? "현재 계정으로 결정 기록" : "이 환불만 제출 요청" }).click();
  const done = await response;
  assert.equal(done.status(), 200);
  assert.equal(done.request().postDataJSON().reviewRevision, 1);
}

async function main() {
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  try {
    const page = await openFinance(browser, cert, key);
    if (stage === "pending") await pendingReview(browser, page);
    else await retryReview(page);
    console.log(stage + " verified using the original order and refund");
  } finally {
    await browser.close();
  }
}
main().catch((error) => {
  console.error(String(error.message).replace(/0x[0-9a-f]+|[a-f0-9-]{36}/gi, "[fixture]"));
  process.exitCode = 1;
});

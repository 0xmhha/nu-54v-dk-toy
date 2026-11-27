"use strict";

const assert = require("node:assert/strict");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const [origin, branch, certPath, keyPath, requestKey, phase, retained, expired] =
  process.argv.slice(2);

async function exercise() {
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  try {
    const page = await browser.newPage({
      ignoreHTTPSErrors: true,
      clientCertificates: [{ origin, certPath, keyPath }],
      viewport: { width: 1440, height: 900 },
    });
    page.setDefaultTimeout(12000);
    await page.goto(origin + "/ops/");
    await page.getByRole("button", { name: "지급 주소", exact: true }).click();
    await page.getByLabel("지점 참조").fill(branch);
    if (phase !== "start") await page.getByLabel("변경 요청 참조 (선택)").fill(requestKey);
    await page.getByRole("button", { name: "같은 변경 상태 조회" }).click();
    const current = page.getByText("현재 지급 주소 버전", { exact: true }).locator("..");
    const predecessor = phase === "start" || phase === "scheduled";
    await current.getByText(predecessor ? /^v1 · 0x/ : /^v2 · 0x/).waitFor();
    if (phase === "start") {
      await page.getByText("검토 중인 요청이 없습니다.", { exact: true }).waitFor();
      return;
    }
    await page.getByText(requestKey, { exact: true }).waitFor();
    const proposed = page.getByText("제안 지급 주소 버전", { exact: true }).locator("..");
    await proposed.getByText(/^v2 · 0x/).waitFor();
    await page.getByText("지점 통지 확인 기록", { exact: true }).waitFor();
    await page.getByText("효력 예정", { exact: true }).waitFor();
    const quotes = page.getByRole("region", { name: "기발급 견적 처리" });
    const old = quotes.getByRole("article").filter({ hasText: retained });
    const unused = quotes.getByRole("article").filter({ hasText: expired });
    if (phase === "scheduled") {
      await old.getByText("지급 결과 확인 중", { exact: true }).waitFor();
      await unused.getByText("지급 결과 확인 중", { exact: true }).waitFor();
      assert.equal(await page.getByText("등록부 효력 시작", { exact: true }).count(), 0);
    } else {
      await page.getByText("등록부 효력 시작", { exact: true }).waitFor();
      await page.getByText("확정 등록 블록", { exact: true }).waitFor();
      await old.getByText("발급 당시 지급 주소 유지", { exact: true }).waitFor();
      await unused.getByText("새 등록부에서 기존 견적 지급 거절", { exact: true }).waitFor();
    }
    if (phase === "outcomes") {
      await old.getByText("기존 주소로 지급 확정", { exact: true }).waitFor();
      await old.getByText(/^원지급 거래 0x/).waitFor();
      await unused.getByText("확정 근거로 미지급 만료", { exact: true }).waitFor();
      await unused.getByText(/^미지급 확정 근거 /).waitFor();
    }
    const updated = page.waitForResponse((reply) => reply.url().includes("requestKey=" + requestKey));
    await page.getByRole("button", { name: "이 변경 요청 다시 조회" }).click();
    assert.equal((await updated).status(), 200);
    await page.getByText(requestKey, { exact: true }).waitFor();
    console.log("registry version and original quote disposition verified");
  } finally {
    await browser.close();
  }
}
exercise().catch((error) => {
  console.error(String(error.message).replace(/0x[0-9a-f]+|[a-f0-9-]{36}/gi, "[fixture]"));
  process.exitCode = 1;
});

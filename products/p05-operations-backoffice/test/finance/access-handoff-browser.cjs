"use strict";

const assert = require("node:assert/strict");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const [origin, branch, target, original, recipient, role, phase, key, certPath, keyPath] =
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
    const btn = (name) => page.getByRole("button", { name, exact: true });
    await page.goto(origin + "/ops/");
    await page.getByLabel("담당 지점").selectOption(branch);

    if (phase === "cancel" || phase === "propose") {
      await page.getByLabel("검증된 대상 참조").fill(target);
      await page.getByLabel("역할", { exact: true }).selectOption(role);
      await page.getByLabel("변경", { exact: true }).selectOption("revoke");
      await btn("자격 변경 검토").click();
      const preview = page.getByRole("region", { name: "자격 회수 검토" });
      await preview.getByText(original, { exact: true }).waitFor();
      assert.equal(await btn("검토한 요청 기록").isEnabled(), false);
      await preview.getByLabel("인계 담당자 참조").fill(recipient);
      await btn("인계 담당 확인").click();
      await preview.getByText("확인한 지점 담당자 " + recipient, { exact: true }).waitFor();
      if (phase === "cancel") {
        await preview.getByRole("button", { name: "취소" }).click();
        await preview.waitFor({ state: "hidden" });
      } else {
        let originalPayload;
        await page.route("**/propose", async (route) => {
          const accepted = await route.fetch();
          assert.equal(accepted.status(), 200);
          originalPayload = route.request().postDataJSON();
          await route.abort("connectionreset");
        }, { times: 1 });
        const lost = page.waitForEvent("requestfailed", (request) => request.url().endsWith("/propose"));
        await btn("검토한 요청 기록").click();
        await lost;
        await page.getByRole("status").filter({ hasText: /fetch|network|확인/i }).waitFor();
        const reply = page.waitForResponse((r) => r.url().endsWith("/propose"));
        await btn("검토한 요청 기록").click();
        const replay = await reply;
        assert.equal(replay.status(), 200);
        assert.deepEqual(replay.request().postDataJSON(), originalPayload);
        await page.getByRole("article").filter({ hasText: target }).getByText(/별도 승인 대기/).waitFor();
      }
    } else if (phase === "approve") {
      await btn("같은 요청 상태 조회").click();
      const row = page.getByRole("article").filter({ hasText: key });
      await row.getByRole("button", { name: "독립 승인 검토" }).click();
      const approval = page.getByRole("region", { name: "자격 독립 승인" });
      await approval.getByText(original, { exact: true }).waitFor();
      await approval.getByText("인계 담당 " + recipient, { exact: true }).waitFor();
      await btn("현재 계정으로 승인").click();
      await row.getByText(/권한 변경 확인/).waitFor();
    } else if (phase === "receive") {
      const own = page.getByRole("region", { name: "내가 인계받은 원주문" });
      await own.getByRole("button", { name: "인계 주문 조회" }).click();
      const row = own.getByRole("article").filter({ hasText: original });
      await row.getByText(new RegExp("회수 요청 " + key)).waitFor();
      await row.getByRole("button", { name: "이 원주문 인계 접수" }).click();
      await row.getByText("원주문 인계 접수됨", { exact: true }).waitFor();
      await page.reload();
      await page.getByLabel("담당 지점").selectOption(branch);
      await own.getByRole("button", { name: "인계 주문 조회" }).click();
      await row.getByText("원주문 인계 접수됨", { exact: true }).waitFor();
    }
    console.log("scoped original order and request verified");
  } finally {
    await browser.close();
  }
}
exercise().catch((error) => {
  console.error(String(error.message).replace(/[a-f0-9-]{36}/gi, "[fixture]"));
  process.exitCode = 1;
});

"use strict";

const assert = require("node:assert/strict");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const [origin, branch, sale, certPath, keyPath, , phase, hash] = process.argv.slice(2);

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
    await page.getByRole("button", { name: "v2 지급 증거" }).click();
    await page.getByLabel("담당 지점").selectOption(branch);

    if (phase === "profile" || phase === "invalid") {
      await page.getByRole("button", { name: "배포 프로필 검증" }).click();
      const profile = page.getByRole("region", { name: "배포 프로필", exact: true });
      if (phase === "profile") {
        for (const label of ["서명된 배포 프로필 확인", "허용 TestUSDC", "ABI 해시",
          "배포 기준 블록", "등록 수취 주소 버전", "가맹점 릴레이 가스 부담자"]) {
          await profile.getByText(label, { exact: true }).waitFor();
        }
        await profile.getByText("v1", { exact: true }).waitFor();
        return;
      }
      await profile.getByText("배포 설정을 사용할 수 없습니다", { exact: true }).waitFor();
      await profile.getByText(/결제 제출을 중단했습니다/).waitFor();
    }

    await page.getByLabel("지급 증거 원주문").fill(sale);
    await page.getByRole("button", { name: "같은 원주문 증거 조회" }).click();
    const evidence = page.getByRole("region", { name: "등록된 지급 증거" });
    if (phase === "invalid") {
      await evidence.getByText("배포 또는 지급 근거 불일치", { exact: true }).waitFor();
      assert.equal(await evidence.getByText(/0\.00 TestUSDC|이 시도의 주문 소비/).count(), 0);
    } else {
      const reverted = evidence.getByRole("region", { name: "검증된 원복 거래" });
      await reverted.getByText(new RegExp(hash)).waitFor();
      await reverted.getByText("0.00 TestUSDC", { exact: true }).waitFor();
      await reverted.getByText("없음", { exact: true }).waitFor();
      for (const label of ["가스 사용량", "실효 가스 단가", "가스 비용", "가스 부담자"]) {
        await reverted.getByText(label, { exact: true }).waitFor();
      }
      await evidence.getByText("6.50 TestUSDC", { exact: true }).waitFor();
      if (phase === "paid") {
        await evidence.getByText("독립 확정 근거 일치", { exact: true }).waitFor();
        await evidence.getByText(/^확정 거래 0x/).waitFor();
      } else {
        assert.equal(await evidence.getByText(/^확정 거래 0x/).count(), 0);
      }
    }
    console.log("enrolled scope and separate exact attempt evidence verified");
  } finally {
    await browser.close();
  }
}
exercise().catch((error) => {
  console.error(String(error.message).replace(/0x[0-9a-f]+|[a-f0-9-]{36}/gi, "[fixture]"));
  process.exitCode = 1;
});

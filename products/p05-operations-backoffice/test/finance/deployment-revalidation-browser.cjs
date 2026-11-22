"use strict";

const assert = require("node:assert/strict");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const [origin] = process.argv.slice(2);
const branch = "41000000-0000-4000-8000-000000000001";
const sale = "41000000-0000-4000-8000-000000000002";
const hash = (digit) => "0x" + digit.repeat(64);

async function exercise() {
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  try {
    const page = await browser.newPage();
    let nowInvalid = false;
    const profile = {
      status: "verified",
      chain: 8283,
      contract: "0x" + "a".repeat(40),
      asset: "0x" + "b".repeat(40),
      manifest: hash("c"),
      abi: hash("d"),
      runtimeCodeHash: hash("e"),
      gasPayer: "0x" + "f".repeat(40),
    };
    const evidence = {
      ...profile,
      status: "verified",
      amount: "6500000",
      transaction: hash("1"),
      block: hash("2"),
      log: 1,
      finalizedHeight: 30,
      revertedAttempt: {
        transaction: hash("3"), block: hash("4"), blockHeight: 10, transactionIndex: 0,
        gasUsed: 12000, effectivePriceWei: "2", feeWei: "24000", gasPayer: profile.gasPayer,
        principalDelta: "0", orderConsumed: false,
      },
    };
    await page.route("**/v2/finance/scopes", (route) => route.fulfill({
      json: [{ id: branch, label: "시험 지점", timezone: "Asia/Seoul", capabilities: ["reconcile_read"] }],
    }));
    await page.route("**/v2/finance/deployment-profile", (route) => {
      assert.deepEqual(route.request().postDataJSON(), { branchId: branch });
      return route.fulfill({ json: nowInvalid ? { status: "invalid" } : profile });
    });
    await page.route("**/v2/finance/payment-evidence", (route) => {
      assert.deepEqual(route.request().postDataJSON(), { branchId: branch, saleId: sale });
      return route.fulfill({ json: nowInvalid ? { status: "invalid", deploymentBlocked: true } : evidence });
    });

    await page.goto(origin + "/ops/");
    await page.getByRole("button", { name: "v2 지급 증거" }).click();
    await page.getByLabel("담당 지점").selectOption(branch);
    await page.getByLabel("지급 증거 원주문").fill(sale);
    await page.getByRole("button", { name: "같은 원주문 증거 조회" }).click();
    const paymentView = page.getByRole("region", { name: "등록된 지급 증거" });
    const deploymentView = page.getByRole("region", { name: "배포 프로필", exact: true });
    await paymentView.getByText("6.50 TestUSDC", { exact: true }).waitFor();
    await paymentView.getByRole("region", { name: "검증된 원복 거래" }).getByText("24000 wei").waitFor();

    nowInvalid = true;
    await page.getByRole("button", { name: "배포 프로필 검증" }).click();
    await deploymentView.getByText("배포 설정을 사용할 수 없습니다").waitFor();
    assert.equal(await paymentView.count(), 0, "invalid profile left the old payment on screen");

    nowInvalid = false;
    await page.getByRole("button", { name: "배포 프로필 검증" }).click();
    await deploymentView.getByText("서명된 배포 프로필 확인").waitFor();
    nowInvalid = true;
    await page.getByRole("button", { name: "같은 원주문 증거 조회" }).click();
    await paymentView.getByText("배포 또는 지급 근거 불일치").waitFor();
    assert.equal(await deploymentView.count(), 0, "invalid payment lookup left the old verified profile on screen");
    assert.equal(await paymentView.getByText(/6\.50 TestUSDC|24000 wei/).count(), 0);

    nowInvalid = false;
    await page.getByRole("button", { name: "같은 원주문 증거 조회" }).click();
    await paymentView.getByText("6.50 TestUSDC", { exact: true }).waitFor();
    assert.equal(await deploymentView.count(), 0);
    console.log("same-tab invalidation and fresh evidence recovery verified");
  } finally {
    await browser.close();
  }
}
exercise().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});

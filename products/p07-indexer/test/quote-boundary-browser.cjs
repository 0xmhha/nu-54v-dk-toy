'use strict';

const assert = require('node:assert/strict');
const {chromium} = require(process.env.P07_PLAYWRIGHT_MODULE || 'playwright');

async function main() {
  const env = process.env;
  const browser = await chromium.launch({headless: true, executablePath: env.P07_CHROMIUM});
  try {
    const context = await browser.newContext({
      ignoreHTTPSErrors: true,
      clientCertificates: [{origin: env.X06_URL, certPath: env.X06_CERT, keyPath: env.X06_KEY}],
    });
    const page = await context.newPage();
    page.setDefaultTimeout(8000);
    await page.goto(env.X06_URL);
    await page.locator('#deployment').selectOption(env.X06_STREAM);
    await page.getByLabel('공개 가맹점 키').fill(env.X06_MERCHANT);
    await page.getByLabel('공개 주문 키').fill(env.X06_ORDER);
    await page.getByLabel('발급 지점 ID').fill(env.X06_BRANCH);
    await page.getByLabel('발급 원주문 ID').fill(env.X06_SALE);
    const reply = page.waitForResponse((response) => response.url().endsWith('/v2/explorer/boundary') && response.request().method() === 'POST');
    await page.getByRole('button', {name:'관측 증거 조회'}).click();
    const response = await reply;
    assert.equal(response.status(), 200);
    const result = await response.json();
    assert.equal(result.kind, env.X06_PHASE);
    await page.getByRole('heading', {name:'발급 원본과 연속 확정 범위'}).waitFor();
    await page.getByText('마지막 검증 확정 블록 시각', {exact:true}).waitFor();
    if (env.X06_PHASE === 'pending') {
      await page.getByText(/확정 범위가 견적 마감에 닿지 않았습니다.*예약·재고 해제 보류.*금융 조치는 0건/).waitFor();
      assert.ok(new Date(result.coverage.time) < new Date(result.quoteExpiresAt));
      assert.equal(await page.getByText('미소비 증거 참조', {exact:true}).count(), 0);
    } else {
      await page.getByRole('heading', {name:'완전 구간의 미소비 증거'}).waitFor();
      assert.ok(new Date(result.coverage.time) >= new Date(result.quoteExpiresAt));
      assert.match(result.evidence.proof, /^sha256:[0-9a-f]{64}$/);
    }
  } finally {
    await browser.close();
  }
}

main().catch((error) => { console.error(error); process.exitCode = 1; });

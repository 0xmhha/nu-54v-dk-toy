'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const {chromium} = require('playwright');

let stage = 'launch';
process.on('unhandledRejection', () => { console.error('W24_STAGE '+stage); process.exit(1); });

async function main() {
  const fixture = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
  const browser = await chromium.launch({channel: 'chrome', headless: true});
  try {
    const context = await browser.newContext({ignoreHTTPSErrors: true});
    const page = await context.newPage();
    const status = fixture.origin+'/status?wallet='+fixture.wallet;
    async function signIn() {
      stage='realm_start';
      await page.goto(fixture.origin);
      const [issued] = await Promise.all([
        page.waitForResponse(res => res.url()===fixture.origin+'/login' && res.request().method()==='POST'),
        page.getByRole('button',{name:'별도 복구 운영자 인증'}).click(),
      ]);
      stage='realm_issue_'+issued.status();
      assert.equal(issued.status(),200);
      stage='realm_link';
      const link = page.getByRole('link',{name:'독립 운영자 확인 열기'});
      await link.waitFor();
      stage='realm_open';
      const [realm] = await Promise.all([
        context.waitForEvent('page'),
        link.click(),
      ]);
      await realm.locator('input[name=user]').fill(fixture.username);
      await realm.locator('input[name=password]').fill(fixture.password);
      stage='realm_confirm';
      await realm.getByRole('button',{name:'Confirm'}).click();
      await realm.getByText(/Confirmation recorded/).waitFor();
      await realm.close();
      stage='realm_poll';
      await page.getByRole('button',{name:'인증 결과 확인'}).click();
    }
    stage='unauthenticated';
    assert.equal((await page.goto(status)).status(),401);
    stage='sign_in';
    await signIn();
    stage='select_wallet';
    await page.getByRole('textbox',{name:'지갑 참조'}).fill(fixture.wallet);
    fs.writeFileSync(fixture.outage,'1',{mode:0o600});
    await page.getByRole('button',{name:'서비스 상태 조회'}).click();
    stage='degraded_probe';
    await page.getByText(/참여자 B — 응답 미확인/).waitFor();
    stage='outage_view';
    await page.getByRole('link',{name:'서비스 장애'}).click();
    await page.getByText(/A\+C 우회 — 사용 불가/).waitFor();
    fs.rmSync(fixture.outage);
    await page.getByRole('link',{name:'서비스 재조회'}).click();
    stage='restored_probe';
    await page.getByText(/참여자 B — 응답 확인/).waitFor();
    stage='cross_wallet';
    assert.equal((await page.goto(fixture.origin+'/status?wallet='+fixture.other)).status(),403);
    await page.goto(status);
    stage='breach_view';
    await page.getByRole('link',{name:'정족수 유출 신고'}).click();
    await page.getByRole('button',{name:'신고 및 동결'}).click();
    assert.equal(new URL(page.url()).pathname,'/status');
    stage='explicit_confirmation';
    await page.getByRole('checkbox').check();
    await page.getByRole('button',{name:'신고 및 동결'}).click();
    stage='lost_response';
    await page.getByText(/동일 지갑을 다시 조회/).waitFor();
    await page.goto(status+'&view=breach');
    stage='freeze_view';
    await page.getByText(/과거 키 노출 위험 유지/).waitFor();
    await page.getByText('자동 자산 이동 · 0건').waitFor();
    await page.getByRole('link',{name:'감사 기록'}).click();
    stage='audit_view';
    await page.getByText(/incident_hold/).waitFor();
    stage='revocation';
    fs.writeFileSync(fixture.revoked,'1',{mode:0o600});
    assert.equal((await page.goto(status)).status(),401);
    fs.rmSync(fixture.revoked);
    await page.getByRole('button',{name:'별도 복구 운영자 인증'}).waitFor();
    await signIn();
    await page.goto(status+'&view=breach');
    await page.getByText(/과거 키 노출 위험 유지/).waitFor();
    assert.equal(await page.getByRole('button',{name:'신고 및 동결'}).count(),0);
  } finally {
    await browser.close();
  }
}

main().catch(() => { console.error('W24_STAGE '+stage); process.exitCode = 1; });

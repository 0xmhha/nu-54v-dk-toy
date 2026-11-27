'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {chromium} = require(process.env.P07_PLAYWRIGHT_MODULE || 'playwright');
const env = process.env;

async function main() {
  const browser = await chromium.launch({headless: true, executablePath: env.P07_CHROMIUM});
  try {
    const publicPage = await browser.newPage({viewport: {width: 390, height: 844}});
    const errors = [];
    publicPage.on('pageerror', (error) => errors.push(error.message));
    const deepLink = (stream, order) => env.EXPLORER_PUBLIC + '/?' + new URLSearchParams({
      stream, merchant: env.EXPLORER_MERCHANT, order,
    });
    await publicPage.goto(deepLink(env.EXPLORER_STREAM, env.EXPLORER_ORDER));
    const context = await browser.newContext({
      ignoreHTTPSErrors: true,
      clientCertificates: [{origin: env.EXPLORER_OPS, certPath: env.EXPLORER_CERT, keyPath: env.EXPLORER_KEY}],
    });
    const ops = await context.newPage();
    ops.on('pageerror', (error) => errors.push(error.message));
    await ops.goto(env.EXPLORER_OPS);
    await ops.getByText('허용된 배포 2개').waitFor();

    if (env.EXPLORER_PHASE === 'replay') {
      const start = ops.getByRole('button', {name:'격리 재생 요청'});
      const refresh = ops.getByRole('button', {name:'작업 상태 조회'});
      const selector = ops.getByLabel('재생 배포');
      const startJob = async () => {
        await start.waitFor();
        const reply = ops.waitForResponse((r) => r.url().endsWith('/v2/explorer/replays') && r.request().method() === 'POST');
        await start.click();
        const response = await reply;
        assert.equal(response.status(), 200);
        return response.request().postDataJSON();
      };
      const waitForState = async (label) => {
        for (let attempt = 0; attempt < 30; attempt++) {
          await refresh.click();
          try {
            await ops.getByRole('heading', {name:label}).waitFor({timeout:500});
            return;
          } catch (_) {
            // The worker and its database commit are external to the UI.
          }
        }
        throw new Error('Replay state not rendered: ' + label);
      };
      await selector.selectOption(env.EXPLORER_STREAM);
      const first = await startJob();
      await ops.getByRole('heading', {name:'재생 중'}).waitFor();
      await ops.reload();
      await selector.selectOption(env.EXPLORER_STREAM);
      await ops.getByText(first.id, {exact:true}).waitFor();
      const jobQuery = '/v2/explorer/replays?' + new URLSearchParams({stream:env.EXPLORER_STREAM});
      const job = async (id) => (await (await context.request.get(env.EXPLORER_OPS + jobQuery)).json()).jobs.find((item) => item.id === id);
      const running = await job(first.id);
      assert.equal(running.state, 'running');
      for (const field of ['reason','comparison','differingLogs','paidDelta','refundDelta','replayedHeight']) {
        assert.equal(Object.hasOwn(running, field), false);
      }
      assert.equal((await publicPage.request.get(env.EXPLORER_PUBLIC + jobQuery)).status(), 404);
      const retry = await context.request.post(env.EXPLORER_OPS + '/v2/explorer/replays', {data:first});
      assert.equal(retry.status(), 200);
      assert.equal((await retry.json()).id, first.id);
      fs.writeFileSync(path.join(env.EXPLORER_REPLAY_SYNC, 'first'), '');
      await waitForState('검증 완료 · 기존 전환 권한으로 승인 요청 가능');
      const verified = await job(first.id);
      assert.deepEqual([verified.differingLogs,verified.paidDelta,verified.refundDelta],[0,'0','0']);
      assert.equal(verified.replayedHeight,verified.anchor);
      assert.ok(verified.comparison);
      const second = await startJob();
      await ops.getByRole('heading', {name:'재생 중'}).waitFor();
      fs.writeFileSync(path.join(env.EXPLORER_REPLAY_SYNC, 'second'), '');
      await waitForState('불일치 · 기존 조회 유지');
      await ops.getByText('로그 또는 금액 차이', {exact:true}).waitFor();
      const mismatch = await job(second.id);
      assert.deepEqual([mismatch.differingLogs,mismatch.paidDelta,mismatch.refundDelta],[2,'2','0']);
      assert.equal(Object.hasOwn(mismatch,'comparison'),false);
      await publicPage.reload();
      await publicPage.getByRole('heading', {name:'확정 가맹점 지급'}).waitFor();
      await selector.selectOption(env.EXPLORER_LEGACY);
      await start.waitFor({state:'hidden'});
      const denied = await context.request.post(env.EXPLORER_OPS + '/v2/explorer/replays', {data:{...first, stream:env.EXPLORER_LEGACY}});
      assert.equal(denied.status(), 404);
      const closed = await context.request.post(env.EXPLORER_OPS + '/v2/explorer/replays/switch', {data:first});
      assert.equal(closed.status(), 404);
    } else if (env.EXPLORER_PHASE === 'stale') {
      await publicPage.getByRole('heading', {name:'확정 가맹점 지급'}).waitFor();
      await publicPage.getByText(/최신 체인 확인 중.*마지막 관측/).waitFor();
      await publicPage.getByLabel('공개 주문 키', {exact:true}).fill(env.EXPLORER_ABSENCE);
      await publicPage.getByRole('button', {name:'영수증 찾기'}).click();
      await publicPage.getByText(/최신 체인 확인 중.*미지급이나 재청구를 판정할 수 없습니다/).waitFor();
      assert.equal(await publicPage.getByText('영수증이 아직 색인되지 않았습니다.', {exact:false}).count(),0);
    } else if (env.EXPLORER_PHASE === 'frozen') {
      await publicPage.getByText('확정 증거가 충돌하여 이 배포의 공개 지급 증거를 보류합니다.').waitFor();
      assert.equal(await publicPage.getByText('확정 가맹점 지급', {exact: true}).count(), 0);
      await ops.getByText('확정 충돌 조사 번호', {exact: true}).waitFor();
      await ops.getByText('이전 해시', {exact: true}).waitFor();
    } else {
      await publicPage.getByRole('heading', {name: '확정 가맹점 지급'}).waitFor();
      await publicPage.getByText('로그 인덱스', {exact: true}).first().waitFor();
      assert.doesNotMatch(await publicPage.locator('#results').innerText(), /픽업|출고|공급자 주문/);
      if (env.EXPLORER_PHASE === 'reorg') {
        await ops.getByText('최근 미확정 분기의 마지막 공통 높이').waitFor();
      } else {
        await publicPage.getByRole('heading', {name: '원지급과 누적 환불'}).waitFor();
        await publicPage.getByText('0.000030', {exact: true}).first().waitFor();
        assert(!((await publicPage.locator('#results').innerText()).includes(env.EXPLORER_RECIPIENT)));
        const release = [];
        publicPage.on('request', (request) => release.push(request.url()));
        const recipient = publicPage.getByLabel('원 견적에서 확인한 수령 주소');
        await recipient.fill('0x' + '1'.repeat(40));
        await publicPage.getByRole('button', {name: '이 기기에서 수령 주소 대조'}).click();
        await publicPage.getByText('이 주소는 해당 지급 로그의 수령 주소와 일치하지 않습니다.').waitFor();
        await recipient.fill(env.EXPLORER_RECIPIENT);
        await publicPage.getByRole('button', {name: '이 기기에서 수령 주소 대조'}).click();
        await publicPage.getByText('원 견적 주소가 확정 지급 로그의 수령 주소와 일치합니다.').waitFor();
        assert.deepEqual(release, []);
        await publicPage.getByLabel('검색 기준').selectOption('transaction');
        await publicPage.getByLabel('거래 해시', {exact: true}).fill(env.EXPLORER_TX);
        await publicPage.getByLabel('로그 인덱스 (선택)').fill(env.EXPLORER_LOG);
        await publicPage.getByRole('button', {name: '영수증 찾기'}).click();
        await publicPage.getByRole('link', {name:'같은 주문의 지급·환불 모두 보기'}).click();
        await publicPage.getByRole('heading', {name:'원지급과 누적 환불'}).waitFor();
        await publicPage.locator('#deployment').selectOption(env.EXPLORER_LEGACY);
        await publicPage.getByRole('button', {name: '영수증 찾기'}).click();
        await publicPage.getByText(/아직 색인되지 않았습니다.*미지급이나 재청구를 판정할 수 없습니다/).waitFor();
        await publicPage.getByLabel('공개 가맹점 키').fill(env.EXPLORER_LEGACY_MERCHANT);
        await publicPage.getByLabel('공개 주문 키', {exact:true}).fill(env.EXPLORER_LEGACY_ORDER);
        await publicPage.getByRole('button', {name:'영수증 찾기'}).click();
        await publicPage.getByRole('heading', {name:'확정 가맹점 적립'}).waitFor();
        assert.equal(await publicPage.getByRole('link', {name:'인가된 점주 인출 원장 열기'}).getAttribute('href'), 'https://operator.invalid/?view=v1');
        assert.equal(await publicPage.getByRole('button', {name:'이 기기에서 수령 주소 대조'}).count(),0);

        const unavailable = await publicPage.request.get(env.EXPLORER_PUBLIC + '/v2/explorer/monitor');
        assert.equal(unavailable.status(), 404);
        await ops.getByText('배포 구성을 확인하세요 · 이 구성만 수집 보류', {exact:true}).waitFor();
        await ops.getByText('미등록 ABI', {exact:true}).waitFor();
        assert.equal(await publicPage.getByText('미등록 ABI', {exact:true}).count(), 0);
        await ops.locator('#deployment').selectOption(env.EXPLORER_STREAM);
        await ops.getByLabel('공개 가맹점 키').fill(env.EXPLORER_MERCHANT);
        async function query(order, text) {
          await ops.getByLabel('공개 주문 키').fill(order);
          await ops.getByRole('button', {name: '관측 증거 조회'}).click();
          await ops.getByText(text).waitFor();
        }
        await query(env.EXPLORER_ORDER, '확정 지급의 포함·확정 시각');
        await ops.getByText('원 견적 제출 기한', {exact:true}).waitFor();
        await ops.getByText(/서명 견적의 기한 안에 포함된 지급입니다/).waitFor();
        await query(env.EXPLORER_ABSENCE, '완전 구간의 미소비 증거');
        await query('0x' + '0'.repeat(62) + '98', /관측 공백만으로 확정 미지급을 입증할 수 없습니다/);
        await ops.locator('#deployment').selectOption(env.EXPLORER_LEGACY);
        await query(env.EXPLORER_ORDER, '접근 가능한 대사 범위를 확인하세요.');
      }
    }
    assert.deepEqual(errors, []);
    assert.equal(await publicPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
    console.log('public projection and authorized operator observations are consistent');
  } finally {
    await browser.close();
  }
}

main().catch((error) => { console.error(error); process.exitCode = 1; });

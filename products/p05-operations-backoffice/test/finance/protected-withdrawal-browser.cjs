"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const { setTimeout: sleep } = require("node:timers/promises");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");

const [origin, branch, device, generation, phase, closeHash, certPath, keyPath, gate] = process.argv.slice(2);
const phases = {
  indeterminate: "현재 근거로 인출 상태를 확인할 수 없습니다",
  zero: "원래 반납 요청의 인출액은 0입니다",
  "cleanup-authenticated": "원래 반납 요청의 인출액은 0입니다",
  "cleanup-zero": "원래 반납 요청의 인출액은 0입니다",
  "cleanup-received": "등록한 주소의 정본 수령 확인",
  delay: "지연 인출 대기",
  held: "등록한 수취인의 수령 대기",
  ready: "등록한 주소로 인출 가능",
  received: "등록한 주소의 정본 수령 확인",
  revoked: "등록한 주소의 정본 수령 확인",
  "stale-paid": "원요청 기록됨 · 정본 결과 확인 중",
};

async function verify() {
  const server = await chromium.launchServer({ channel: "chrome", headless: true });
  const browser = await chromium.connect(server.wsEndpoint());
  const stop = () => { void server.kill(); };
  process.once("SIGTERM", stop);
  process.once("SIGINT", stop);
  try {
    const page = await browser.newPage({
      ignoreHTTPSErrors: true,
      clientCertificates: [{ origin, certPath, keyPath }],
      timezoneId: "Asia/Seoul",
    });
    page.setDefaultTimeout(7000);
    const button = (name) => page.getByRole("button", { name, exact: true });
    const requestPath = `/v2/operations/rental/${branch}/${device}`;
    const lookup = async () => {
      await button("대여 기기").click();
      for (const [name, value] of [["지점 참조", branch], ["기기 참조", device], ["대여 세대", generation]]) {
        await page.getByLabel(name, { exact: true }).fill(value);
      }
      const received = page.waitForResponse(r => new URL(r.url()).pathname === requestPath && r.request().method() === "GET");
      await button("같은 기기 조회").click();
      return received;
    };
    await page.goto(origin + "/ops/");
    let writes = 0;
    page.on("request", r => {
      if (new URL(r.url()).pathname.startsWith(requestPath) && r.method() === "POST") writes++;
    });
    const response = await lookup();
    if (phase === "denied") {
      assert.equal(response.status(), 404);
      const body = await response.text();
      assert.equal(body.includes(closeHash), false);
      assert.equal(await page.getByRole("region", { name: "보호 세대 인출 상태" }).count(), 0);
      assert.equal(writes, 0);
      console.log("O09_PROTECTED_FLEET " + JSON.stringify({ phase, writes }));
      return;
    }
    assert.equal(response.status(), 200);
    await page.getByRole("heading", { name: "v1 대여 계정 · fixture" }).waitFor();
    await page.getByText("기기 인증 시각", { exact: true }).waitFor();
    const scoped = await response.json();
    assert.ok(scoped.attestedAt);
    await page.getByText(new Date(scoped.attestedAt).toLocaleString("ko-KR", { timeZone: "Asia/Seoul" }), { exact: true }).waitFor();
    const section = page.getByRole("region", { name: "보호 세대 인출 상태" });
    await section.getByText(phases[phase], { exact: true }).waitFor();
    await section.getByText(`원반납 거래 ${closeHash}`, { exact: true }).waitFor();
    assert.equal(await button("보호 세대 인출 검토").count(), phase === "ready" ? 1 : 0);
    assert.equal(await button("대여 세션 정리 검토").count(), 0);
    if (phase === "delay") await section.getByText(/인출 가능 시각/).waitFor();
    if (phase === "indeterminate") assert.equal(await section.getByText(/인출액.*최소 단위/).count(), 0);
    if (phase === "zero") await section.getByText("인출액 0 최소 단위", { exact: true }).waitFor();
    if (phase === "ready") await section.getByText("인출액 25 최소 단위", { exact: true }).waitFor();
    if (phase === "received" || phase === "held" || phase === "revoked") {
      const { payoutHash, acceptanceHash } = scoped.withdrawal;
      assert.match(payoutHash, /^0x[0-9a-f]{64}$/);
      await section.getByText(`인출 거래 ${payoutHash}`, { exact: true }).waitFor();
      if (phase === "held") assert.equal(acceptanceHash, undefined);
      if (acceptanceHash) {
        assert.notEqual(acceptanceHash, payoutHash);
        await section.getByText(`수령 거래 ${acceptanceHash}`, { exact: true }).waitFor();
      }
    }
    // A settlement status must identify the original return generation in its own region.
    await section.getByText(`반납 세대 · G-${generation}`, { exact: true }).waitFor();
    await section.getByText(device, { exact: true }).waitFor();
    if (phase === "cleanup-authenticated") {
      const requestKey = scoped.cleanup?.requestKey;
      assert.match(requestKey, /^[0-9a-f-]{36}$/);
      const verifyAuthenticated = async (view) => {
        assert.equal(view.status(), 200);
        const body = await view.json();
        assert.deepEqual(body.cleanup, { state: "receipt-authenticated-handoff-required", requestKey });
        assert.equal(/"receipt"\s*:|"intent"\s*:|"signedReceipt"\s*:/i.test(JSON.stringify(body)), false);
        await section.getByText("보드 영수증 인증 · 기기 재배정 승인 대기", { exact: true }).waitFor();
        await section.getByText(`정리 원요청 ${requestKey}`, { exact: true }).waitFor();
        await section.getByText(`원반납 거래 ${closeHash}`, { exact: true }).waitFor();
        assert.equal(await button("정리 원요청 접수").count(), 0);
        assert.equal(await page.getByRole("button", {name: /재배정|대여 계정 개설/}).count(), 0);
      };
      await verifyAuthenticated(response);
      await page.reload();
      await verifyAuthenticated(await lookup());
      assert.equal(writes, 0);
      console.log("O09_PROTECTED_FLEET " + JSON.stringify({ phase, writes }));
      return;
    }
    await section.getByText("기기 정리 확인 대기 · 재배정 보류", { exact: true }).waitFor();
    if (phase === "revoked" || phase === "stale-paid") {
      if (phase === "stale-paid") await button("같은 원인출 전송 재시도").waitFor();
      await fs.writeFile(gate + ".ready", "", { mode: 0o600 });
      for (let n = 0; n < 100; n++) {
        try {
          await fs.access(gate + ".revoked");
          break;
        } catch (error) {
          if (n === 99) throw error;
          await sleep(100);
        }
      }
      if (phase === "revoked") {
        const denied = page.waitForResponse(r => new URL(r.url()).pathname === requestPath && r.request().method() === "GET");
        await button("같은 인출 조회").click();
        assert.equal((await denied).status(), 404);
        await page.getByText("이 지점의 등록 기기와 담당자 권한을 확인하세요.", { exact: true }).waitFor();
        assert.equal(await page.getByText(`원반납 거래 ${closeHash}`, { exact: true }).count(), 0);
        assert.equal(await section.count(), 0);
        assert.equal(await button("보호 세대 인출 검토").count(), 0);
      } else {
        const retry = page.waitForResponse(r => new URL(r.url()).pathname === requestPath + "/withdraw" && r.request().method() === "POST");
        await button("같은 원인출 전송 재시도").click();
        const replay = await retry;
        assert.equal(replay.status(), 202);
        await section.getByText(phases.received, { exact: true }).waitFor();
        await section.getByText(`원인출 요청 거래 ${scoped.withdrawal.withdrawalHash}`, { exact: true }).waitFor();
        await section.getByText("정본에서 원인출 거래 실패 확인", { exact: true }).waitFor();
        assert.equal(await button("보호 세대 인출 검토").count(), 0);
        assert.equal(writes, 1);
        console.log("O09_PROTECTED_FLEET " + JSON.stringify({ phase, writes }));
        return;
      }

    }
    if (phase === "cleanup-zero" || phase === "cleanup-received") {
      const posted = page.waitForResponse(r => new URL(r.url()).pathname === requestPath + "/cleanup" && r.request().method() === "POST");
      await button("정리 원요청 접수").click();
      const accepted = await posted;
      assert.equal(accepted.status(), 202);
      const identity = section.getByText(/^정리 원요청 [0-9a-f-]{36}$/);
      await identity.waitFor();
      const requestKey = (await identity.textContent()).slice("정리 원요청 ".length);
      const verifyPending = async () => {
        await section.getByText(`정리 원요청 ${requestKey}`, { exact: true }).waitFor();
        await section.getByText("정리 원요청 접수 · 보드 완료 영수증 대기 · 재배정 보류", { exact: true }).waitFor();
        await section.getByText(`원반납 거래 ${closeHash}`, { exact: true }).waitFor();
        if (scoped.withdrawal.payoutHash) {
          await section.getByText(`인출 거래 ${scoped.withdrawal.payoutHash}`, { exact: true }).waitFor();
        }
        assert.equal(await button("정리 원요청 접수").count(), 0);
      };
      await verifyPending();
      await page.reload();
      const reopened = await lookup();
      assert.equal(reopened.status(), 200);
      await verifyPending();
      assert.equal(writes, 1);
      console.log("O09_PROTECTED_FLEET " + JSON.stringify({ phase, writes, requestKey }));
      return;
    }
    let requestKey;
    if (phase === "ready") {
      await button("보호 세대 인출 검토").click();
      const review = page.getByRole("region", { name: "대여 작업 검토" });
      await review.getByText(closeHash, { exact: true }).waitFor();
      await review.getByText("등록한 인출 주소", { exact: true }).waitFor();
      await review.getByText("25", { exact: true }).waitFor();
      assert.equal(writes, 0);
      const posted = page.waitForResponse(r => new URL(r.url()).pathname === requestPath + "/withdraw" && r.request().method() === "POST");
      await review.getByRole("button", { name: "요청 기록", exact: true }).click();
      const result = await posted;
      assert.equal(result.status(), 202);
      const operation = (await result.json()).withdrawal;
      requestKey = operation.requestKey;
      assert.match(requestKey, /^[0-9a-f-]{36}$/);
      assert.equal(operation.closeHash, closeHash);
      assert.equal(writes, 1);
    } else {
      assert.equal(writes, 0);
    }
    console.log("O09_PROTECTED_FLEET " + JSON.stringify({ phase, writes, requestKey }));
  } catch (error) {
    console.error(error);
    process.exitCode = 1;
  } finally {
    process.off("SIGTERM", stop);
    process.off("SIGINT", stop);
    const closed = await Promise.race([server.close().then(() => true), sleep(5000, false, { ref: false })]);
    if (!closed) await server.kill();
  }
}
verify().catch(error => { console.error(error); process.exitCode = 1; });

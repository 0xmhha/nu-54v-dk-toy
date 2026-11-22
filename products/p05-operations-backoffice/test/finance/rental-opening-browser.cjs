"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const { setTimeout: delay } = require("node:timers/promises");
const { chromium } = require(process.env.NU54_PLAYWRIGHT_MODULE || "playwright");
const [origin, branch, device, account, destination, phase, certPath, keyPath, attested, key, hash, block] =
  process.argv.slice(2);

async function exercise() {
  const browser = await chromium.launch({ channel: "chrome", headless: true });
  try {
    const page = await browser.newPage({
      ignoreHTTPSErrors: true,
      clientCertificates: [{ origin, certPath, keyPath }],
      timezoneId: "Asia/Seoul",
      viewport: { width: 1440, height: 900 },
    });
    page.setDefaultTimeout(12000);
    const button = (name) => page.getByRole("button", { name, exact: true });
    async function load(generation = "17") {
      await page.goto(origin + "/ops/");
      await button("대여 기기").click();
      for (const [name, value] of [["지점 참조", branch], ["기기 참조", device], ["대여 세대", generation]]) {
        await page.getByLabel(name).fill(value);
      }
      await button("같은 기기 조회").click();
    }
    const identity = page.getByRole("region", { name: "등록 대여 기기" });
    const reviewIdentity = page.getByRole("region", { name: "대여 작업 검토" });
    async function checkIdentity(region, expectedRevision = "nu54_rev1") {
      for (const value of [branch, device, "17", expectedRevision, account]) {
        await region.getByText(value, { exact: true }).waitFor();
      }
      if (expectedRevision === "nu54_rev1") {
        await region.getByText("NU-54V-DK", { exact: true }).waitFor();
        const time = region.locator("time");
        assert.equal(Date.parse(await time.getAttribute("datetime")), Date.parse(attested));
        const local = await page.evaluate(at => new Date(at).toLocaleString("ko-KR"), attested);
        assert.equal(await time.innerText(), local);
      } else {
        await region.getByText("기록 없음", { exact: true }).waitFor();
        assert.equal(await region.getByText("NU-54V-DK", { exact: true }).count(), 0);
        assert.equal(await region.locator("time").count(), 0);
      }
    }
    if (phase.startsWith("start-")) {
      let writes = 0;
      page.on("request", req => {
        if (req.method() === "POST" && new URL(req.url()).pathname.startsWith("/v2/operations/"))
          writes++;
      });
      await load();
      await checkIdentity(identity, phase === "start-legacy" ? "fixture-unlisted" : undefined);
      if (phase === "start-legacy") {
        assert.equal(writes, 0);
        console.log("unlisted revision and absent historical attestation rendered");
        return;
      }

      const active = phase === "start-active";
      const reviewButton = button(active ? "이 세대 반납 검토" : "대여 계정 개설 검토");
      if (active) {
        await page.getByRole("region", { name: "원래 대여 개설 결과" }).getByText(block, { exact: true }).waitFor();
      } else {
        await page.getByLabel("TestUSDC 예치금").fill("0.000025");
      }
      await reviewButton.click();
      await checkIdentity(reviewIdentity);
      await button("닫기").click();
      await reviewIdentity.waitFor({ state: "hidden" });
      if (!active) {
        await page.getByLabel("대여 세대").fill("18");
        await identity.waitFor({ state: "hidden" });
        assert.equal(await reviewButton.count(), 0);
        const denied = page.waitForResponse(response =>
          new URL(response.url()).pathname === "/v2/operations/rental/" + branch + "/" + device &&
          new URL(response.url()).searchParams.get("generation") === "18");
        await button("같은 기기 조회").click();
        assert.equal((await denied).status(), 404);
        await page.getByRole("status").filter({ hasText: /권한/ }).waitFor();
        assert.equal(await identity.count(), 0);
        assert.equal(await page.getByText(account, { exact: true }).count(), 0);
        assert.equal(await page.getByText("NU-54V-DK", { exact: true }).count(), 0);
        assert.equal(await page.locator("time").count(), 0);
        assert.equal(await reviewButton.count(), 0);
        await page.getByLabel("대여 세대").fill("17");
        await button("같은 기기 조회").click();
        await checkIdentity(identity);
      }
      assert.equal(writes, 0);
      console.log(active ? "same finalized account, return review cancelled, writes=0"
        : "original opening identity restored after rejected scope, writes=0");
      return;
    }
    const opening = page.getByRole("region", { name: "원래 대여 개설 결과" });
    if (phase.startsWith("qualified-")) {
      const seen = [];
      page.on("response", response => {
        if (/\/v2\/operations\/(rental|legacy)\//.test(new URL(response.url()).pathname))
          seen.push({method: response.request().method(), status: response.status()});
      });
      await load();
      const review = page.getByRole("region", {name: "대여 작업 검토"});
      const openReview = () => button("이 세대 반납 검토").click();
      await openReview();
      await review.getByText(new RegExp("17 · " + destination, "i")).waitFor();
      await review.getByText(account, {exact: true}).waitFor();
      await review.getByText("확정 계정의 반환 목적지와 일치", {exact: true}).waitFor();
      const labels = {
        "qualified-two": "미결 2건 · 지급 1건 · 한도 변경 1건",
        "qualified-one": "미결 1건 · 지급 1건 · 한도 변경 0건",
        "qualified-zero": "미결 0건 · 지급 0건 · 한도 변경 0건",
      };
      const label = labels[phase];
      assert.ok(label);
      await review.getByText(label, {exact: true}).waitFor();
      assert.equal(await button("요청 기록").isEnabled(), phase === "qualified-zero");
      if (phase === "qualified-one") {
        await review.getByText("건수 확인 필요", {exact: true}).waitFor({timeout: 40000});
        assert.equal(await review.getByText(label, {exact: true}).count(), 0);
        assert.equal(await button("요청 기록").isEnabled(), false);
      }
      if (phase === "qualified-two") {
        await review.getByRole("button", {name: "v1 계약 대사", exact: true}).click();
        await page.getByRole("heading", {name: "v1 적립금과 인출"}).waitFor();
        const branchPicker = page.getByLabel("담당 지점");
        await branchPicker.locator(`option[value="${branch}"]`).waitFor({state: "attached"});
        assert.equal(await branchPicker.inputValue(), branch);
        const ledger = page.waitForResponse(r => new URL(r.url()).pathname === "/v2/operations/legacy/" + branch && r.request().method() === "GET");
        await button("기존 v1 계약 조회").click();
        assert.equal((await ledger).status(), 200);
        await page.getByRole("region", {name: "v1 원장과 인출"}).waitFor();
        await button("같은 대여 기기 반납 준비").click();
        for (const [name, value] of [["지점 참조", branch], ["기기 참조", device], ["대여 세대", "17"]]) {
          assert.equal(await page.getByLabel(name).inputValue(), value);
        }
        assert.equal(await review.count(), 0);
        await button("같은 기기 조회").click();
        await openReview();
        await review.getByText(label, {exact: true}).waitFor();
      }
      await button("닫기").click();
      assert.equal(seen.filter(r => r.method === "POST").length, 0);
      if (phase === "qualified-zero") {
        await openReview();
        await review.getByText(label, {exact: true}).waitFor();
        const post = page.waitForResponse(r => r.url().includes("/close?generation=17") && r.request().method() === "POST");
        await button("요청 기록").click();
        assert.equal((await post).status(), 202);
      }
      console.log("O09_LIVE_BROWSER " + JSON.stringify({phase, label, originalGeneration:17,
        expiredOnScreen: phase === "qualified-one", writes:seen.filter(r => r.method === "POST").length,
        postCodes:seen.filter(r => r.method === "POST").map(r=>r.status)}));
      return;
    }

    if (phase.startsWith("return-")) {
      if (phase === "return-resumed") {
        let writes = 0;
        page.on("request", req => {
          if (req.method() === "POST" && new URL(req.url()).pathname.startsWith("/v2/operations/"))
            writes++;
        });
        const checkOriginal = async () => {
          await checkIdentity(identity);
          await page.getByText(new RegExp(key)).waitFor();
          await page.getByText("기록된 반납 거래 " + block, { exact: true }).waitFor();
          await page.getByText("검증된 반환 거래 " + hash, { exact: true }).waitFor();
          await page.getByText("이전 세대 반환 경로 확인 · 보드 증거 대기", { exact: true }).waitFor();
          for (const action of ["이 세대 반납 검토", "지연 인출 실행 검토", "대여 세션 정리 검토", "정리 원요청 접수"]) {
            assert.equal(await button(action).count(), 0);
          }
        };
        await load();
        await checkOriginal();
        const reloaded = page.waitForResponse(response => {
          const url = new URL(response.url());
          return response.request().method() === "GET" &&
            url.pathname === "/v2/operations/rental/" + branch + "/" + device &&
            url.searchParams.get("generation") === "17";
        });
        await button("같은 기기 조회").click();
        assert.equal((await reloaded).status(), 200);
        await checkOriginal();
        assert.equal(writes, 0);
        console.log("same scoped close and withdrawal restored on re-query; cleanup writes=0");
        return;
      }
      await load();
      if (phase === "return-withdrawal") {
        await page.getByText(new RegExp(key)).waitFor();
        await button("지연 인출 실행 검토").click();
        await button("요청 기록").click();
        await page.getByRole("status").getByText(/같은 요청의 체인/).waitFor();
        console.log("withdrawal requested on the same account after finalized delay");
        return;
      }
      const review = page.getByRole("region", { name: "대여 작업 검토" });
      let reads = 0, writes = 0;
      page.on("request", req => {
        if (!req.url().includes("/v2/operations/rental/")) return;
        if (req.method() === "GET") reads++;
        if (req.method() === "POST") writes++;
      });
      await button("이 세대 반납 검토").click();
      for (const text of ["원 대여 계정", account,
        "확정 계정의 반환 목적지와 일치", "건수 확인 필요"]) {
        await review.getByText(text, { exact: true }).waitFor();
      }
      await review.getByText(new RegExp("17 · " + destination, "i")).waitFor();
      assert.equal(reads, 1);
      assert.doesNotMatch(await review.innerText(), /0\s*건/);
      if (phase === "return-blocked") {
        await review.getByText("대사 필요", { exact: true }).waitFor();
        assert.equal(await button("요청 기록").isDisabled(), true);
        await button("닫기").click();
        assert.equal(writes, 0);
        console.log("expired scoped reconciliation evidence holds the original account without creating a command");
        return;
      }
      await review.getByText(/^유효 기한 · /).waitFor();
      await button("닫기").click();
      assert.equal(writes, 0);
      await button("이 세대 반납 검토").click();
      await review.getByText(/^유효 기한 · /).waitFor();
      if (phase === "return-stale") {
        await page.route("**/close?generation=17", async route => {
          await fs.writeFile(block + ".reviewed", "");
          for (let pending = 0; pending < 100; pending++) {
            try {
              await fs.access(block + ".changed");
              break;
            } catch {
              await delay(50);
            }
          }
          await fs.access(block + ".changed");
          const reply = await route.fetch();
          assert.equal(reply.status(), 409);
          await route.fulfill({ response: reply });
        }, {times: 1});
        await button("요청 기록").click();
        await page.getByRole("status").filter({hasText: /보류/}).waitFor();
        await button("닫기").click();
        assert.equal(writes, 1);
        console.log("clearance changed after Chrome review; live submit was held");
        return;
      }
      let command;
      await page.route("**/close?generation=17", async route => {
        const reply = await route.fetch();
        assert.equal(reply.status(), 202);
        command = route.request().postDataJSON();
        assert.match(command.reviewRef, /^[0-9a-f]{64}$/);
        await route.abort("connectionreset");
      }, {times: 1});
      await button("요청 기록").click();
      await page.getByRole("status").filter({hasText: /fetch|network|조회/i}).waitFor();
      await load();
      await page.getByText(new RegExp(command.requestKey)).waitFor();
      assert.equal(await button("이 세대 반납 검토").count(), 0);
      assert.equal(writes, 1);
      console.log("fresh authenticated return review, cancelled zero, exactly one saved close after response loss");
      return;
    }

    if (phase === "confirmed") {
      await load();
      await page.getByText(new RegExp(key)).waitFor();
      await opening.getByText("개설 예치 거래 " + hash, { exact: true }).waitFor();
      await opening.getByText("개설 예치 확정 블록 높이", { exact: true }).waitFor();
      await opening.getByText(block, { exact: true }).waitFor();
      assert.equal(await button("대여 계정 개설 검토").count(), 0);
      console.log("same request and finalized opening block restored");
      return;
    }

    await load("18");
    await page.getByRole("status").filter({ hasText: /권한/ }).waitFor();
    assert.equal(await button("대여 계정 개설 검토").count(), 0);
    await load();
    await page.getByText("검증된 장비 · 예치 대기", { exact: true }).waitFor();
    await page.getByLabel("TestUSDC 예치금").fill("0.000025");
    const review = page.getByRole("region", { name: "대여 작업 검토" });
    let writes = 0;
    page.on("request", (req) => {
      if (req.method() === "POST" && req.url().includes("/open?")) writes++;
    });
    await button("대여 계정 개설 검토").click();
    const attestedText = await page.evaluate((at) => new Date(at).toLocaleString("ko-KR"), attested);
    for (const value of [account, "0.000025 TestUSDC", "0.00004 TestUSDC", "0.00002 TestUSDC", attestedText]) {
      await review.getByText(value, { exact: true }).waitFor();
    }
    await review.getByText(new RegExp("17 · " + destination, "i")).waitFor();
    await button("닫기").click();
    assert.equal(writes, 0);
    await button("대여 계정 개설 검토").click();
    await review.getByText("요청 예치금", { exact: true }).waitFor();
    let command;
    await page.route("**/open?generation=17", async (route) => {
      const reply = await route.fetch();
      assert.equal(reply.status(), 202);
      command = route.request().postDataJSON();
      assert.equal(command.amount, "25");
      assert.match(command.reviewRef, /^[0-9a-f]{64}$/);
      await route.abort("connectionreset");
    }, { times: 1 });
    await button("요청 기록").click();
    await page.getByRole("status").filter({ hasText: /fetch|network|조회/i }).waitFor();
    await load();
    await page.getByText(new RegExp(command.requestKey)).waitFor();
    await opening.getByText(/^개설 예치 거래 0x/).waitFor();
    await opening.getByText("개설 예치 확정 대기", { exact: true }).waitFor();
    assert.equal(writes, 1);
    assert.equal(await button("대여 계정 개설 검토").count(), 0);
    console.log("amount, contract limits and verified time reviewed; cancelled zero, recovered one same request");
  } finally {
    await browser.close();
  }
}
exercise().catch((error) => {
  console.error(String(error.message).replace(/0x[0-9a-f]+|[a-f0-9-]{36}/gi, "[fixture]"));
  process.exitCode = 1;
});

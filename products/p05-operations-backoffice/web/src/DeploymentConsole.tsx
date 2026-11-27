import { useState } from "react";
import { BranchPicker, useAuthorizedBranches } from "./BranchPicker";
import { financeRequest, formatFinanceAmount as money } from "./Finance";
import { replyToken, type Branded } from "./money";

type DeploymentReview = {
  status: "verified" | "unverified" | "invalid";
  chain?: number;
  contract?: string;
  asset?: string;
  manifest?: string;
  abi?: string;
  runtimeCodeHash?: string;
  startHeight?: number;
  startHash?: string;
  gasPayer?: string;
  beneficiaryVersion?: number;
};
type RevertedAttempt = {
  transaction: string;
  block: string;
  blockHeight: number;
  transactionIndex: number;
  gasUsed: number;
  effectivePriceWei: string;
  feeWei: string;
  gasPayer: string;
  principalDelta: string;
  orderConsumed: boolean;
};
type PaymentEvidence = {
  status: string;
  chain?: number;
  contract?: string;
  asset?: string;
  manifest?: string;
  abi?: string;
  runtimeCodeHash?: string;
  deploymentBlocked?: boolean;
  revertedAttempt?: RevertedAttempt;
  beneficiary?: string;
  beneficiaryVersion?: number;
  amount?: string;
  transaction?: string;
  block?: string;
  log?: number;
  finalizedHeight?: number;
  attempt?: string;
  quoteIssuedAt?: string;
  quoteExpiresAt?: string;
  quoteDigest?: string;
  includedAt?: string;
  attestedAt?: string;
  includedBeforeExpiry?: boolean;
} & Branded;
type DeploymentResult =
  | { kind: "profile"; branch: string; profile: DeploymentReview }
  | { kind: "payment"; branch: string; sale: string; evidence: PaymentEvidence };
type LegacyReview = {
  reference: string;
  version: "v1";
  contract: string;
  asset: string;
  payout: string;
  balance: string;
};
type Legacy = {
  ledger: {
    contract: string;
    asset: string;
    merchant: string;
    registeredPayout: string;
    balance: string;
  };
  canWithdraw: boolean;
  canResume: boolean;
  review?: LegacyReview;
  version: "v1";
  outcome?: {
    status: string;
    transaction?: string;
    block?: string;
    payout?: string;
    amount?: string;
    acceptedTransaction?: string;
  };
};

const uuid = /^[0-9a-f-]{36}$/;
const status: Record<string, string> = {
  pending: "기존 거래 확정 대기",
  held: "독립 확정 근거 보류",
  invalid: "배포 또는 지급 근거 불일치",
  verified: "독립 확정 근거 일치",
  "reverted-attempt": "기존 전송 시도가 원복됨 · 최종 지급 상태 별도 조회",
  reverted: "기존 거래 원복",
  "transfer-held": "기존 토큰 수취 보류",
  "no-withdrawal": "거래에 인출 효과 없음",
  "review-mismatch": "현재 수취 주소 또는 금액이 검토와 달라 재전송 보류",
  "different-execution": "실제 수취 주소 또는 금액이 승인과 달라 대사 보류",
  "expired-before-signing": "서명 전 검토 만료",
  "nonce-superseded": "기존 서명 순번 만료 · 이 거래에서 수취 미확인",
};

export function DeploymentConsole({
  mode,
  initialBranch,
}: {
  mode: "v1" | "v2";
  initialBranch: string;
}) {
  const branches = useAuthorizedBranches(
    mode === "v2" ? "finance" : "operator"
  );
  const [branch, setBranch] = useState(initialBranch);
  const [sale, setSale] = useState("");
  const [legacy, setLegacy] = useState<Legacy>();
  const [result, setResult] = useState<DeploymentResult>();
  const deployment = result?.kind === "profile" && result.branch === branch ? result.profile : undefined;
  const payment = result?.kind === "payment" && result.branch === branch && result.sale === sale
    ? result.evidence : undefined;
  const [command, setCommand] = useState<LegacyReview>();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function run(action: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setMessage("");
    try {
      await action();
    } catch (error) {
      setResult(undefined);
      if (!command) setLegacy(undefined);
      setMessage((error as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function legacyRequest(commit?: LegacyReview, resume?: string) {
    const posting = Boolean(commit || resume);
    const suffix = commit
      ? "/cashout"
      : resume
      ? "/resume"
      : command
      ? "?reference=" + encodeURIComponent(command.reference)
      : "";
    const reply = await fetch(
      `/v2/operations/legacy/${encodeURIComponent(branch)}${suffix}`,
      {
        credentials: "same-origin",
        method: posting ? "POST" : "GET",
        headers: posting ? { "Content-Type": "application/json" } : undefined,
        body: posting
          ? JSON.stringify(commit ?? { reference: resume })
          : undefined,
      }
    );
    if (reply.status === 404)
      throw new Error("인증된 지점의 기존 계약 권한이 없습니다.");
    if (reply.status === 409)
      throw new Error(
        "등록된 v1 배포 또는 검토한 잔액이 바뀌었습니다. 기존 원장을 다시 조회하세요."
      );
    if (reply.status === 503) {
      const error = (await reply.json()) as { error?: string };
      if (error.error === "BUSY")
        throw new Error(
          "기존 인출 대사 중입니다. 같은 요청을 다시 확인하세요."
        );
    }
    if (!reply.ok)
      throw new Error("결과를 확인하지 못했습니다. 같은 요청을 확인하세요.");
    setLegacy((await reply.json()) as Legacy);
  }

  return (
    <>
      <h1>{mode === "v2" ? "v2 배포와 지급 증거" : "v1 적립금과 인출"}</h1>
      <form
        className="branch"
        onSubmit={(event) => {
          event.preventDefault();
          if (!uuid.test(branch)) return;
          void run(async () => {
            if (mode === "v2") {
              setResult(undefined);
              const reply = await financeRequest("payment-evidence", { branchId: branch, saleId: sale });
              const evidence = (await reply.json()) as PaymentEvidence;
              setResult({ kind: "payment", branch, sale, evidence });
            } else await legacyRequest();
          });
        }}
      >
        <BranchPicker
          branches={branches.filter((site) =>
            site.capabilities.includes(
              mode === "v2" ? "reconcile_read" : "legacy_ledger"
            )
          )}
          value={branch}
          disabled={busy}
          change={(value) => {
            setBranch(value);
            setLegacy(undefined);
            setResult(undefined);
            setCommand(undefined);
          }}
        />
        {mode === "v2" && (
          <button
            type="button"
            disabled={busy || !uuid.test(branch)}
            onClick={() => void run(async () => {
              setResult(undefined);
              const reply = await financeRequest("deployment-profile", { branchId: branch });
              const profile = (await reply.json()) as DeploymentReview;
              setResult({ kind: "profile", branch, profile });
            })}
          >
            배포 프로필 검증
          </button>
        )}
        {mode === "v2" && (
          <label>
            지급 증거 원주문
            <input
              value={sale}
              required
              maxLength={36}
              disabled={busy}
              onChange={(event) => {
                setSale(event.target.value);
                setResult((value) => value?.kind === "payment" ? undefined : value);
              }}
            />
          </label>
        )}
        <button disabled={busy}>
          {mode === "v2" ? "같은 원주문 증거 조회" : "기존 v1 계약 조회"}
        </button>
      </form>
      <p role="status">{message}</p>
      {deployment && (
        <section aria-label="배포 프로필">
          <h2>{{
            verified: "서명된 배포 프로필 확인",
            unverified: "배포 서명 근거 대기",
            invalid: "배포 설정을 사용할 수 없습니다",
          }[deployment.status]}</h2>
          {deployment.status === "invalid" ? (
            <p>이 배포의 결제 제출을 중단했습니다. 체인과 서명된 배포 프로필을 확인하세요.</p>
          ) : (
            <dl>
              <dt>배포 계약</dt>
              <dd>StableNet {deployment.chain} · {deployment.contract}</dd>
              <dt>허용 결제 토큰</dt>
              <dd>{deployment.asset}</dd>
              <dt>배포 manifest 해시</dt>
              <dd>{deployment.manifest}</dd>
              <dt>ABI 해시</dt>
              <dd>{deployment.abi}</dd>
              <dt>계약 코드 해시</dt>
              <dd>{deployment.runtimeCodeHash}</dd>
              {deployment.startHash && (
                <>
                  <dt>배포 기준 블록</dt>
                  <dd>{deployment.startHeight} · {deployment.startHash}</dd>
                </>
              )}
              {deployment.beneficiaryVersion != null && (
                <>
                  <dt>등록 수취 주소 버전</dt>
                  <dd>v{deployment.beneficiaryVersion}</dd>
                </>
              )}
              {deployment.gasPayer && (
                <>
                  <dt>가맹점 릴레이 가스 부담자</dt>
                  <dd>{deployment.gasPayer}</dd>
                </>
              )}
            </dl>
          )}
        </section>
      )}
      {payment && (
        <section aria-label="등록된 지급 증거">
          <h2>{status[payment.status]}</h2>
          {payment.deploymentBlocked && <p>이 배포의 결제 제출을 중단했습니다.</p>}
          {!payment.deploymentBlocked && <>
          {payment.contract && <dl>
            <dt>배포 계약</dt>
            <dd>
              StableNet {payment.chain} · {payment.contract}
            </dd>
            <dt>허용 결제 토큰</dt>
            <dd>{payment.asset}</dd>
            <dt>배포 manifest 해시</dt>
            <dd>{payment.manifest}</dd>
            <dt>ABI 해시</dt>
            <dd>{payment.abi}</dd>
            <dt>계약 코드 해시</dt>
            <dd>{payment.runtimeCodeHash}</dd>
            {payment.beneficiary && (
              <>
                <dt>견적의 등록 수취 주소 버전</dt>
                <dd>
                  v{payment.beneficiaryVersion} · {payment.beneficiary}
                </dd>
              </>
            )}
            {payment.quoteExpiresAt && (
              <>
                <dt>원 견적 해시</dt>
                <dd>{payment.quoteDigest}</dd>
                <dt>원 견적 발급 및 제출 기한</dt>
                <dd>
                  {payment.quoteIssuedAt} · {payment.quoteExpiresAt}
                </dd>
              </>
            )}
            {payment.includedBeforeExpiry && (
              <>
                <dt>기한 내 블록 포함</dt>
                <dd>{payment.includedAt}</dd>
                <dt>확정 근거 기록 시각</dt>
                <dd>{payment.attestedAt}</dd>
              </>
            )}
            {payment.amount && (
              <>
                <dt>
                  {payment.status === "verified"
                    ? "확정된 지급 금액"
                    : "견적 금액"}
                </dt>
                <dd>{money(payment.amount, replyToken(payment, payment.asset))}</dd>
              </>
            )}
          </dl>}
          {payment.attempt && <p>기존 전송 시도 {payment.attempt}</p>}
          {payment.revertedAttempt && (
            <section aria-label="검증된 원복 거래">
              <h3>이 결제 시도가 원복됐습니다</h3>
              <dl>
                <dt>원복 거래와 포함 블록</dt>
                <dd>{payment.revertedAttempt.transaction} · {payment.revertedAttempt.blockHeight} · {payment.revertedAttempt.block}</dd>
                <dt>이 시도의 수취 원금 증가</dt>
                <dd>{money(payment.revertedAttempt.principalDelta, replyToken(payment, payment.asset))}</dd>
                <dt>이 시도의 주문 소비</dt>
                <dd>{payment.revertedAttempt.orderConsumed ? "소비됨" : "없음"}</dd>
                <dt>가스 사용량</dt>
                <dd>{payment.revertedAttempt.gasUsed}</dd>
                <dt>실효 가스 단가</dt>
                <dd>{payment.revertedAttempt.effectivePriceWei} wei</dd>
                <dt>가스 비용</dt>
                <dd>{payment.revertedAttempt.feeWei} wei</dd>
                <dt>가스 부담자</dt>
                <dd>{payment.revertedAttempt.gasPayer}</dd>
              </dl>
            </section>
          )}
          {payment.transaction && (
            <p>
              확정 거래 {payment.transaction} · 포함 블록 {payment.block} · 로그{" "}
              {payment.log} · 확정 높이 {payment.finalizedHeight}
            </p>
          )}
          </>}
        </section>
      )}
      {legacy && (
        <section aria-label="v1 원장과 인출">
          <h2>기존 v1 적립 · {money(legacy.ledger.balance)}</h2>
          <p>등록 점주 {legacy.ledger.merchant}</p>
          <p>등록 수취 주소 {legacy.ledger.registeredPayout}</p>
          <p>
            v1 계약 {legacy.ledger.contract} · TestUSDC {legacy.ledger.asset}
          </p>
          {legacy.review && (
            <p>
              보관된 인출 검토 {money(legacy.review.balance)} ·{" "}
              {legacy.review.payout}
            </p>
          )}
          {legacy.outcome && (
            <p>
              {status[legacy.outcome.status]} · {legacy.outcome.transaction}
              {legacy.outcome.amount && ` · ${money(legacy.outcome.amount)}`}
              {legacy.outcome.payout && ` · ${legacy.outcome.payout}`}
            </p>
          )}
          {legacy.canResume &&
            legacy.review &&
            legacy.outcome?.transaction &&
            [
              "pending",
              "review-mismatch",
              "transfer-held",
              "verified",
            ].includes(legacy.outcome.status) && (
              <button
                disabled={busy}
                onClick={() =>
                  void run(() =>
                    legacyRequest(undefined, legacy.review?.reference)
                  )
                }
              >
                {legacy.outcome.status === "verified"
                  ? "확정 증거 기록"
                  : "보관된 거래만 다시 대사"}
              </button>
            )}
          {!command &&
            legacy.canWithdraw &&
            BigInt(legacy.ledger.balance) > 0n && (
              <button
                disabled={busy}
                onClick={() =>
                  setCommand({
                    reference: crypto.randomUUID(),
                    version: "v1",
                    contract: legacy.ledger.contract,
                    asset: legacy.ledger.asset,
                    payout: legacy.ledger.registeredPayout,
                    balance: legacy.ledger.balance,
                  })
                }
              >
                기존 적립 인출 검토
              </button>
            )}
          {command && (
            <section aria-label="기존 적립 인출 확인">
              <h3>v1 등록 주소로 기존 적립금 인출</h3>
              <p>
                현재 등록 주소로 실제 실행 시점의 적립금 전액을 인출합니다. 다른
                배포의 결제와 재원은 별도입니다.
              </p>
              <p>
                검토한 금액 {money(command.balance)} · {command.payout}
              </p>
              <button
                disabled={busy}
                onClick={() =>
                  void run(async () => {
                    await legacyRequest(command);
                    setMessage(
                      "동일 인출의 방송 또는 확정 상태를 조회했습니다."
                    );
                  })
                }
              >
                같은 v1 인출 요청
              </button>
              <button
                disabled={busy}
                onClick={() => void run(() => legacyRequest())}
              >
                같은 거래 조회
              </button>
              <button disabled={busy} onClick={() => setCommand(undefined)}>
                검토 닫기
              </button>
            </section>
          )}
        </section>
      )}
    </>
  );
}

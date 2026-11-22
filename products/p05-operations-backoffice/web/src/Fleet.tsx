import { useEffect, useRef, useState, type FormEvent } from "react";
import { useAuthorizedBranches } from "./BranchPicker";
import boardReference from "../../../p10-platform/release/board-reference.json";

type Operation = {
  action: string;
  requestKey: string;
  status: string;
  amount?: string;
  evidence?: { submitted?: string; transactionHash?: string };
  protectedReviewRef?: string;
  protectedRecoveryReviewRef?: string;
};
type ReturnReview = {
  generation: string;
  account: string;
  destination: string;
  destinationVerified: boolean;
  pendingState: "clearance-current" | "reconciliation-required";
  pendingEvidenceUntil?: string;
  generationPending?: {
    state: "unknown" | "frozen" | "incomplete" | "pending" | "verified";
    pendingCount?: number;
    pendingPayments?: number;
    pendingLimits?: number;
    validUntil?: string;
  };
  reviewRef?: string;
};
type ProtectedWithdrawal = {
  state: string;
  closeHash: string;
  amount?: string;
  withdrawAfter?: number;
  withdrawalHash?: string;
  payoutHash?: string;
  acceptanceHash?: string;
  requestKey?: string;
  reviewRef?: string;
  recoveryReviewRef?: string;
  attempt?: {
    state: "reverted" | "superseded" | "executed" | "settled_other";
    originalHash: string;
    evidenceHash: string;
    confirmedAt: string;
  };
};
type Rental = {
  revision: string;
  attestedAt: string | null;
  attestedUntil: string;
  reviewRef: string;
  returnReview?: ReturnReview;
  operations: Operation[];
  withdrawal?: ProtectedWithdrawal;
  cleanup?: { state: string; requestKey?: string };
  cleanupBlocked?: boolean;
  observation: {
    status: string;
    device: string;
    destination: string;
    balance?: string;
    dailyLimit?: string;
    perPaymentLimit?: string;
    withdrawAfter?: number;
    withdrawalEvidence?: string;
    opening?: { txHash: string; finalized?: { height: number; hash: string } };
  };
};
type FirmwareStatus = {
  revision: string;
  installedVersion: string;
  installedCounter: number;
  hold: boolean;
  campaign: string | null;
  state: "running" | "paused" | "verified" | null;
  version: string | null;
  counter: number | null;
};
type Firmware = {
  status: FirmwareStatus;
  releases: Array<{
    id: string;
    version: string;
    boardRevision: string;
    counter: number;
  }>;
};
export type FleetScope = { branch: string; device: string; generation: string };

function RentalDeviceFields({ scope, revision, attestedAt }: {
  scope: FleetScope;
  revision: string;
  attestedAt: string | null;
}) {
  return (
    <>
      <dt>지점 참조</dt><dd>{scope.branch}</dd>
      <dt>기기 참조</dt><dd>{scope.device}</dd>
      <dt>대여 세대</dt><dd>{scope.generation}</dd>
      <dt>등록 리비전</dt><dd>{revision}</dd>
      {revision === boardReference.board && (
        <><dt>제품 모델</dt><dd>{boardReference.catalogModel}</dd></>
      )}
      <dt>기기 인증 시각</dt>
      <dd>{attestedAt && Number.isFinite(Date.parse(attestedAt))
        ? <time dateTime={attestedAt}>{new Date(attestedAt).toLocaleString("ko-KR")}</time>
        : "기록 없음"}</dd>
    </>
  );
}

const reference =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const rentals: Record<string, string> = {
  unopened: "검증된 장비 · 예치 대기",
  active: "대여 계정 개설 확인",
  "wrong-destination": "등록 반환 경로 불일치",
  "payment-hold": "미결 지급 대사 중",
  "withdrawal-delay": "지연 인출 대기",
  "withdrawal-ready": "검증된 경로로 인출 가능",
  "withdrawal-evidence": "인출 결과 증거 대기",
  "cleanup-ready": "이전 세대 반환 경로 확인 · 보드 증거 대기",
  cleaned: "이전 정리 응답 기록 · 보드 증거 없음",
};
const actions: Record<string, string> = {
  open: "대여 계정 개설",
  close: "이 세대 반납",
  withdraw: "지연 인출 실행",
  cleanup: "대여 세션 정리",
};
function parseUnits(amount: string): string {
  if (!/^(0|[1-9][0-9]{0,12})(\.[0-9]{1,6})?$/.test(amount))
    throw new Error("TestUSDC 예치 금액을 확인하세요.");
  const [whole, part = ""] = amount.split(".");
  const units = BigInt(whole + part.padEnd(6, "0"));
  if (units <= 0n) throw new Error("예치 금액은 0보다 커야 합니다.");
  return units.toString();
}
function humanUnits(units: string): string {
  const value = BigInt(units);
  const part = (value % 1000000n)
    .toString()
    .padStart(6, "0")
    .replace(/0+$/, "")
    .padEnd(2, "0");
  return `${value / 1000000n}.${part} TestUSDC`;
}

export function Fleet({
  mode,
  initialBranch,
  initialRentalScope,
  onReconcile,
}: {
  mode: "rental" | "fota";
  initialBranch: string;
  initialRentalScope?: FleetScope;
  onReconcile?: (scope: FleetScope) => void;
}) {
  const branches = useAuthorizedBranches("operator");
  const [input, setInput] = useState<FleetScope>(initialRentalScope ?? {
    branch: initialBranch,
    device: "",
    generation: "1",
  });
  const [site, setSite] = useState<FleetScope>();
  const [rental, setRental] = useState<Rental>();
  const [firmware, setFirmware] = useState<Firmware>();
  const [amount, setAmount] = useState("");
  const [release, setRelease] = useState("");
  const [cohort, setCohort] = useState("");
  const [command, setCommand] = useState("");
  const [proof, setProof] = useState("");
  const [pending, setPending] = useState<{
    action: string;
    key: string;
    returning?: ReturnReview;
    withdrawal?: ProtectedWithdrawal;
    recoverClose?: { originalRef: string; currentRef: string; hash: string };
    opening?: {
      amount: string;
      account: string;
      destination: string;
      dailyLimit: string;
      perPaymentLimit: string;
      attestedUntil: string;
      reviewRef: string;
    };
  }>();
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const running = useRef(false);
  const cleanupKeys = useRef<Record<string, string>>({});
  const [, refreshEvidenceTime] = useState(0);
  const generationEvidence = pending?.returning?.generationPending;
  const evidenceDeadline = Date.parse(generationEvidence?.validUntil ?? "");
  useEffect(() => {
    if (!Number.isFinite(evidenceDeadline)) return;
    const timer = setTimeout(() => refreshEvidenceTime(time => time + 1),
      Math.max(0, evidenceDeadline - Date.now()));
    return () => clearTimeout(timer);
  }, [evidenceDeadline]);
  const count = generationEvidence?.pendingCount;
  const payments = generationEvidence?.pendingPayments;
  const limits = generationEvidence?.pendingLimits;
  const displayEvidenceCount = pending?.returning?.destinationVerified && evidenceDeadline > Date.now() &&
    typeof count === "number" && typeof payments === "number" && typeof limits === "number" &&
    [count, payments, limits].every(value => Number.isSafeInteger(value) && value >= 0 && value <= 8) &&
    count === payments + limits &&
    ((generationEvidence?.state === "pending" && count > 0) || (generationEvidence?.state === "verified" && count === 0));

  function run(task: () => Promise<void>) {
    if (running.current) return;
    running.current = true;
    setNotice("");
    setBusy(true);
    task()
      .catch((error: Error) => setNotice(error.message))
      .finally(() => {
        running.current = false;
        setBusy(false);
      });
  }

  function clearRentalEvidence() {
    setRental(undefined);
    setPending(undefined);
  }

  async function request(scope: FleetScope, action?: string, body?: object) {
    const path = `/v2/operations/${mode}/${encodeURIComponent(
      scope.branch
    )}/${encodeURIComponent(scope.device)}`;
    const query =
      mode === "rental"
        ? `?generation=${encodeURIComponent(scope.generation)}`
        : "";
    const reply = await fetch(path + (action ? "/" + action : "") + query, {
      method: body ? "POST" : "GET",
      credentials: "same-origin",
      headers: body && { "Content-Type": "application/json" },
      body: body && JSON.stringify(body),
    });
    if (mode === "rental" && [401, 403, 404].includes(reply.status)) clearRentalEvidence();
    if (reply.status === 404)
      throw new Error("이 지점의 등록 기기와 담당자 권한을 확인하세요.");
    if (reply.status === 409)
      throw new Error(
        "이 작업은 현재 보류되었습니다. 같은 기기를 다시 조회하세요."
      );
    if (!reply.ok)
      throw new Error(
        "결과를 확인할 수 없습니다. 같은 기기를 다시 조회하세요."
      );
    return reply;
  }

  async function readRental(scope: FleetScope) {
    try {
      return (await (await request(scope)).json()) as Rental;
    } catch (error) {
      clearRentalEvidence();
      throw error;
    }
  }

  async function refresh(scope: FleetScope) {
    if (mode === "rental") setRental(await readRental(scope));
    else setFirmware((await (await request(scope)).json()) as Firmware);
    setSite(scope);
  }
  function read(event: FormEvent) {
    event.preventDefault();
    if (!reference.test(input.branch) || !reference.test(input.device)) {
      setNotice("지점과 기기 참조 형식을 확인하세요.");
      return;
    }
    setRental(undefined);
    setFirmware(undefined);
    setPending(undefined);
    setCommand("");
    setProof("");
    run(() => refresh(input));
  }
  function reset(field: keyof FleetScope, value: string) {
    setInput({ ...input, [field]: value });
    setSite(undefined);
    setRental(undefined);
    setFirmware(undefined);
    setPending(undefined);
    setRelease("");
    setCommand("");
    setProof("");
  }

  function currentReturn(review?: ReturnReview) {
    return review?.pendingState === "clearance-current" &&
      !!review.pendingEvidenceUntil &&
      Date.parse(review.pendingEvidenceUntil) > Date.now();
  }

  async function reviewRental(scope: FleetScope, action: string) {
    const key = crypto.randomUUID();
    if (action === "close") {
      const latest = await readRental(scope);
      setRental(latest);
      if (latest.operations.some((item) => item.action === "close") ||
        !["active", "payment-hold", "wrong-destination"].includes(latest.observation.status)) {
        throw new Error("원래 대여 계정의 현재 반납 상태를 확인하세요.");
      }
      const returning = latest.returnReview;
      if (returning && returning.generation !== scope.generation)
        throw new Error("등록된 대여 세대를 다시 확인하세요.");
      setPending({
        action, key,
        returning: returning ?? {
          generation: scope.generation,
          account: latest.observation.device,
          destination: latest.observation.destination,
          destinationVerified: false,
          pendingState: "reconciliation-required",
        },
      });
      return;
    }
    if (action !== "open") {
      setPending({ action, key });
      return;
    }
    const reviewedAmount = parseUnits(amount);
    const latest = await readRental(scope);
    setRental(latest);
    const { device, destination, dailyLimit, perPaymentLimit, status } =
      latest.observation;
    if (status !== "unopened" || latest.operations.some((item) => item.action === "open") ||
        !dailyLimit || !perPaymentLimit || !latest.attestedUntil || !latest.reviewRef) {
      throw new Error("이 기기의 개설 근거를 다시 확인하세요.");
    }
    setPending({
      action,
      key,
      opening: {
        amount: reviewedAmount,
        account: device,
        destination,
        dailyLimit,
        perPaymentLimit,
        attestedUntil: latest.attestedUntil,
        reviewRef: latest.reviewRef,
      },
    });
  }

  const rentalAction =
    rental &&
    (
      {
        unopened: rental.cleanupBlocked ? undefined : "open",
        active: "close",
        "payment-hold": "close",
        "wrong-destination": "close",
        "withdrawal-ready": "withdraw",
      } as Record<string, string>
    )[rental.observation.status];
  function recordCleanup(scope: FleetScope) {
    const ref = [scope.branch, scope.device, scope.generation].join("/");
    const key = cleanupKeys.current[ref] ?? crypto.randomUUID();
    cleanupKeys.current[ref] = key;
    return run(async () => {
      await request(scope, "cleanup", { requestKey: key });
      await refresh(scope);
    });
  }
  const openingCommand = rental?.operations.find((item) => item.action === "open");
  const openingEvidence = rental?.observation.opening;
  const openingHash = openingEvidence?.txHash ?? openingCommand?.evidence?.submitted;
  const campaign = firmware?.status.campaign;
  const selectedRelease = firmware?.releases?.find(
    (item) => item.id === release
  );
  return (
    <>
      <h1>{mode === "rental" ? "대여 기기 개설과 반납" : "펌웨어 캠페인"}</h1>
      <form className="branch" onSubmit={read}>
        <label>
          지점 참조
          <input
            required
            maxLength={36}
            value={input.branch}
            disabled={busy}
            onChange={(ev) => reset("branch", ev.target.value)}
          />
        </label>
        <label>
          기기 참조
          <input
            required
            maxLength={36}
            value={input.device}
            disabled={busy}
            onChange={(ev) => reset("device", ev.target.value)}
          />
        </label>
        {mode === "rental" && (
          <label>
            대여 세대
            <input
              required
              inputMode="numeric"
              pattern="[1-9][0-9]{0,15}"
              value={input.generation}
              disabled={busy}
              onChange={(ev) => reset("generation", ev.target.value)}
            />
          </label>
        )}
        <button disabled={busy}>같은 기기 조회</button>
      </form>
      <p role="status">{notice}</p>
      {site && rental && (
        <section>
          <section aria-label="등록 대여 기기">
            <h2>v1 대여 계정 · {rental.revision}</h2>
            <dl>
              <RentalDeviceFields scope={site} revision={rental.revision} attestedAt={rental.attestedAt} />
              <dt>기존 계정</dt><dd>{rental.observation.device}</dd>
            </dl>
          </section>
          {!rental.withdrawal && <p>{rentals[rental.observation.status] ?? "확인 필요"}</p>}
          <dl>
            <dt>등록된 반환 경로</dt>
            <dd>{rental.observation.destination}</dd>
            {rental.observation.balance && (
              <>
                <dt>최종 잔액</dt>
                <dd>{humanUnits(rental.observation.balance)}</dd>
              </>
            )}
            {rental.observation.dailyLimit && (
              <>
                <dt>일일 한도</dt>
                <dd>{humanUnits(rental.observation.dailyLimit)}</dd>
              </>
            )}
            {!!rental.observation.withdrawAfter && (
              <>
                <dt>인출 가능 시간</dt>
                <dd>
                  {new Date(
                    rental.observation.withdrawAfter * 1000
                  ).toLocaleString("ko-KR")}
                </dd>
              </>
            )}
          </dl>
          {openingCommand && (
            <section aria-label="원래 대여 개설 결과">
              <h3>대여 개설 조회</h3>
              {openingCommand.amount && <p>요청 예치금 {humanUnits(openingCommand.amount)}</p>}
              {openingHash && <p>개설 예치 거래 {openingHash}</p>}
              {openingEvidence?.finalized ? (
                <dl>
                  <dt>개설 예치 확정 블록 높이</dt>
                  <dd>{openingEvidence.finalized.height}</dd>
                  <dt>개설 예치 확정 블록 해시</dt>
                  <dd>{openingEvidence.finalized.hash}</dd>
                </dl>
              ) : <p>개설 예치 확정 대기</p>}
            </section>
          )}
          {rental.observation.withdrawalEvidence && (
            <p>검증된 반환 거래 {rental.observation.withdrawalEvidence}</p>
          )}
          {rental.withdrawal && (
            <section aria-label="보호 세대 인출 상태">
              <h3>같은 반납의 인출 상태</h3>
              <dl>
                <dt>기기 참조</dt><dd>{site.device}</dd>
                <dt>리비전</dt><dd>{rental.revision}</dd>
                <dt>반납 세대</dt><dd>{`반납 세대 · G-${site.generation}`}</dd>
              </dl>
              <p>{({
                "withdrawal-awaiting-close": "반납 거래 확정 대기",
                "withdrawal-indeterminate": "현재 근거로 인출 상태를 확인할 수 없습니다",
                "withdrawal-no-payout": "원래 반납 요청의 인출액은 0입니다",
                "withdrawal-delay": "지연 인출 대기",
                "withdrawal-ready": "등록한 주소로 인출 가능",
                "withdrawal-retry-review": "원인출 확정 실패 · 아직 인출할 금액이 있습니다",
                "withdrawal-reverted": "정본에서 원인출 거래 실패 확인",
                "withdrawal-superseded": "같은 거래 순번의 다른 거래 확인",
                "withdrawal-request-recorded": "원요청 기록됨 · 정본 결과 확인 중",
                "withdrawal-held": "등록한 수취인의 수령 대기",
                "withdrawal-received": "등록한 주소의 정본 수령 확인",
              } as Record<string, string>)[rental.withdrawal.state]}</p>
              <p>원반납 거래 {rental.withdrawal.closeHash}</p>
              {rental.withdrawal.amount !== undefined && <p>인출액 {rental.withdrawal.amount} 최소 단위</p>}
              {rental.withdrawal.withdrawAfter ? <p>인출 가능 시각 {new Date(rental.withdrawal.withdrawAfter * 1000).toLocaleString()}</p> : null}
              {rental.withdrawal.withdrawalHash && <p>원인출 요청 거래 {rental.withdrawal.withdrawalHash}</p>}
              {rental.withdrawal.payoutHash && <p>인출 거래 {rental.withdrawal.payoutHash}</p>}
              {rental.withdrawal.acceptanceHash && <p>수령 거래 {rental.withdrawal.acceptanceHash}</p>}
              {rental.withdrawal.attempt && (
                <dl>
                  <dt>원인출 거래 대사</dt>
                  <dd>{{
                    reverted: "정본에서 원인출 거래 실패 확인",
                    superseded: "같은 거래 순번의 다른 거래 확인",
                    executed: "정본에서 원인출 실행 확인",
                    settled_other: "별도 거래로 지급 확인 · 원인출 결과 미확정",
                  }[rental.withdrawal.attempt.state]}</dd>
                  <dt>확정 근거 거래</dt><dd>{rental.withdrawal.attempt.evidenceHash}</dd>
                  <dt>확정 근거 확인 시각</dt><dd>{new Date(rental.withdrawal.attempt.confirmedAt).toLocaleString("ko-KR")}</dd>
                </dl>
              )}
              <p>{rental.cleanup?.state === "board-receipt-required"
                ? "정리 원요청 접수 · 보드 완료 영수증 대기 · 재배정 보류"
                : rental.cleanup?.state === "receipt-authenticated-handoff-required"
                  ? "보드 영수증 인증 · 기기 재배정 승인 대기"
                  : "기기 정리 확인 대기 · 재배정 보류"}</p>
              {rental.cleanup?.requestKey && <p>정리 원요청 {rental.cleanup.requestKey}</p>}
              {rental.cleanup?.state === "request-available" && (
                <button disabled={busy} onClick={() => recordCleanup(site)}>정리 원요청 접수</button>
              )}
              <button disabled={busy} onClick={() => run(() => refresh(site))}>같은 인출 조회</button>
              {rental.withdrawal.state === "withdrawal-request-recorded" &&
                rental.withdrawal.requestKey && rental.withdrawal.reviewRef && !rental.withdrawal.recoveryReviewRef && (
                  <button disabled={busy} onClick={() => run(async () => {
                    await request(site, "withdraw", {
                      requestKey: rental.withdrawal?.requestKey,
                      reviewRef: rental.withdrawal?.reviewRef,
                    });
                    await refresh(site);
                  })}>같은 원인출 전송 재시도</button>
              )}
              {rental.withdrawal.requestKey && rental.withdrawal.recoveryReviewRef && !pending && (
                <button disabled={busy} onClick={() => setPending({
                  action: "withdraw", key: rental.withdrawal!.requestKey!, withdrawal: rental.withdrawal,
                })}>같은 원인출 다시 검토</button>
              )}
              {["withdrawal-ready", "withdrawal-retry-review"].includes(rental.withdrawal.state) && rental.withdrawal.reviewRef && !pending && (
                <button disabled={busy} onClick={() => setPending({
                  action: "withdraw", key: crypto.randomUUID(), withdrawal: rental.withdrawal,
                })}>보호 세대 인출 검토</button>
              )}
            </section>
          )}
          {rental.cleanupBlocked && <p>이전 세대 보드 정리 증거 대기 · 새 대여 보류</p>}
          {rental.operations.map((item) => {
            const protectedHash = item.evidence?.transactionHash;
            const phase = protectedHash ? ({
              recorded: "원요청 기록됨",
              prepared: "원거래 준비됨",
              attempting: "전송 결과 확인 중",
              submitted: "전송 접수 · 정본 확정 대기",
              confirmed: "정본에서 반납 거래 확인",
              reverted: "정본에서 거래 실패 확인",
              superseded: "같은 거래 순번의 다른 거래 확인",
            } as Record<string, string>)[item.status] : undefined;
            const canRetry = item.protectedReviewRef &&
              ["prepared", "attempting", "submitted"].includes(item.status);
            return (
              <div key={item.requestKey || protectedHash}>
                <p>
                  {actions[item.action]} · {phase ??
                    (item.action === "cleanup" ? "이전 정리 기록 · 보드 증거 없음" : "같은 요청을 대사 중")}
                  {item.requestKey && <> · {item.requestKey}</>}
                </p>
                {protectedHash && <p>원거래 해시 {protectedHash}</p>}
                {item.action === "close" && !protectedHash && item.evidence?.submitted && (
                  <p>기록된 반납 거래 {item.evidence.submitted}</p>
                )}
                {protectedHash && item.requestKey && item.protectedReviewRef && item.protectedRecoveryReviewRef && !pending && (
                  <button disabled={busy} onClick={() => setPending({
                    action: "close", key: item.requestKey,
                    recoverClose: { originalRef: item.protectedReviewRef!, currentRef: item.protectedRecoveryReviewRef!, hash: protectedHash },
                  })}>
                    같은 원반납 다시 검토
                  </button>
                )}
                {canRetry && (
                  <button disabled={busy} onClick={() => run(async () => {
                    await request(site, "close", {
                      requestKey: item.requestKey,
                      reviewRef: item.protectedReviewRef,
                    });
                    await refresh(site);
                  })}>
                    같은 원반납 전송 재시도
                  </button>
                )}
              </div>
            );
          })}
          {rentalAction &&
            !rental.operations.some((item) => item.action === rentalAction) &&
            !pending && (
              <>
                {rentalAction === "open" && (
                  <label>
                    TestUSDC 예치금
                    <input
                      inputMode="decimal"
                      value={amount}
                      disabled={busy}
                      onChange={(ev) => setAmount(ev.target.value)}
                    />
                  </label>
                )}
                <button
                  disabled={busy}
                  onClick={() => run(() => reviewRental(site, rentalAction))}
                >
                  {actions[rentalAction]} 검토
                </button>
              </>
            )}
          {pending && (
            <section aria-label="대여 작업 검토">
              <h3>{actions[pending.action]} 요청</h3>
              {(pending.opening || pending.returning) && (
                <dl>
                  <RentalDeviceFields scope={site} revision={rental.revision} attestedAt={rental.attestedAt} />
                </dl>
              )}
              {pending.recoverClose && (
                <dl>
                  <dt>재검토할 원반납 거래</dt><dd>{pending.recoverClose.hash}</dd>
                  <dt>등록한 계정</dt><dd>{rental.observation.device}</dd>
                  <dt>등록한 반환 주소</dt><dd>{rental.observation.destination}</dd>
                </dl>
              )}
              {pending.withdrawal && (
                <dl>
                  <dt>원반납 거래</dt><dd>{pending.withdrawal.closeHash}</dd>
                  <dt>등록한 인출 주소</dt><dd>{rental.observation.destination}</dd>
                  <dt>인출액 최소 단위</dt><dd>{pending.withdrawal.amount}</dd>
                </dl>
              )}
              <p>
                지정된 대여 세대 {site.generation} ·{" "}
                {pending.returning?.destination ?? pending.opening?.destination ?? rental.observation.destination}
              </p>
              {pending.returning && (
                <dl>
                  <dt>원 대여 계정</dt>
                  <dd>{pending.returning.account}</dd>
                  <dt>반환 경로 확인</dt>
                  <dd>{pending.returning.destinationVerified ? "확정 계정의 반환 목적지와 일치" : "검증 필요"}</dd>
                  <dt>미결 지급</dt>
                  <dd>{displayEvidenceCount
                    ? `미결 ${count}건 · 지급 ${payments}건 · 한도 변경 ${limits}건`
                    : "건수 확인 필요"}</dd>
                  <dt>대사 근거</dt>
                  <dd>{currentReturn(pending.returning) && pending.returning.pendingEvidenceUntil
                    ? `유효 기한 · ${new Date(pending.returning.pendingEvidenceUntil).toLocaleString("ko-KR")}`
                    : "대사 필요"}</dd>
                </dl>
              )}
              {pending.returning && onReconcile && branches.some(
                branch => branch.id === site.branch && branch.capabilities.includes("legacy_ledger")
              ) && (
                <button type="button" disabled={busy} onClick={() => onReconcile(site)}>
                  v1 계약 대사
                </button>
              )}
              {pending.opening && (
                <dl>
                  <dt>대여 계정</dt>
                  <dd>{pending.opening.account}</dd>
                  <dt>요청 예치금</dt>
                  <dd>{humanUnits(pending.opening.amount)}</dd>
                  <dt>일일 한도</dt>
                  <dd>{humanUnits(pending.opening.dailyLimit)}</dd>
                  <dt>건당 한도</dt>
                  <dd>{humanUnits(pending.opening.perPaymentLimit)}</dd>
                  <dt>기기 인증 유효 기한</dt>
                  <dd>{new Date(pending.opening.attestedUntil).toLocaleString("ko-KR")}</dd>
                </dl>
              )}
              <button
                disabled={busy || (!!pending.returning && (!pending.returning.reviewRef || !currentReturn(pending.returning)))}
                onClick={() =>
                  run(async () => {
                    if (pending.returning && (!pending.returning.reviewRef || !currentReturn(pending.returning)))
                      throw new Error("반납 근거가 만료되었습니다. 검토를 다시 열어 주세요.");
                    const body = {
                      requestKey: pending.key,
                      ...(pending.opening ? {
                        amount: pending.opening.amount,
                        reviewRef: pending.opening.reviewRef,
                      } : {}),
                      ...(pending.returning ? { reviewRef: pending.returning.reviewRef } : {}),
                      ...(pending.withdrawal ? { reviewRef: pending.withdrawal.reviewRef,
                        ...(pending.withdrawal.recoveryReviewRef ? { recoveryReviewRef: pending.withdrawal.recoveryReviewRef } : {}),
                      } : {}),
                      ...(pending.recoverClose ? { reviewRef: pending.recoverClose.originalRef, recoveryReviewRef: pending.recoverClose.currentRef } : {}),
                    };
                    const result = (await (
                      await request(site, pending.action, body)
                    ).json()) as Rental;
                    if (pending.withdrawal && result.withdrawal) {
                      setRental(previous => previous && { ...previous, withdrawal: result.withdrawal });
                    } else if (result.observation) setRental(result);
                    else {
                      // Keep the last chain view until the next read. The guarded
                      // write returns the durable operation only.
                      setRental(previous => previous && { ...previous,
                        operations: [...previous.operations.filter(item => item.action !== pending.action), ...result.operations],
                      });
                    }
                    setPending(undefined);
                    setNotice("같은 요청의 체인 또는 기기 응답을 조회하세요.");
                  })
                }
              >
                요청 기록
              </button>
              <button disabled={busy} onClick={() => setPending(undefined)}>
                닫기
              </button>
            </section>
          )}
        </section>
      )}
      {site && firmware && (
        <section>
          <h2>
            {firmware.status.revision} · 현재 {firmware.status.installedVersion}
          </h2>
          {firmware.status.hold && (
            <p>회수 또는 미결 승인 증거를 확인할 때까지 적용을 보류합니다.</p>
          )}
          {firmware.status.state && (
            <p>
              캠페인 {campaign} · {firmware.status.version} ·{" "}
              {
                {
                  running: "적용과 응답 대기",
                  paused: "중단됨",
                  verified: "보드의 확인된 적용 증거",
                }[firmware.status.state]
              }
            </p>
          )}
          {(!campaign || firmware.status.state === "verified") && (
            <>
              <label>
                서명 검증된 배포물
                <select
                  value={release}
                  disabled={busy}
                  onChange={(ev) => setRelease(ev.target.value)}
                >
                  <option value="">배포물 선택</option>
                  {firmware.releases
                    .filter(
                      (item) => item.counter > firmware.status.installedCounter
                    )
                    .map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.version} · 카운터 {item.counter}
                      </option>
                    ))}
                </select>
              </label>
              <label>
                기존 캠페인 참조 (여러 기기에 같은 버전 배포 시)
                <input
                  value={cohort}
                  disabled={busy}
                  maxLength={36}
                  onChange={(event) => {
                    setCohort(event.target.value);
                    setPending(undefined);
                  }}
                />
              </label>
              <button
                disabled={
                  busy ||
                  firmware.status.hold ||
                  !selectedRelease ||
                  (!!cohort && !reference.test(cohort))
                }
                onClick={() => {
                  if (!selectedRelease) return;
                  setPending({
                    action: "start",
                    key: cohort || crypto.randomUUID(),
                  });
                }}
              >
                캠페인 검토
              </button>
              {pending?.action === "start" && selectedRelease && (
                <section aria-label="캠페인 확인">
                  <h3>{selectedRelease.version} 적용 확인</h3>
                  <p>
                    {selectedRelease.boardRevision} · 카운터{" "}
                    {selectedRelease.counter}
                  </p>
                  <button
                    disabled={busy}
                    onClick={() =>
                      run(async () => {
                        await request(site, "start", {
                          campaign: pending.key,
                          release: selectedRelease.id,
                        });
                        setPending(undefined);
                        await refresh(site);
                      })
                    }
                  >
                    이 기기 캠페인 시작
                  </button>
                  <button disabled={busy} onClick={() => setPending(undefined)}>
                    취소
                  </button>
                </section>
              )}
            </>
          )}
          {!!campaign && firmware.status.state !== "verified" && (
            <>
              <button
                disabled={busy}
                onClick={() =>
                  run(async () => {
                    await request(site, "pause", { campaign });
                    setCommand("");
                    setProof("");
                    await refresh(site);
                  })
                }
              >
                캠페인 중지
              </button>
              <button
                disabled={busy || firmware.status.hold}
                onClick={() =>
                  run(async () => {
                    await request(site, "resume", { campaign });
                    await refresh(site);
                  })
                }
              >
                같은 캠페인 재개
              </button>
            </>
          )}
          {!!campaign &&
            firmware.status.state === "running" &&
            !firmware.status.hold && (
              <>
                <button
                  disabled={busy}
                  onClick={() =>
                    run(async () => {
                      const reply = await request(site, "image", { campaign });
                      if (
                        reply.headers.get("content-type") !==
                        "application/octet-stream"
                      )
                        throw new Error("서명 이미지 응답을 확인하세요.");
                      const url = URL.createObjectURL(await reply.blob());
                      const link = document.createElement("a");
                      link.href = url;
                      link.download = "nu54-signed-update.bin";
                      link.click();
                      setTimeout(() => URL.revokeObjectURL(url), 1000);
                    })
                  }
                >
                  등록된 서명 이미지 받기
                </button>
                <button
                  disabled={busy}
                  onClick={() =>
                    run(async () => {
                      const reply = (await (
                        await request(site, "challenge", { campaign })
                      ).json()) as { command: string };
                      if (!/^10[0-9a-f]{96}$/.test(reply.command))
                        throw new Error("확인 요청이 올바르지 않습니다.");
                      setCommand(reply.command);
                    })
                  }
                >
                  확인된 부팅 증거 요청
                </button>
                {!!command && (
                  <>
                    <p>
                      등록 기기의 펌웨어 증거 화면에서 이 요청을 확인하세요.
                    </p>
                    <label>
                      기기 확인 요청
                      <textarea readOnly value={command} rows={3} />
                    </label>
                    <label>
                      보드가 서명한 측정 증거
                      <textarea
                        value={proof}
                        disabled={busy}
                        maxLength={360}
                        onChange={(e) => setProof(e.target.value)}
                      />
                    </label>
                    <button
                      disabled={busy || !/^[0-9a-f]{360}$/.test(proof.trim())}
                      onClick={() =>
                        run(async () => {
                          await request(site, "ack", {
                            campaign,
                            proof: proof.trim(),
                          });
                          setCommand("");
                          setProof("");
                          await refresh(site);
                          setNotice(
                            "보드의 확인된 측정값과 등록 이미지가 일치합니다."
                          );
                        })
                      }
                    >
                      이 기기 적용 확인
                    </button>
                  </>
                )}
              </>
            )}
        </section>
      )}
    </>
  );
}

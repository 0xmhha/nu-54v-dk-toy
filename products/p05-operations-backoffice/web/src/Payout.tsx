import { useRef, useState, type FormEvent } from "react";

type PredecessorQuote = {
  saleId: string;
  quoteId: string;
  expiresAt: string;
  status: "pending" | "paid" | "expired";
  admission?: "snapshotted" | "grandfathered" | "cutover-rejected";
  transaction?: string;
  expiryEvidence?: string;
};

type PayoutState = {
  requestKey?: string;
  currentAddress?: string;
  currentVersion?: number;
  proposedVersion?: number;
  activatedAt?: string;
  effectiveBlock?: string;
  quoteTracking?: "issuer-profile" | "rotation-archive" | "unavailable";
  oldQuotes?: PredecessorQuote[];
  oldQuoteCount?: number;
  oldAddress?: string;
  oldVersion?: number;
  address?: string;
  reason?: string;
  approvals?: number;
  effectiveAt?: string;
  notifiedAt?: string;
  txHash?: string;
  phase?: "review" | "frozen" | "signed" | "active" | "held";
};
type Challenge = {
  requestKey: string;
  address: string;
  reason: string;
  expiresAt: number;
};

async function exchange(branch: string, operation?: string, body?: object, requestKey?: string) {
  const suffix = operation ? `/${operation}` : requestKey ? `?requestKey=${encodeURIComponent(requestKey)}` : "";
  const response = await fetch(
    `/v2/operations/payout/${encodeURIComponent(branch)}${suffix}`,
    {
      method: body ? "POST" : "GET",
      credentials: "same-origin",
      headers: body && { "Content-Type": "application/json" },
      body: body && JSON.stringify(body),
    }
  );
  if (response.status === 404)
    throw new Error("지점 또는 지급 주소 변경 권한이 없습니다.");
  if (response.status === 409)
    throw new Error("변경 단계가 갱신됐습니다. 같은 요청을 조회하세요.");
  if (response.status === 400)
    throw new Error("주소, 서명 또는 소유 증명 시간을 확인하세요.");
  if (!response.ok)
    throw new Error("처리 여부를 확인할 수 없습니다. 같은 변경을 조회하세요.");
  return response.json();
}

export function Payout({ initialBranch }: { initialBranch: string }) {
  const [site, setSite] = useState(initialBranch);
  const [branch, setBranch] = useState("");
  const [requestRef, setRequestRef] = useState("");
  const [state, setState] = useState<PayoutState>();
  const [address, setAddress] = useState("");
  const [reason, setReason] = useState("custody_rotation");
  const [proof, setProof] = useState("");
  const [challenge, setChallenge] = useState<{
    request: Challenge;
    message: string;
  }>();
  const [decision, setDecision] = useState<"approve" | "acknowledge">();
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const running = useRef(false);

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

  function open(event: FormEvent) {
    event.preventDefault();
    setState(undefined);
    setChallenge(undefined);
    setDecision(undefined);
    run(async () => {
      const activeSite = site.trim();
      const result = (await exchange(activeSite, undefined, undefined, requestRef.trim())) as PayoutState;
      if (requestRef.trim() && !result.requestKey)
        throw new Error("이 지점에서 해당 변경 요청을 확인할 수 없습니다.");
      setState(result);
      setBranch(activeSite);
    });
  }

  function begin(event: FormEvent) {
    event.preventDefault();
    run(async () => {
      const request = {
        requestKey: crypto.randomUUID(),
        address: address.toLowerCase(),
        reason,
        expiresAt: Math.floor(Date.now() / 1000) + 600,
      };
      const reply = (await exchange(branch, "challenge", request)) as {
        requestKey: string;
        message: string;
      };
      if (reply.requestKey !== request.requestKey)
        throw new Error("소유 증명 요청을 다시 확인하세요.");
      setProof("");
      setChallenge({ request, message: reply.message });
    });
  }

  return (
    <>
      <h1>지급 주소 등록부</h1>
      <form className="branch" onSubmit={open}>
        <label>
          지점 참조
          <input
            required
            maxLength={36}
            value={site}
            disabled={busy}
            onChange={(event) => {
              setSite(event.target.value);
              setBranch("");
              setRequestRef("");
              setState(undefined);
              setChallenge(undefined);
              setDecision(undefined);
              setProof("");
            }}
          />
        </label>
        <label>
          변경 요청 참조 (선택)
          <input
            value={requestRef}
            maxLength={36}
            disabled={busy}
            onChange={(event) => setRequestRef(event.target.value)}
          />
        </label>
        <button disabled={busy}>같은 변경 상태 조회</button>
      </form>
      <p role="status">{notice}</p>
      {branch && (
        <>
          <section>
            <h2>지점 지급 주소 변경</h2>
            <p>StableNet 테스트넷 8283</p>
            {state?.currentVersion != null && (
              <dl>
                <dt>현재 지급 주소 버전</dt>
                <dd>v{state.currentVersion} · {state.currentAddress}</dd>
              </dl>
            )}
            {state?.requestKey ? (
              <>
                <dl>
                  <dt>등록부 요청</dt>
                  <dd>{state.requestKey}</dd>
                  <dt>이전 지급 주소 버전</dt>
                  <dd>
                    {state.oldVersion} · {state.oldAddress}
                  </dd>
                  <dt>제안 지급 주소 버전</dt>
                  <dd>v{state.proposedVersion} · {state.address}</dd>
                  <dt>별도 승인</dt>
                  <dd>{state.approvals ?? 0} / 2</dd>
                  <dt>변경 통지</dt>
                  <dd>{state.notifiedAt ? "확인됨" : "확인 대기"}</dd>
                  {state.notifiedAt && (
                    <>
                      <dt>지점 통지 확인 기록</dt>
                      <dd>{new Date(state.notifiedAt).toLocaleString("ko-KR")}</dd>
                    </>
                  )}
                  <dt>효력 예정</dt>
                  <dd>
                    {state.effectiveAt
                      ? new Date(state.effectiveAt).toLocaleString("ko-KR")
                      : "별도 승인 후"}
                  </dd>
                  {state.activatedAt && (
                    <>
                      <dt>등록부 효력 시작</dt>
                      <dd>{new Date(state.activatedAt).toLocaleString("ko-KR")}</dd>
                    </>
                  )}
                  {state.effectiveBlock && (
                    <>
                      <dt>확정 등록 블록</dt>
                      <dd>{state.effectiveBlock}</dd>
                    </>
                  )}
                  <dt>진행 상태</dt>
                  <dd>
                    {
                      {
                        review: "승인 또는 효력 대기",
                        frozen: "새 견적 일시 정지",
                        signed: "동일 거래 확정 조회",
                        active: "새 버전 적용",
                        held: "보류",
                      }[state.phase ?? "review"]
                    }
                  </dd>
                </dl>
                {state.txHash && <p>등록 거래 {state.txHash}</p>}
                <button disabled={busy} onClick={() => run(async () => {
                  setState((await exchange(branch, undefined, undefined, state.requestKey)) as PayoutState);
                })}>
                  이 변경 요청 다시 조회
                </button>
                <section aria-label="기발급 견적 처리">
                  <h3>기발급 견적 처리</h3>
                  {state.quoteTracking === "unavailable" && <p>보관된 견적 연결 근거를 확인할 수 없습니다.</p>}
                  {state.quoteTracking === "rotation-archive" && <p>변경 거래에 보존된 견적을 표시합니다.</p>}
                  {state.oldQuoteCount === 0 && state.quoteTracking === "issuer-profile" && <p>추적할 기발급 견적이 없습니다.</p>}
                  {state.oldQuoteCount != null && state.oldQuotes && state.oldQuoteCount > state.oldQuotes.length && (
                    <p>기발급 견적 {state.oldQuoteCount}건 중 최근 {state.oldQuotes.length}건</p>
                  )}
                  {state.oldQuotes?.map((quote) => (
                    <article key={quote.saleId}>
                      <h4>견적 {quote.quoteId}</h4>
                      <p>원주문 {quote.saleId}</p>
                      <p>견적 유효 시각 {new Date(quote.expiresAt).toLocaleString("ko-KR")}</p>
                      <p>{{
                        pending: "지급 결과 확인 중",
                        paid: "기존 주소로 지급 확정",
                        expired: "확정 근거로 미지급 만료",
                      }[quote.status]}</p>
                      {quote.admission && (
                        <p>{{
                          snapshotted: "변경 거래에 포함된 견적",
                          grandfathered: "발급 당시 지급 주소 유지",
                          "cutover-rejected": "새 등록부에서 기존 견적 지급 거절",
                        }[quote.admission]}</p>
                      )}
                      {quote.transaction && <p>원지급 거래 {quote.transaction}</p>}
                      {quote.expiryEvidence && <p>미지급 확정 근거 {quote.expiryEvidence}</p>}
                    </article>
                  ))}
                </section>
                {state.phase === "review" && (
                  <>
                    <button
                      disabled={busy}
                      onClick={() => setDecision("approve")}
                    >
                      현재 승인자 검토
                    </button>
                    <button
                      disabled={busy || state.approvals !== 2}
                      onClick={() => setDecision("acknowledge")}
                    >
                      변경 통지 확인
                    </button>
                  </>
                )}
              </>
            ) : (
              <p>검토 중인 요청이 없습니다.</p>
            )}
            {decision && state?.requestKey && (
              <section aria-label="주소 변경 확인">
                <h3>
                  {decision === "approve"
                    ? "제안자와 다른 승인자로 검토합니다"
                    : "실제로 전달된 지점 변경 통지를 확인합니다"}
                </h3>
                <p>
                  {state.oldAddress} → {state.address}
                </p>
                <button
                  disabled={busy}
                  onClick={() =>
                    run(async () => {
                      setState(
                        (await exchange(branch, decision, {
                          requestKey: state.requestKey,
                        })) as PayoutState
                      );
                      setDecision(undefined);
                      setNotice("같은 요청에 검토 결과를 기록했습니다.");
                    })
                  }
                >
                  현재 계정으로 확인
                </button>
                <button disabled={busy} onClick={() => setDecision(undefined)}>
                  취소
                </button>
              </section>
            )}
          </section>
          {(!state?.requestKey || state.phase === "active") && (
            <section>
              <h2>다음 지급 주소 제안</h2>
              <form onSubmit={begin}>
                <label>
                  새 지급 주소
                  <input
                    required
                    value={address}
                    disabled={busy || !!challenge}
                    maxLength={42}
                    pattern="0x[0-9a-fA-F]{40}"
                    onChange={(event) => setAddress(event.target.value)}
                  />
                </label>
                <label>
                  변경 사유
                  <select
                    disabled={busy || !!challenge}
                    value={reason}
                    onChange={(event) => setReason(event.target.value)}
                  >
                    <option value="custody_rotation">보관 계정 교체</option>
                    <option value="governance">운영 정책 변경</option>
                  </select>
                </label>
                <button disabled={busy || !!challenge}>소유 증명 요청</button>
              </form>
              {challenge && (
                <form
                  onSubmit={(event) => {
                    event.preventDefault();
                    run(async () => {
                      setState(
                        (await exchange(branch, "propose", {
                          ...challenge.request,
                          proof: proof.trim(),
                        })) as PayoutState
                      );
                      setChallenge(undefined);
                      setProof("");
                      setNotice(
                        "소유 증명을 검증했고 분리 승인 대기열에 등록했습니다."
                      );
                    });
                  }}
                >
                  <label>
                    새 주소의 지갑에서 서명할 메시지
                    <textarea readOnly value={challenge.message} rows={11} />
                  </label>
                  <label>
                    새 지갑에서 받은 서명
                    <input
                      required
                      maxLength={132}
                      value={proof}
                      disabled={busy}
                      pattern="(0x)?[0-9a-fA-F]{130}"
                      onChange={(event) => setProof(event.target.value)}
                    />
                  </label>
                  <button disabled={busy}>소유 증명 확인</button>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => {
                      setChallenge(undefined);
                      setProof("");
                    }}
                  >
                    취소
                  </button>
                </form>
              )}
            </section>
          )}
        </>
      )}
    </>
  );
}

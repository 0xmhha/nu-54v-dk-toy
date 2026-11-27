'use strict';

const byId = (id) => document.getElementById(id);
const deployment = byId('deployment');
const summary = byId('summary');
const results = byId('results');
const operations = document.body.dataset.screen === 'operations';
let profiles = [];
let pending;

async function read(path, query, signal, payload) {
  const request = query ? path + '?' + new URLSearchParams(query) : path;
  const options = {cache: 'no-store', signal};
  if (payload) {
    options.method = 'POST';
    options.headers = {'Content-Type': 'application/json'};
    options.body = JSON.stringify(payload);
  }
  const reply = await fetch(request, options);
  if (!reply.ok) throw new Error(String(reply.status));
  return reply.json();
}

function node(tag, text) {
  const element = document.createElement(tag);
  if (text != null) element.textContent = text;
  return element;
}

function card(title, fields, container = results) {
  const article = node('article');
  const list = node('dl');
  for (const [label, value] of fields) {
    if (value != null) list.append(node('dt', label), node('dd', value));
  }
  article.append(node('h3', title), list);
  container.append(article);
  return article;
}

function showTime(value) {
  return value ? new Date(value).toLocaleString('ko-KR', {timeZoneName: 'short'}) : null;
}

function deploymentTitle(profile) {
  return profile.chain + ' · v' + profile.version + ' · ' + profile.contract;
}

function status(profile) {
  if (profile.status === 'unmapped') return '배포 구성을 확인하세요 · 이 구성만 수집 보류';
  if (profile.status === 'held') return '확정 증거 충돌 · 지급 증거 제공 보류';
  if (profile.status === 'catching_up') return '최신 체인 확인 중';
  return '현재 관측 범위 확인';
}

function currentProfile() {
  return profiles.find((profile) => profile.stream === deployment.value);
}

async function loadProfiles() {
  try {
    const prior = deployment.value;
    profiles = (await read(operations ? '/v2/explorer/monitor' : '/v2/explorer/deployments')) || [];
    deployment.replaceChildren();
    for (const profile of profiles) {
      if (profile.status === 'unmapped') continue;
      const option = node('option', deploymentTitle(profile));
      option.value = profile.stream;
      deployment.append(option);
    }
    if (profiles.some((profile) => profile.stream === prior)) deployment.value = prior;
    if (!profiles.length) {
      deployment.append(node('option', '조회 가능한 배포 없음'));
      deployment.selectedOptions[0].value = '';
      summary.textContent = '등록된 배포 구성을 확인하세요.';
      return false;
    }
    if (operations) {
      results.replaceChildren();
      for (const profile of profiles) {
        card(deploymentTitle(profile), [
          ['동기화 상태', status(profile)], ['시작 높이', profile.base], ['커서', profile.tip],
          ['보류 구성', profile.proposedGeneration], ['ABI 해시', profile.abiHash],
          ['보류 사유', {unknown_abi: '미등록 ABI', unapproved_profile: '서명된 배포 구성이 필요합니다',
            identity_mismatch: '체인·주소·코드·시작 블록 확인이 필요합니다'}[profile.holdReason]],
          ['커서 해시', profile.tipHash], ['확정 커서', profile.finalized], ['확정 해시', profile.finalHash],
          ['RPC 확정 높이', profile.reportedFinality], ['마지막 관측 시각', showTime(profile.observedAt)],
          ['최근 미확정 분기의 마지막 공통 높이', profile.ancestor], ['분기 관측 시각', showTime(profile.forkAt)],
          ['확정 충돌 조사 번호', profile.incident], ['충돌 높이', profile.disputed],
          ['이전 해시', profile.previousHash], ['새 해시', profile.nextHash], ['충돌 접수', showTime(profile.openedAt)],
        ]);
      }
      const held = profiles.filter((profile) => profile.status === 'unmapped').length;
      summary.textContent = '허용된 배포 ' + (profiles.length - held) + '개' + (held ? ' · 구성 보류 ' + held + '개' : '');
      const replay = byId('replay-deployment');
      const selected = replay.value;
      replay.replaceChildren(...Array.from(deployment.options, (option) => option.cloneNode(true)));
      if (Array.from(replay.options).some((option) => option.value === selected)) replay.value = selected;
      void loadReplay();
    } else {
      summary.textContent = status(currentProfile());
    }
    return true;
  } catch (_) {
    results.replaceChildren();
    deployment.replaceChildren(node('option', '조회 불가'));
    deployment.selectedOptions[0].value = '';
    summary.textContent = '배포 정보를 조회할 수 없습니다.';
    return false;
  }
}

function selectedOrder(form) {
  const fields = new FormData(form);
  return {
    stream: deployment.value,
    merchant: String(fields.get('merchant')).trim().toLowerCase(),
    order: String(fields.get('order')).trim().toLowerCase(),
  };
}

function invalidOrder(order) {
  return !/^0x(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(order.merchant) ||
    !/^0x[a-f0-9]{64}$/.test(order.order);
}

function formatAmount(raw, decimals) {
  if (!/^[0-9]+$/.test(raw) || !Number.isInteger(decimals) || decimals < 0 || decimals > 255) return null;
  if (!decimals) return raw;
  const digits = raw.padStart(decimals + 1, '0');
  const split = digits.length - decimals;
  return digits.slice(0, split) + '.' + digits.slice(split);
}

function localRecipientCheck(article, current, receipt) {
  if (!receipt.recipientCommitment) return;
  const form = node('form');
  const field = node('input');
  field.type = 'text';
  field.autocomplete = 'off';
  field.spellcheck = false;
  const label = node('label', '원 견적에서 확인한 수령 주소');
  label.append(field);
  const button = node('button', '이 기기에서 수령 주소 대조');
  const status = node('p');
  status.setAttribute('aria-live', 'polite');
  form.append(label, button, status);
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    const address = field.value.trim().toLowerCase();
    if (!/^0x[a-f0-9]{40}$/.test(address)) {
      status.textContent = '전체 수령 주소 형식을 확인하세요.';
      return;
    }
    if (!globalThis.isSecureContext || !globalThis.crypto?.subtle) {
      status.textContent = '주소 대조에는 HTTPS 브라우저가 필요합니다.';
      return;
    }
    const data = ['nu54-explorer-recipient-v1', current.chain, current.contract.toLowerCase(),
      receipt.txHash.toLowerCase(), receipt.logIndex, address].join('|');
    const bytes = new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(data)));
    const hash = [...bytes].map((byte) => byte.toString(16).padStart(2, '0')).join('');
    status.textContent = hash === receipt.recipientCommitment ?
      '원 견적 주소가 확정 지급 로그의 수령 주소와 일치합니다.' : '이 주소는 해당 지급 로그의 수령 주소와 일치하지 않습니다.';
  });
  article.append(node('p', '입력한 주소는 이 브라우저 안에서만 대조합니다.'), form);
}

function publicSearch(form) {
  const select = form.elements.lookup;
  const isTransaction = select.value === 'transaction';
  byId('reference-label').textContent = isTransaction ? '거래 해시' : '공개 주문 키';
  byId('log-field').hidden = !isTransaction;
  form.elements.logIndex.disabled = !isTransaction;
}

async function renderReceipts(form, next = '') {
  if (pending) pending.abort();
  const controller = new AbortController();
  pending = controller;
  if (!next) results.replaceChildren();
  const order = selectedOrder(form);
  if (invalidOrder(order)) {
    summary.textContent = '주소 또는 32바이트 가맹점 키와 32바이트 주문 키를 확인하세요.';
    return;
  }
  const profile = currentProfile();
  if (!profile) return;
  summary.textContent = '선택한 배포의 영수증을 조회합니다.';
  const query = next ? {...order, after: next} : {...order};
  const fields = new FormData(form);
  if (fields.get('lookup') === 'transaction') {
    query.txHash = query.order;
    delete query.order;
    if (fields.get('logIndex')) query.logIndex = fields.get('logIndex');
  }
  try {
    const data = await read('/v2/receipts', query, controller.signal);
    if (controller.signal.aborted) return;
    const current = data.deployment;
    if (!current) {
      results.replaceChildren();
      summary.textContent = '현재 배포의 기준 높이를 확인할 수 없습니다.';
      return;
    }
    if (current.status === 'held') {
      results.replaceChildren();
      summary.textContent = '확정 증거가 충돌하여 이 배포의 공개 지급 증거를 보류합니다.';
      return;
    }
    const view = '최종 확정 높이 ' + (current.finalized ?? '대기') +
      ' · 마지막 관측 ' + (showTime(current.observedAt) ?? '없음');
    if (!data.receipts.length) {
      summary.textContent = (current.status === 'catching_up' ? '최신 체인 확인 중. ' : '영수증이 아직 색인되지 않았습니다. ') +
        view + ' · 이 결과로 미지급이나 재청구를 판정할 수 없습니다.';
      return;
    }
    summary.textContent = status(current) + ' · ' + view;
    if (!next && data.totals) {
      const amount = (raw) => formatAmount(raw, data.assetDecimals) ?? raw + ' 최소 단위';
      card('원지급과 누적 환불', [
        ['원지급', amount(data.totals.paid)], ['누적 확정 환불', amount(data.totals.refunded)],
        ['환불 차감 순지급', amount(data.totals.remaining)], ['주문 키', data.totals.order],
      ]);
    }
    for (const receipt of data.receipts) {
      const title = receipt.kind === 'refunded' ? '확정 환불' : current.version === 1 ? '확정 가맹점 적립' : '확정 가맹점 지급';
      const receiptCard = card(title, [
        ['금액', formatAmount(receipt.amount, data.assetDecimals)],
        ['지급 또는 환불 금액 (최소 단위)', receipt.amount], ['체인', current.chain], ['배포 계약', current.contract],
        ['자산 계약', receipt.asset], ['가맹점 키', receipt.merchant], ['주문 키', receipt.order], ['환불 키', receipt.refund],
        ['거래 해시', receipt.txHash], ['로그 인덱스', receipt.logIndex], ['포함 높이', receipt.inclusionHeight],
        ['포함 시각', showTime(receipt.includedAt)], ['확정 증거 관측 시각', showTime(receipt.attestedAt)],
        ['증거 기준 높이', receipt.finalizedAnchorHeight],
        ['의미', current.version === 1 ? '적립 이벤트입니다. 인출은 별도 거래입니다.' : '거래 해시와 로그 인덱스로 실제 체인 이벤트를 확인할 수 있습니다.'],
      ]);
      if (receipt.kind === 'paid') localRecipientCheck(receiptCard, current, receipt);
      if (current.version === 1 && typeof data.legacyLedger === 'string' && data.legacyLedger.startsWith('https://')) {
        const link = node('a', '인가된 점주 인출 원장 열기');
        link.href = data.legacyLedger;
        link.rel = 'noreferrer';
        receiptCard.append(link);
      }
      if (fields.get('lookup') === 'transaction') {
        const link = node('a', '같은 주문의 지급·환불 모두 보기');
        link.href = '/?' + new URLSearchParams({stream: order.stream, merchant: order.merchant, order: receipt.order});
        receiptCard.append(link);
      }
    }
    if (data.next) {
      const more = node('button', '다음 기록');
      more.type = 'button';
      more.addEventListener('click', () => {
        more.remove();
        void renderReceipts(form, data.next);
      });
      results.append(more);
    }
  } catch (_) {
    if (controller.signal.aborted) return;
    results.replaceChildren();
    summary.textContent = '공개 지급 증거를 조회할 수 없습니다. 같은 범위를 다시 확인하세요.';
  }
}

async function renderBoundary(form) {
  if (pending) pending.abort();
  const controller = new AbortController();
  pending = controller;
  const state = byId('boundary-state');
  const evidence = byId('evidence');
  evidence.replaceChildren();
  const order = selectedOrder(form);
  if (invalidOrder(order)) {
    state.textContent = '공개 가맹점 키와 주문 키를 확인하세요.';
    return;
  }
  state.textContent = '현재 대사 증거를 조회합니다.';
  try {
    const fields = new FormData(form);
    const branch = String(fields.get('branch')).trim();
    const sale = String(fields.get('sale')).trim();
    const response = branch || sale
      ? await read('/v2/explorer/boundary', null, controller.signal, {...order, branch, sale})
      : await read('/v2/explorer/boundary', order, controller.signal);
    if (controller.signal.aborted) return;
    if (response.coverage) {
      const {coverage} = response;
      card('발급 원본과 연속 확정 범위', [
        ['서명 견적 해시', response.quoteDigest],
        ['원 견적 제출 기한', showTime(response.quoteExpiresAt)],
        ['연속 구간 시작 높이', coverage.coverageStart],
        ['마지막 검증 확정 높이', coverage.height],
        ['마지막 검증 확정 해시', coverage.hash],
        ['마지막 검증 확정 블록 시각', showTime(coverage.time)],
      ], evidence);
      if (response.kind === 'pending') {
        const short = new Date(coverage.time) < new Date(response.quoteExpiresAt);
        state.textContent = (short ? '판정 보류 · 확정 범위가 견적 마감에 닿지 않았습니다. ' :
          '판정 보류 · 이 원 견적의 지급 또는 미소비 증거를 기다립니다. ') +
          '예약·재고 해제 보류. 이 조회로 수행된 금융 조치는 0건입니다.';
        return;
      }
    }
    const item = response.evidence || response;
    if (item.kind === 'unobserved') {
      state.textContent = '판정 보류. 관측 공백만으로 확정 미지급을 입증할 수 없습니다.';
      return;
    }
    const labels = {paid: '확정 지급의 포함·확정 시각', absent: '완전 구간의 미소비 증거', invalidated: '확정 무효화'};
    const inWindow = item.kind === 'paid' && item.quoteExpiresAt && item.includedAt &&
      new Date(item.includedAt) >= new Date(item.quoteIssuedAt) && new Date(item.includedAt) < new Date(item.quoteExpiresAt);
    state.textContent = item.kind === 'paid'
      ? (inWindow ? '서명 견적의 기한 안에 포함된 지급입니다. 견적 해시와 두 시각을 원주문에 대사하세요.' :
        '포함 시각은 확인했습니다. 이 거래에서 원 견적 기한을 대조할 수 없으므로 발급자 원주문을 확인하세요.')
      : (item.kind === 'absent' && !response.coverage ? '미소비 증거의 경계를 발급자의 원 견적과 대조하세요. ' : '') +
        '이 증거를 가맹점 원장과 대조하세요. 결제·예약·재고 변경은 가맹점 원장에서 결정합니다.';
    card(labels[item.kind], [
      ['거래 해시', item.txHash], ['로그 인덱스', item.logIndex], ['포함 높이', item.inclusionHeight],
      ['서명 견적 해시', item.quoteDigest], ['원 견적 발급 시각', showTime(item.quoteIssuedAt)],
      ['원 견적 제출 기한', showTime(item.quoteExpiresAt)], ['포함 시각', showTime(item.includedAt)],
      ['증거 생성 시각', showTime(item.attestedAt)],
      ['확정 기준 높이', item.anchor], ['확정 기준 해시', item.anchorHash], ['확인 구간 시작', item.coverageStart],
      ['증거 만료', showTime(item.expiresAt)], ['미소비 증거 참조', item.proof],
    ], evidence);
  } catch (error) {
    if (controller.signal.aborted) return;
    state.textContent = error.message === '404' ? '접근 가능한 대사 범위를 확인하세요.' : '현재 대사 증거를 조회할 수 없습니다.';
  }
}

let replayRequest;
let replayReading;
async function loadReplay() {
  const deployment = byId('replay-deployment');
  const status = byId('replay-state');
  const start = byId('replay-start');
  const jobs = byId('replay-jobs');
  const stream = deployment.value;
  if (replayReading) replayReading.abort();
  replayReading = new AbortController();
  const signal = replayReading.signal;
  start.hidden = true;
  jobs.replaceChildren();
  if (!stream) {
    status.textContent = '재생할 배포를 확인하세요.';
    return;
  }
  try {
    const result = await read('/v2/explorer/replays', {stream}, signal);
    if (signal.aborted) return;
    const running = result.jobs.find(({state}) => state === 'running');
    status.textContent = running ? '재구축 결과를 기다립니다. 기존 조회를 계속 제공합니다.' : '별도 인덱스에서 검증하세요. 기존 조회를 계속 제공합니다.';
    start.hidden = !result.requestAllowed || !result.revision || !!running;
    start.dataset.revision = String(result.revision);
    for (const job of result.jobs) {
      const title = {
        running: '재생 중',
        matched: '검증 완료 · 기존 전환 권한으로 승인 요청 가능',
        diff: '불일치 · 기존 조회 유지',
      }[job.state];
      card(title, [
        ['작업 ID', job.id], ['확정 기준 높이', job.anchor], ['확정 기준 해시', job.anchorHash],
        ['재생 커서', job.replayedHeight], ['다른 로그 수', job.differingLogs],
        ['지급 차이 · 최소 단위', job.paidDelta], ['환불 차이 · 최소 단위', job.refundDelta],
        ['비교 증거 ID', job.comparison],
        ['불일치 사유', {projection_diff: '로그 또는 금액 차이', anchor_changed: '확정 체크포인트 또는 운영 revision 변경'}[job.reason]],
      ], jobs);
    }
    if (replayRequest && result.jobs.some(({id}) => id === replayRequest.id)) replayRequest = undefined;
  } catch (_) {
    if (!signal.aborted) status.textContent = '격리 재생 상태를 조회하지 못했습니다.';
  }
}

async function startReplay() {
  const stream = byId('replay-deployment').value;
  const start = byId('replay-start');
  const revision = Number(start.dataset.revision);
  if (!replayRequest || replayRequest.stream !== stream || replayRequest.revision !== revision) {
    replayRequest = {stream, revision, id: crypto.randomUUID()};
  }
  start.disabled = true;
  try {
    const response = await fetch('/v2/explorer/replays', {
      method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(replayRequest),
    });
    if (!response.ok) {
      byId('replay-state').textContent = '같은 작업의 상태와 현재 체크포인트를 다시 확인하세요.';
      return;
    }
    await loadReplay();
  } catch (_) {
    byId('replay-state').textContent = '요청 결과를 확인 중입니다. 같은 작업을 다시 요청할 수 있습니다.';
  } finally {
    start.disabled = false;
  }
}

if (operations) {
  byId('refresh').addEventListener('click', () => void loadProfiles());
  byId('replay-deployment').addEventListener('change', () => void loadReplay());
  byId('replay-refresh').addEventListener('click', () => void loadReplay());
  byId('replay-start').addEventListener('click', () => void startReplay());
  byId('boundary-form').addEventListener('submit', (event) => {
    event.preventDefault();
    void renderBoundary(event.currentTarget);
  });
} else {
  deployment.addEventListener('change', () => {
    if (pending) pending.abort();
    results.replaceChildren();
    summary.textContent = status(currentProfile());
  });
  const form = byId('receipt-form');
  publicSearch(form);
  form.elements.lookup.addEventListener('change', () => publicSearch(form));
  form.addEventListener('input', () => {
    if (pending) pending.abort();
    results.replaceChildren();
  });
  form.addEventListener('submit', (event) => {
    event.preventDefault();
    void renderReceipts(event.currentTarget);
  });
  const links = new URLSearchParams(location.search);
  void loadProfiles().then((available) => {
    if (available && profiles.some(({stream}) => stream === links.get('stream')) && links.has('merchant') && (links.has('order') || links.has('txHash'))) {
      deployment.value = links.get('stream');
      form.elements.merchant.value = links.get('merchant');
      form.elements.lookup.value = links.has('txHash') ? 'transaction' : 'order';
      form.elements.order.value = links.get('txHash') || links.get('order');
      form.elements.logIndex.value = links.get('logIndex') ?? '';
      publicSearch(form);
      void renderReceipts(form);
    }
  });
}
if (operations) void loadProfiles();

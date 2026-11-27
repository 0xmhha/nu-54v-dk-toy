// The board has no screen, so the app tells the owner which board step is running and which
// buttons to press. Steps, buttons and LED patterns mirror the design book
// (docs/content/commercial/screens/board.json); test/boardSteps.test.ts keeps the two in sync.
// The phone never shows what is being approved: content checks belong to the independent display.

export type BoardButton = 'SW1' | 'SW2' | 'SW3' | 'SW4' | 'SW1+SW2';
export type BoardStepId = 'window' | 'display' | 'approve' | 'pin' | 'recovery' | 'recoveryproof' | 'hold' |
  'update' | 'confirm' | 'working' | 'locate' | 'done' | 'failed';
export type BoardFlowId = 'register' | 'sign' | 'order' | 'address' | 'return' | 'send' | 'reset' | 'replace' | 'backup' | 'install' | 'update' | 'locate' | 'erase';

export type BoardStep = {label: string; do: string; buttons: [BoardButton, string][]; led?: string; app: string; pending?: string};
export type BoardWait = {flow: BoardFlowId; step: BoardStepId; /** Replaces the step's own note. */ note?: string};

export const boardSteps: Record<BoardStepId, BoardStep> = {
  window: {label: '연결 창 열기', do: '보드의 SW4를 1초 이상 길게 누르세요.',
    buttons: [['SW4', '1초 이상 길게 · 연결·등록 창']],
    app: '보드가 연결 창을 열면 앱이 주변 기기 목록에서 찾습니다.'},
  display: {label: '독립 표시 대조', do: '독립 표시 장치에서 요청 내용을 대조하세요. 송금이면 금액·받는 곳·네트워크를 봅니다.', buttons: [],
    app: '이 휴대전화 화면은 대조 기준이 아닙니다. 독립 표시와 다르면 보드에서 SW2로 거절하세요.'},
  approve: {label: '보드 승인', do: '승인하려면 SW1, 거절하려면 SW2를 짧게 누르세요.',
    buttons: [['SW1', '짧게 · 승인'], ['SW2', '짧게 · 거절']], led: '빠르게 깜박임(0.2초 간격)',
    app: '길게 누르면 승인으로 처리되지 않습니다. 내용 대조가 필요한 요청은 독립 표시 장치에서 먼저 대조합니다.'},
  pin: {label: '보드 PIN 입력', do: '보드 버튼으로 4자리 PIN을 입력하세요. 45초 안에 끝내야 하고, 처음 등록할 때는 같은 PIN을 한 번 더 입력해 확인합니다.',
    buttons: [['SW1', '짧게 · 현재 자리 숫자 올림'], ['SW3', '짧게 · 다음 자리'], ['SW2', '짧게 · 모두 지우기']],
    led: '시작할 때 두 번, 누를 때마다 짧게, 다음 자리로 넘어갈 때 0.5초',
    app: 'PIN은 보드에서만 입력하며 숫자와 입력 위치는 휴대전화로 전송되지 않습니다. 앱은 PIN 단계라는 것과 버튼 사용법만 안내합니다.'},
  recovery: {label: '복구 창 열기', do: '원래 보드의 SW3을 1초 이상 길게 누르세요.',
    buttons: [['SW3', '1초 이상 길게 · 복구 창']], app: '복구 창이 열린 뒤 앱에서 같은 보드에 다시 연결합니다.'},
  recoveryproof: {label: '독립 복구 장치 승인', do: '등록된 독립 복구 장치에서 새 휴대전화의 지문을 대조하고 승인하세요.', buttons: [],
    app: '독립 복구 장치가 승인해야 보드가 버튼 입력을 기다립니다.'},
  hold: {label: '백업·복원 물리 확인', do: '보드의 SW3을 1초 이상 길게 누른 뒤 PIN을 입력하세요.',
    buttons: [['SW3', '1초 이상 길게 · 물리 확인']], app: '복구 자료는 독립 복구 장치에만 표시됩니다. 휴대전화에는 나타나지 않습니다.'},
  update: {label: '유선 정비 창', do: '전원을 켤 때 SW1과 SW2를 함께 누른 채 유지하세요. 전송이 끝나면 SW3을 길게 눌러 새 이미지를 시험 부팅합니다.',
    buttons: [['SW1+SW2', '전원을 켤 때 함께 유지 · 정비 창'], ['SW3', '1초 이상 길게 · 시험 부팅']],
    led: '정비 창 동안 두 표시등이 켜져 있음', app: '정비 창은 2분 동안 열립니다.'},
  confirm: {label: '보드 물리 확인', do: '보드에서 물리 확인을 받습니다.', buttons: [],
    pending: '이 동작의 버튼 배치는 펌웨어에 아직 구현되지 않았습니다.', app: '구현 전까지 이 단계는 시연할 수 없습니다.'},
  working: {label: '보드 처리 중', do: '보드가 처리하는 동안 버튼을 누르지 마세요.', buttons: [],
    app: '끝나면 앱이 보드 상태를 다시 읽습니다.'},
  locate: {label: '찾기 신호', do: '보드의 표시등을 확인하세요.', buttons: [], led: '0.4초씩 네 번 켜짐',
    app: '찾기 신호는 앱에서 취소할 수 있습니다.'},
  done: {label: '완료', do: '보드 작업이 끝났습니다.', buttons: [],
    led: '승인 대기 깜박임이 멈춥니다. 백업·복원은 30초 동안 0.5초 간격으로 깜박입니다.',
    app: '결과는 앱이 보드와 원장에서 다시 확인한 뒤 표시합니다.'},
  failed: {label: '요청 종료', do: '요청이 거절되거나 만료되어 끝났습니다. 같은 요청을 다시 열어 상태를 확인하세요.', buttons: [],
    led: '승인 대기 깜박임이 멈춥니다. 백업·복원 거절은 짧게 세 번 깜박입니다.',
    app: '거절·만료된 요청은 다시 승인할 수 없습니다.'},
};

export const boardFlows: Record<BoardFlowId, {label: string; steps: BoardStepId[]}> = {
  register: {label: '보드 등록', steps: ['window', 'approve', 'pin', 'done']},
  sign: {label: '내용 대조 후 서명', steps: ['display', 'approve', 'pin', 'done']},
  // Not in the design book yet: in the spending profile the phone shows the request, so no display step.
  order: {label: '주문 결제', steps: ['approve', 'pin', 'done']},
  address: {label: '반납 주소 저장', steps: ['approve', 'pin', 'done']},
  send: {label: '보내기', steps: ['approve', 'pin', 'done']},
  return: {label: '반납', steps: ['approve', 'pin', 'done']},
  reset: {label: '보드 초기화', steps: ['approve', 'pin', 'done']},
  replace: {label: '휴대전화 교체', steps: ['recovery', 'recoveryproof', 'approve', 'pin', 'done']},
  backup: {label: '백업·복원', steps: ['hold', 'pin', 'working', 'done']},
  install: {label: '이미지 기록', steps: ['working', 'done']},
  update: {label: '펌웨어 적용', steps: ['update', 'working', 'done']},
  locate: {label: '기기 찾기', steps: ['locate']},
  erase: {label: '개인 슬롯 삭제', steps: ['confirm', 'working', 'done']},
};

/** Position of the step in its flow, or undefined for an outcome outside the sequence. */
export function boardPosition(wait: BoardWait): {index: number; total: number} | undefined {
  const order = boardFlows[wait.flow].steps;
  const index = order.indexOf(wait.step);
  return index < 0 ? undefined : {index, total: order.length};
}

/** Steps after the current one that need a button, so the owner sees what comes next. */
export function boardStepsAhead(wait: BoardWait): BoardStepId[] {
  const position = boardPosition(wait);
  if (!position) return [];
  return boardFlows[wait.flow].steps.slice(position.index + 1).filter(step => boardSteps[step].buttons.length > 0);
}

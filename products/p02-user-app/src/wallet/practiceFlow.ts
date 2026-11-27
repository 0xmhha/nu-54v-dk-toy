export type Practice = {scenario: 'matching' | 'wrongAmount'; state: 'practice' | 'done' | 'reject'};
export function beginPractice(scenario: Practice['scenario']): Practice { return {scenario, state: 'practice'}; }
export function practiceDecision(practice: Practice, decision: 'approve' | 'reject'): Practice {
  if (practice.state !== 'practice') return practice;
  if (decision === 'reject') return {...practice, state: 'reject'};
  return practice.scenario === 'matching' ? {...practice, state: 'done'} : practice;
}
export function readPractice(raw: string): Practice | undefined {
  try {
    const value = JSON.parse(raw);
    if (value && Object.keys(value).length === 2 && ['matching','wrongAmount'].includes(value.scenario) &&
        ['practice','done','reject'].includes(value.state) && !(value.scenario === 'wrongAmount' && value.state === 'done')) return value as Practice;
  } catch {}
  return undefined;
}

const practiceOrder = {amount: 2, mismatchedAmount: 5, fee: 0};

export function practiceOrderText(language: 'ko' | 'en', scenario: Practice['scenario'] = 'matching') {
  const ko = language === 'ko';
  const coins = (amount: number) => ko ? `${amount} 연습 코인` : `${amount} practice coins`;
  return {
    recipient: ko ? '연습 카페' : 'Practice Café',
    amount: coins(practiceOrder.amount),
    comparisonAmount: coins(scenario === 'matching' ? practiceOrder.amount : practiceOrder.mismatchedAmount),
    chain: ko ? '연습 모드' : 'Practice mode',
    fee: String(practiceOrder.fee),
  };
}

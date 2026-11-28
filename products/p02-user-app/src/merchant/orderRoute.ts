import {entranceAnchor} from './customerEntrance.ts';
import {approvedCustomerEntrance, approvedOrderPayment} from './customerEntranceDeployment.ts';
import {orderPaymentPin} from './orderPaymentTrust.ts';

/**
 * Whether this build orders without a store registration: both its customer entrance and its order contract are
 * bound (customerEntranceDeployment.ts). Such a build never asks a customer to register a phone or a board at a
 * store, and shows no way to. A build without either keeps the earlier route, registration and all; the repository
 * ships that one.
 */
export const ordersWithoutRegistration = (anchor = approvedCustomerEntrance, payment = approvedOrderPayment): boolean =>
  !!entranceAnchor(anchor) && !!orderPaymentPin(payment);

const notInThisBuild = '이 앱에서는 아직 쓸 수 없는 기능입니다.';
/**
 * What the earlier route says where it sends the customer to register a phone or a board at a store, and what a
 * build without that route says in its place. Such a build has no registration form and enrolls no board, so the
 * first sentence would point at a way that does not exist. Sending, returning and test tokens still go through a
 * store's registered connection, which such a build does not set up yet: it says only that.
 */
const withoutRegistrationWords: Record<string, string> = {
  '먼저 매장 주문 등록이 필요합니다. 결제 탭에서 매장 담당자가 준 등록서를 입력하세요.': notInThisBuild,
  '매장 등록을 확인하지 못했습니다. 등록했다면 다시 확인하고, 아니면 결제 탭에서 매장 담당자가 준 등록서를 입력하세요.': notInThisBuild,
  '이 휴대전화의 주문 등록으로는 받을 수 없습니다. 매장 담당자에게 새 등록서를 받으세요.': notInThisBuild,
  '이 휴대전화의 주문 등록으로는 할 수 없습니다. 매장 담당자에게 새 등록서를 받으세요.': notInThisBuild,
  '주문 등록이 만료되었습니다. 매장 담당자에게 새 등록서를 받으세요.': notInThisBuild,
  '이 매장에 보드 계정이 등록되어 있지 않습니다. 설정에서 매장 등록을 먼저 하세요.': '매장 서버가 이 보드 계정을 알지 못해 진행할 수 없습니다.',
  '다른 보드 계정이 이 매장에 등록되어 있습니다. 설정에서 매장 등록을 다시 하세요.': '매장 서버가 다른 보드 계정을 알고 있어 진행할 수 없습니다.',
  '이 매장에 등록된 보드 계정이 아니거나 서버가 서명을 받아들이지 않았습니다. 설정에서 매장 등록을 다시 하세요.': '매장 서버가 이 보드 계정을 알지 못하거나 서명을 받아들이지 않았습니다.',
  '매장 등록을 확인하고 있습니다.': '확인하는 중',
  '네트워크 주소나 매장 등록이 없어 잔액을 읽지 못했습니다. 보낸 돈은 원래 지갑의 거래 기록에서 확인하세요.': '잔액을 읽지 못했습니다. 보낸 돈은 원래 지갑의 거래 기록에서 확인하세요.',
  '보드 계정의 잔액을 확인할 수 없습니다. 네트워크 주소와 매장 등록을 확인하세요.': '보드 계정의 잔액을 확인할 수 없습니다. 네트워크 주소를 확인하세요.',
  '서명 시험, 반납 주소, 매장 등록': '서명 시험, 반납 주소',
  'Signing test, return address, store enrollment': 'Signing test, return address',
};

/**
 * What the pages that move the board account's money say in a build that orders from store pages, on a phone no
 * store registered: in place of the action, that this app cannot do it yet and what the customer can do instead.
 * Nothing in such a build sends the money back to the wallet it came from, so no page says to return it first;
 * the reset page says that what is left is lost (`reset`, after its own first sentences).
 */
export const notHereYet = {
  return: {title: '이 앱에서는 아직 반납할 수 없습니다', body: '보드 계정의 돈을 원래 지갑으로 돌려보내는 기능이 이 앱에는 아직 없습니다. 남은 돈은 매장 주문에 쓸 수 있습니다.'},
  send: {title: '이 앱에서는 아직 보낼 수 없습니다', body: '보드 계정의 돈을 다른 주소로 보내는 기능이 이 앱에는 아직 없습니다. 남은 돈은 매장 주문에 쓸 수 있습니다.'},
  faucet: {title: '이 앱에서는 아직 시험 토큰을 받을 수 없습니다', body: '원래 지갑에서 보드 계정 주소로 입금하세요. 주소는 지갑 탭의 받기에서 볼 수 있습니다.'},
  reset: '이 앱에서는 아직 반납할 수 없어, 남은 돈을 원래 지갑으로 돌려받을 방법이 없습니다.',
};

/** `text` as this build says it: unchanged on the earlier route, and without the way to a registration elsewhere. */
export const routeWords = (text: string, withoutRegistration = ordersWithoutRegistration()): string =>
  withoutRegistration ? withoutRegistrationWords[text] ?? text : text;

import {paymentProfile} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';
import {storeOf, type Store} from './tokens.ts';

/** The registered store of any kind of contract: only for showing amounts, never for signing. */
export async function registeredStore(): Promise<Store | undefined> {
  const raw = Http ? await Http.loadProfile('customer') : '';
  const policy = raw ? paymentProfile(raw) : undefined;
  return policy && storeOf(policy, (JSON.parse(raw) as {scope?: {branchRef?: string}}).scope?.branchRef);
}

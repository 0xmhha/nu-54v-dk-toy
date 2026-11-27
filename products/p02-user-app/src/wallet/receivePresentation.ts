import {xrplIssue} from './nonEvmAssets.ts';
import qrcode from 'qrcode-generator';
import {checkSegwitV0, checkXrplClassic, decodePublicBase58, xrplXAddress} from './chainAccounts.ts';
import {normalizeEvmAddress} from './evmTransfer.ts';

type ReceiveProfile =
  | {kind: 'btc'; hrp: 'bc' | 'tb' | 'bcrt'; network: string}
  | {kind: 'evm'; network: string; chainId: string; symbol: string; token?: string}
  | {kind: 'sol'; network: string; mint?:string}
  | {kind: 'xrp'; network: string; test: boolean; tag?: string; issue?:{currency:string;issuer:string}};

export function presentReceive(profile: ReceiveProfile, address: string) {
  let protocolUri: string | undefined, asset: string, xAddress: string | undefined;
  if (profile.kind === 'btc') {
    address = checkSegwitV0(profile.hrp, address);
    asset = 'BTC';
    protocolUri = 'bitcoin:' + address + '?label=' + encodeURIComponent(profile.network);
  } else if (profile.kind === 'evm') {
    address = normalizeEvmAddress(address);
    if (!/^[1-9][0-9]{0,19}$/.test(profile.chainId)) throw new Error('RECEIVE_CHAIN');
    const token = profile.token ? normalizeEvmAddress(profile.token) : undefined;
    asset = token ? 'ERC-20 ' + token : profile.symbol;
    if (!token) protocolUri = 'ethereum:' + address + '@' + profile.chainId;
  } else if (profile.kind === 'sol') {
    decodePublicBase58(address, 32);
    if (profile.mint) decodePublicBase58(profile.mint,32);
    asset = profile.mint ? 'SPL mint ' + profile.mint : 'SOL';
  } else {
    checkXrplClassic(address);
    if (profile.issue) xrplIssue(profile.issue.currency,profile.issue.issuer);
    asset = profile.issue ? profile.issue.currency + ' · Issuer: ' + profile.issue.issuer : 'XRP';
    xAddress = xrplXAddress(address, profile.tag, profile.test);
  }
  const detail = [profile.network, 'Address: ' + address,
    ...(profile.kind === 'xrp' && profile.tag !== undefined ? ['DestinationTag: ' + profile.tag] : []),
    'Asset: ' + asset,
    ...(profile.kind === 'sol' && profile.mint ? ['Address type: wallet owner'] : []), ...(xAddress ? ['X-address: ' + xAddress] : [])].join('\n');
  return {address, detail, payload: protocolUri ?? detail, share: protocolUri ? detail + '\n' + protocolUri : detail};
}

export function receiveQr(payload: string) {
  const matrix = qrcode(0, 'M');
  matrix.addData(payload, 'Byte');
  matrix.make();
  return matrix.createDataURL(5, 20);
}

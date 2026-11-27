import type {ReadProfileId} from './readProfiles.ts';

export const walletCapabilities = {
  stable: {fee:'Native gas', read:'Native asset and verified ERC-20', approvals:'Registered StableNet transfers and merchant payments'},
  eth: {fee:'ETH · EIP-1559', read:'ETH and verified ERC-20', approvals:'Registered ETH and ERC-20 transfers, SIWE and scoped allowance'},
  base: {fee:'ETH · L1 data and L2 execution', read:'ETH and verified ERC-20', approvals:'Registered ETH and ERC-20 transfers under the signed Base policy'},
  btc: {fee:'sat/vB', read:'Signet receive address, funded balance and watch-only history', approvals:'Registered Signet · P2WPKH transfer and 2-of-3 PSBT'},
  sol: {fee:'SOL · lamports', read:'Devnet native SOL', approvals:'Registered Devnet · native SOL transfer'},
  xrp: {fee:'XRP · drops', read:'Testnet native XRP and validated activation status', approvals:'Registered Testnet · native XRP Payment with destination tag'},
} satisfies Record<ReadProfileId,{fee:string; read:string; approvals:string}>;

export const unsupportedWalletMethods = [
  {id:'stake',name:'Staking · DelegateStake',description:'The wallet has no verified staking transaction review.'},
  {id:'nft',name:'NFT · ERC-721 SafeTransferFrom',description:'The wallet has no verified ERC-721 transaction review.'},
  {id:'batch',name:'Batch · multicall(bytes[])',description:'The wallet has no verified multicall transaction review.'},
] as const;

export const advancedRequestRefused = {
  stake:'스테이킹 요청을 거절했습니다.',
  nft:'NFT 전송 요청을 거절했습니다.',
  batch:'복합 호출 요청을 거절했습니다.',
} as const;

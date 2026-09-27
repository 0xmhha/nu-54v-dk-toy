# contracts-abi (planned)

P06 계약의 ABI와 배포 manifest를 P04, P05, P07이 같은 값으로 읽게 하는 공유 package다.
P06 인터페이스를 W6에 고정한 뒤(WBS2-P06) 만든다. 그 전에는 이 README만 둔다.

| 항목 | 계획 |
|---|---|
| 원본 | `products/p06-stablenet-contracts/out/`의 Foundry 빌드 산출물, 배포 manifest(주소, chainId, 배포 block, code hash) |
| 산출물 | Go(`abigen`), TypeScript, Python reader |
| 소비 제품 | P04, P05, P07 |
| 검증 | 생성물이 원본과 같은지 `--check`로 확인하고, P10 적합성 harness에 추가한다 |

실제 key, 관리자 secret, 배포 계정 정보는 저장하지 않는다.

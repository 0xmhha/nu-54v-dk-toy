# contracts-abi

정산 컨트랙트의 ABI를 키오스크(TypeScript), 운영 도구와 indexer(Go)가 같은 값으로 쓰게 하는 공유 package다. 원본은 `products/p06-stablenet-contracts`의 Foundry 빌드이고, 이 폴더의 파일은 모두 생성물이다.

| 경로 | 내용 |
|---|---|
| `abi/*.json` | ABI JSON(키 정렬) |
| `abi/manifest.json` | ABI별 sha256과 6주차 게이트에 고정한 인터페이스 표시(`frozenAtW6`) |
| `ts/src/` | `@nu54/contracts-abi`: `paymentSettlementAbi`, `merchantRegistryAbi`, `testUsdcAbi` (`as const`) |
| `go/` | go-ethereum abigen 바인딩: `settlement`, `registry`, `testusdc` 패키지 |
| `tools/generate.py` | 생성기. `--check`는 커밋된 파일이 새로 만든 결과와 같은지 확인한다 |

6주차에 고정하는 인터페이스는 `IPaymentSettlement`(결제, 예치, cashOut)와 `IMerchantRegistry`(등록, 철회, 조회)다. 한도 변경, 출금, 반납은 `IPaymentSettlementExtensions`로 나중에 추가하며, 고정한 ABI의 함수·이벤트·오류는 바꾸지 않는다.

```bash
make build   # 컨트랙트를 빌드하고 ABI, TS, Go 바인딩을 다시 만든다
make test    # 생성물이 최신인지 확인하고 Go 바인딩을 빌드한다
```

배포 주소는 여기에 두지 않는다. 테스트넷 배포 결과는 `products/p06-stablenet-contracts/deployments/`에 기록한다. 키와 관리자 secret은 저장하지 않는다.

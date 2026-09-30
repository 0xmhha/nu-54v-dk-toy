# contracts-abi

정산 컨트랙트의 ABI를 키오스크(TypeScript), 운영 도구와 indexer(Go)가 같은 값으로 쓰게 하는 공유 package다. 원본은 `products/p06-stablenet-contracts`의 Foundry 빌드이고, 이 폴더의 파일은 모두 생성물이다.

| 경로 | 내용 |
|---|---|
| `abi/*.json` | ABI JSON(키 정렬) |
| `abi/manifest.json` | ABI별 sha256과 6주차 게이트에 고정한 인터페이스 표시(`frozenAtW6`) |
| `ts/src/` | `@nu54/contracts-abi`: `paymentSettlementAbi`, `merchantRegistryAbi`, `paymentSettlementExtensionsAbi`, `merchantRegistryExtensionsAbi`, `testUsdcAbi` (`as const`) |
| `go/` | go-ethereum abigen 바인딩: `settlement`, `registry`, `settlementext`, `registryext`, `testusdc` 패키지 |
| `tools/generate.py` | 생성기. `--check`는 커밋된 파일이 새로 만든 결과와 같은지 확인한다 |

6주차에 고정하는 인터페이스는 `IPaymentSettlement`(결제, 예치, cashOut)와 `IMerchantRegistry`(등록, 철회, 조회)다. 한도 변경, 출금, 반납, 가맹점 대리 cashOut(`cashOutFor`)은 `IPaymentSettlementExtensions`, payout 변경 지연은 `IMerchantRegistryExtensions`에 있다. 두 확장은 배포 컨트랙트에 구현되어 있지만 고정 대상이 아니며, 고정한 ABI의 함수·이벤트·오류는 바꾸지 않는다. 같은 주소에 고정 ABI와 확장 ABI를 함께 붙여 쓴다.

```bash
make build   # 컨트랙트를 빌드하고 ABI, TS, Go 바인딩을 다시 만든다
make test    # 생성물이 최신인지 확인하고 Go 바인딩을 빌드한다
```

## 사용법

**Go (운영 도구, indexer):** `go.work`에 이 모듈이 들어 있으므로 import만 하면 된다. 예제는 [`go/examples/example_test.go`](go/examples/example_test.go)에 있고 CI에서 컴파일된다.

```go
import "github.com/0xmhha/nu-54v-dk-toy/packages/contracts-abi/go/registry"

reg, err := registry.NewRegistry(registryAddress, client) // client: *ethclient.Client
tx, err := reg.RegisterMerchant(adminOpts, merchant, payout) // adminOpts: keystore를 secretRef로 연 TransactOpts
```

**TypeScript (키오스크, 폰 앱):** pnpm 작업공간 의존성으로 추가한다(`"@nu54/contracts-abi": "workspace:*"`). ABI는 `as const`라서 viem 같은 라이브러리가 함수 이름과 인자 타입을 추론한다.

```ts
import { paymentSettlementAbi } from "@nu54/contracts-abi";

// 예: viem으로 settle 호출 데이터 만들기
const data = encodeFunctionData({ abi: paymentSettlementAbi, functionName: "settle", args: [authorization, signature] });
```

`settle`의 오류는 ABI의 custom error(`WrongDomain`, `OrderAlreadyPaid`, `Expired` 등)로 디코딩해 키오스크 설계 7절의 결과로 매핑한다.

배포 주소는 여기에 두지 않는다. 테스트넷 배포 결과는 `products/p06-stablenet-contracts/deployments/`에 기록한다. 키와 관리자 secret은 저장하지 않는다.

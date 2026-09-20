# stable-poc-contract 재사용 검토

> 2026-09-18 범위 갱신: [12주 완료 범위 v3](twelve-week-completion-scope-v3.md)가 최신 기준이다. 아래 기존 검토에서 패스키·녹음·스마트 계정 등을 후보/후속으로 둔 부분은 새 범위를 따른다. 초기 EOA 개발 순서와 고객 가스 부담은 유지하며, 새 Cloud Wallet의 MPC는 별도로 설계한다.

작성: 2026-09-17. 사용자가 [0xmhha/stable-poc-contract](https://github.com/0xmhha/stable-poc-contract)를 DApp 개발에 활용할 수 있는 기존 자산으로 제공했다. 검토 기준은 `5d9d6550ef572bd86a036fe92c730e4ff0a1ca94`다. 소스와 설정을 읽었으며 의존성 설치·빌드·테스트·배포는 실행하지 않았다. 아래 도입 순서와 기능 선택은 설계 제안이다.

## 1. 결론

후속으로 [poc-platform](poc-platform-reuse-plan.md)이 제공되어 Bundler·Paymaster Proxy·지갑 SDK 구현도 확인했다. 아래 ‘연결 인프라 필요’는 이제 기존 서비스 재사용을 우선 검토한다는 의미다. 다만 플랫폼의 Paymaster envelope/서명 형식과 이 커밋의 로컬 VerifyingPaymaster 형식이 달라, 실제 대응 버전을 선정하는 작업이 필요하다.

이 저장소는 Solidity/Foundry 기반 스마트 계정·서명 검증·Paymaster 등 **온체인 기능의 재사용 기반**이다. 기존 Indexer와 함께 활용할 수 있다. React Native 고객/매장 앱이나 주문·대여 백오피스가 이미 완성되어 있다는 의미는 아니다.

초기 일반 지갑(EOA)으로 더미 토큰과 기존 Indexer를 연결하고, 스마트 계정과 운영자 후원은 후속 단계로 추가한다는 사용자 결정을 반영했다. 사용자가 저장소를 제공한 것을 모든 DeFi·구독·프라이버시 기능의 도입 승인으로 해석하지 않는다.

## 2. 실제 코드에서 확인한 재사용 후보

아래 링크는 검토 커밋에 고정한다.

| 구현 | 활용 후보 | 적용 조건 |
|---|---|---|
| [ERC20Mock](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/test/mocks/ERC20Mock.sol#L10) | 소수점을 지정하는 더미 토큰의 기반 | `mint`와 타인 주소 대상 `burn`에 접근 제한이 없다. 공동 시연용은 발행 권한을 제한하고 임의 소각 경로를 제거/제한한 별도 테스트 토큰으로 정리 |
| [Kernel](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/erc7579-smartaccount/Kernel.sol#L229), [ECDSAValidator](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/erc7579-validators/ECDSAValidator.sol#L49) | 보드의 서명을 검증하는 스마트 계정 | 계정 주소·보드 키 주소·서명 데이터 형식을 구분하고 실제 배포 버전과 맞춰 시험 |
| [VerifyingPaymaster](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/paymaster/VerifyingPaymaster.sol#L24) | 고객이 가스 토큰을 준비하지 않아도 되는 후원 방식의 후보 | 후원자 서명 서비스, 예치 자금, 한도·남용 방지, Bundler 등 연결 인프라 필요. 초기 고객 부담, 운영자 후원은 후속 단계로 확정. 구체적인 후원 정책은 미정 |
| [ERC20Paymaster](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/paymaster/ERC20Paymaster.sol#L199) | 토큰으로 수수료를 부담하는 후속 후보 | 잔액·allowance·가격 변환·수수료 정산을 추가 검증. 6자리 토큰의 최소 단위까지 확인 |
| [SpendingLimitHook](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/erc7579-hooks/SpendingLimitHook.sol#L76) | 여행 지갑의 지출 한도 기능 후보 | 코드상 실행 전후 순잔액 감소를 계산한다. 모든 외부 출금이나 승인 경로를 포괄하는 한도라고 가정하지 않고 실행 경로별 검증 필요 |

README의 기능 목록보다 실제 파일과 배포 import를 기준으로 범위를 정했다. 테스트 파일 존재는 확인했지만 테스트 통과나 배포 환경에서의 동작을 검증한 것은 아니다. 더미 토큰의 접근 제어 지적도 전체 보안 감사 결과가 아니다.

6자리 토큰을 생성하는 [기존 테스트](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/test/stability/PegStabilityModule.t.sol#L67)는 있다. 다만 검토한 `script/`에서는 더미 USDC를 생성하는 스크립트를 찾지 못했고, 안정화 모듈 배포는 기존 `USDC_ADDRESS`를 입력받는다. 시연용 토큰 배포·초기 발행·주소 기록 스크립트가 추가 작업이다. 이 OpenZeppelin mock에는 ERC20Permit 상속이 없으며, 검토한 `src/test/script`에서 EIP-3009 전송 구현도 확인하지 못했다. 해당 기능을 쓰기로 결정하면 별도 구현·호환 시험이 필요하다. `WKRW`는 native coin 예치 기반 토큰이므로 더미 USDC와 구분한다.

## 3. 기존 지갑 가져오기와 스마트 계정의 관계

니모닉/private key 가져오기 요구는 그대로 유지한다. 앱에서 가져온 키를 보드로 안전하게 전달하고 보드가 고객 지출을 승인하는 기존 방향도 유지한다.

스마트 계정의 서명자로 기존 키를 사용하더라도, **기존 키의 일반 지갑 주소에 있던 자산이 새 스마트 계정 주소로 자동 이동하지 않는다.** 일반 지갑으로 직접 결제할지, 스마트 계정에 자산을 넣어 결제할지는 별도 사용자 흐름이다. EIP-7702 경로를 선택한다면 체인 지원·위임 승인·해제와 키 가져오기 흐름을 별도로 설계한다. 저장소의 EIP-7702 표기만으로 기존 주소 재사용을 보장하지 않는다.

따라서 이번 단계에서는 기존 지갑 가져오기를 새 스마트 계정 생성으로 대체하지 않는다. 일반 전송은 초기 경로, 스마트 계정은 후속 경로로 기록한다. 기존 주소 유지·자산 이동을 포함한 전환 방식은 아직 미정이다. 스마트 계정 서명 지원도 NU-54V-DK의 보호 영역 내 서명·화면 확인 검증을 대신하지 않는다.

## 4. Indexer 연동에 추가되는 사항

일반 전송은 주문·결제 시도와 거래 해시 및 전송 이벤트를 연결한다. ERC-4337 경로를 채택하면 여기에 `userOpHash`, 스마트 계정 주소, 실제 번들 거래 해시를 추가한다.

`UserOperationEvent`는 개별 작업의 `success`를 제공한다. 번들 거래 영수증 성공만으로 주문을 완료하지 않고, 해당 작업 성공과 예상한 토큰·수취 주소·금액의 전송이 같은 작업에 속하는지 확인해야 한다. 한 번들에 여러 작업이 들어갈 수 있으므로 거래 해시만으로 전송을 연결하지 않는다. [EntryPoint 이벤트 정의](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/erc4337-entrypoint/interfaces/IEntryPoint.sol#L30)

앞서 확인한 Indexer 백엔드에는 `userOperation` 조회 필드가 있다. 선택한 EntryPoint·이벤트 버전, 작업별 로그 연결, 누락 복구까지 호환되는지는 통합 시험으로 확인한다. 조회 구현이 Bundler의 제출 기능을 대신하는 것은 아니다. [Indexer 조회 정의](https://github.com/0xmhha/indexer-go/blob/5baff647f3cfab935f73db1bbd742ad42317045f/pkg/api/graphql/schema.go#L1948)

## 5. 배포 전에 맞춰야 할 의존성

- [foundry.toml](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/foundry.toml)은 Solidity 0.8.28, EVM `prague`를 지정한다. 대상 체인과 필요한 opcode/기능 지원을 확인한다.
- [DeployKernel](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/script/DeployKernel.s.sol#L8)은 로컬 `src/erc7579-smartaccount` 대신 `@kernel` 경로를 사용한다. [DeployEntryPoint](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/script/DeployEntryPoint.s.sol#L5)도 `@account-abstraction`을 사용한다. 내부 파일의 기능이 해당 스크립트로 그대로 배포된다고 가정하지 않는다.
- 검토 커밋의 submodule 포인터는 Kernel `6a098bc8f9a7001fe7ae801368e96a5ee635ddb1`, account-abstraction `1c6b669d0eea734e09a87e095ba15e076151718a`다. 이번 검토에서 submodule 소스를 내려받아 빌드하지는 않았다.
- 선택한 Kernel·EntryPoint·Paymaster·Bundler·앱 인코딩·Indexer 디코딩의 호환 버전을 고정한다. 스크립트와 배포 기록 존재만으로 현재 네트워크의 실행 코드를 확인했다고 보지 않는다.
- 후원 방식에서도 [Paymaster 예치](https://github.com/0xmhha/stable-poc-contract/blob/5d9d6550ef572bd86a036fe92c730e4ff0a1ca94/src/paymaster/BasePaymaster.sol#L80)가 필요하다. 더미 토큰 발행과 가스비 조달은 별도다.

## 6. 요구사항 결정 상태

확정: 이 저장소를 재사용 가능한 기존 자산에 추가한다. 더미 토큰은 개발·시연용이다. 기존 Indexer 두 저장소를 활용한다.

제안: 테스트 토큰 정리 → 기본 전송과 주문 대사 → 후속 스마트 계정·운영자 후원을 추가한다. 이 두 후속 기능 사이의 상세 도입 순서는 별도 결정한다. 고객 부담 수수료가 먼저라는 사용자 결정을 반영한다. 정기 결제·DEX·브리지·대출 등은 이번 카페 결제의 필수 범위로 편입하지 않는다.

미정: 후속 스마트 계정 전환 방식, 가스 확보·후원 정책, 기존 Bundler·후원 서비스의 가용성, 12주 실자산 파일럿 포함 여부. 체인과 네이티브 가스 지불은 후속 답변으로 확정했다. [네트워크 확인](stablenet-testnet-baseline.md)

# 배포 스크립트

Foundry 배포 스크립트(`*.s.sol`)와 테스트넷 계정 도구를 둔다. `Deploy.s.sol`은 시험 토큰, registry, 정산 컨트랙트를
배포하고, `SoftwareSettle.s.sol`은 소프트웨어 서명으로 결제 1건을 처리한다(WBS2-P06-02). 개인키는 저장소에 두지 않고,
아래 역할 계정 도구로 만든 암호화 keystore를 secretRef(keystore 경로와 Keychain에 둔 암호)로 연다 [N32].

## 테스트넷 역할 계정

`testnet-accounts.sh`가 역할 계정 6개를 암호화 keystore로 만든다: 배포자(deployer), 운영자(operator), registry 관리자(registry-admin), 시험 토큰 owner(token-owner), 키오스크 가스(kiosk), 소프트웨어 기기 키(device, 6주차 게이트 전용).

- keystore는 저장소 밖 `~/.nu54/keystores/`에 두고, 폴더와 파일 권한을 본인만 읽게 한다.
- keystore 암호는 역할마다 무작위로 만들어 macOS 로그인 Keychain(서비스 `nu54-<role>`)에만 둔다. 암호를 외우거나 입력하지 않는다.
- 공개 주소는 `~/.nu54/testnet-accounts.env`에 `NU54_ADDR_<ROLE>=0x…` 형식으로 모은다.
- 이미 있는 keystore는 덮어쓰지 않는다. `--dry-run`으로 만들 목록만 볼 수 있다.

```bash
script/testnet-accounts.sh --dry-run   # 만들 계정 확인
script/testnet-accounts.sh             # 만들기
script/testnet-accounts.sh --list      # 주소 보기
```

## 테스트넷에서 forge 스크립트 실행

`testnet-forge.sh`로 실행한다. 스크립트는 역할마다 Keychain의 암호를 본인만 읽을 수 있는 임시 파일(`~/.nu54/run.*`)에
써서 `--password-file`로 넘기고, 끝나면(실패해도) 지운다. `~/.nu54/testnet-accounts.env`의 공개 주소도 읽어 들인다.
forge는 `/dev/fd` 경로와 FIFO를 암호 파일로 받지 않으므로 `--password-file <(security ...)` 형태는 쓰지 않는다.

```bash
script/testnet-forge.sh --simulate script/Deploy.s.sol deployer    # 전송 없이 모의 실행
script/testnet-forge.sh script/Deploy.s.sol deployer               # 배포
NU54_SETTLEMENT=0x... script/testnet-forge.sh script/TrustSettlement.s.sol token-owner  # 정산 컨트랙트를 시험 토큰 신뢰 송신자로 등록
NU54_SETTLEMENT=0x... script/testnet-forge.sh script/SoftwareSettle.s.sol \
  kiosk registry-admin token-owner operator device                 # 소프트웨어 서명 정산
```

첫 번째 역할이 기본 송신자다. `SoftwareSettle.s.sol`은 개인키를 읽지 않고 주소로 송신자와 서명자를 고른다
(`vm.broadcast(address)`, `vm.sign(address, digest)`). 가맹점이 이미 등록되어 있고 기기 잔액이 남아 있으면 등록과
입금을 건너뛰고, 정산한 서명을 다시 제출하면 `OrderAlreadyPaid`로 거절되는지 로컬에서 확인한다.

로컬 sandbox(anvil)에서는 같은 스크립트를 anvil 시험 키로 돌린다. `NU54_ADDR_*`를 anvil 계정 주소로 두고
`--private-keys`를 역할 수만큼 넘긴다.

keystore 파일과 Keychain 항목을 잃으면 그 계정의 WKRC와 역할을 되찾을 수 없다. 백업이 필요하면 keystore 폴더를 암호화한 저장 매체에 따로 둔다.

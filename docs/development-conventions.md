# 개발 규칙: 커밋, 브랜치, 검증

이 저장소는 펌웨어, 모바일 앱, 서비스, 스마트 컨트랙트를 함께 관리하는
폴리글랏 monorepo다. 구현을 시작하기 전에는
[저장소 checkpoint](../REPOSITORY-CHECKPOINT.md)와
[제품 인덱스](../products/README.md)를 먼저 확인한다.

## 변경 단위

- 한 커밋에는 리뷰와 되돌리기가 가능한 하나의 논리 변경만 담는다.
- 제품 변경은 해당 `products/pNN-*` 경계 안에서 시작한다.
- 여러 제품이 공유하는 계약이나 생성 코드는 `packages/`에 둔다.
- 설계 authority는 `docs/content/`에 유지한다. 과거 기록인
  `docs/design-history/`는 수정하지 않는다.
- mnemonic, private key, MPC share, OAuth secret, 실제 사용자 데이터는
  커밋하지 않는다. 공개 manifest에는 `secretRef`만 기록한다.

## 커밋 메시지

[Conventional Commits](https://www.conventionalcommits.org/) 형식을 사용한다.

```text
<type>(<scope>): <명령형 요약>
```

허용하는 기본 type은 다음과 같다.

- `feat`: 사용자가 확인할 수 있는 기능
- `fix`: 결함 수정
- `docs`: 문서만 변경
- `refactor`: 동작을 바꾸지 않는 구조 변경
- `test`: 시험 추가 또는 수정
- `build`: 빌드와 의존성 변경
- `ci`: 자동화 변경
- `chore`: 제품 동작과 무관한 유지보수

scope에는 변경한 영역을 기능 이름으로 적는다. 제품 번호(`p01` 등)는 찾아보지 않으면
무엇인지 알 수 없으므로 쓰지 않는다. 영역 이름은 [CONTRIBUTING.md](../CONTRIBUTING.md#commit-messages)의
scope 표(`firmware`, `kiosk`, `backoffice`, `contracts`, `indexer`, `conformance`,
`protocol`, `sandbox`, `docs`)를 따른다. 제목은 72자 이내의 명령형 문장으로 쓰고
마침표를 붙이지 않는다.

예시:

```text
feat(firmware): add authenticated BLE enrollment
fix(indexer): resume indexing from the canonical cursor
docs(protocol): clarify the envelope length field
```

모든 커밋은 [Developer Certificate of Origin](https://developercertificate.org/)에
따라 sign-off를 포함한다.

```bash
git commit -s -m "docs(repo): establish the product monorepo"
```

## 브랜치와 리뷰

- 브랜치는 `feat/firmware-ble-enrollment`, `fix/indexer-reorg-recovery`처럼 목적과
  변경 영역을 기능 이름으로 드러낸다.
- pull request에는 문제, 변경 결과, 검증 방법, 영향을 받는 제품과 계약을
  적는다.
- 공통 계약 변경은 소비 제품과 호환성 증거를 함께 갱신한다.
- 생성 파일은 원본과 생성 명령을 함께 커밋한다.

## 로컬 검증

```bash
python3 scripts/check_markdown_links.py
python3 docs/content/planning/validate_design_freeze.py
python3 docs/content/specifications/validate_specs.py
```

제품 코드가 추가되면 각 제품 README에 빌드, 시험, 실제 기기 검증 명령을
추가한다.

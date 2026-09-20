# Shared packages

여러 제품이 함께 소비하는 코드만 이 경계에 둔다. 현재는 설계 단계이므로
공유 package를 생성하지 않았다.

구현 단계의 예상 영역은 다음과 같다.

- API, BLE, event schema와 생성 타입
- React Native와 웹에서 공유하는 클라이언트 라이브러리
- StableNet ABI와 배포 manifest reader
- 합성 fixture와 contract-test 도구
- 공통 관측, 오류, idempotency 유틸리티

한 제품에서만 사용하는 코드는 해당 `products/pNN-*` 폴더에 둔다. 공유
package를 추가할 때는 소비 제품, 버전 호환성, 생성 원본과 검증 명령을 그
package의 README에 기록한다.

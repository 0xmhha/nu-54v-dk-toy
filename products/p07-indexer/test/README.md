# P07 통합 시험

단위 시험은 Go 관례대로 각 패키지 옆(`internal/*/..._test.go`)에 둔다. 이 폴더에는
로컬 sandbox(`make sandbox-up`: anvil 8283, PostgreSQL)에 붙어 도는 통합 시험을 둔다.
첫 통합 시험은 WBS2-P07-01(가맹점 등록과 attestation 발급)과 함께 추가한다.

v1 대여 세대 대사는 `python3 -B products/p07-indexer/test/generation-v1.py --pg-bin <PostgreSQL bin> --solc <고정 solc>`로 재현한다. 필요한 경우 `--anvil`과 `--forge`에 실행 파일을 지정한다. 격리된 P07 Reader와 실제 v1 계약, PostgreSQL, mTLS 경계를 사용한다. 쓰기 권한과 P05 연결은 추가하지 않는다. 실제 P01 EIP-712 동결 형식과 별도 합성 보드 인증키로 접수한다. 시험 전용 등록과 챌린지를 사용하며 실기 인증은 별도다.

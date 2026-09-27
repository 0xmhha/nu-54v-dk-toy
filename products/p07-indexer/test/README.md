# P07 통합 시험

단위 시험은 Go 관례대로 각 패키지 옆(`internal/*/..._test.go`)에 둔다. 이 폴더에는
로컬 sandbox(`make sandbox-up`: anvil 8283, PostgreSQL)에 붙어 도는 통합 시험을 둔다.
첫 통합 시험은 WBS2-P07-01(가맹점 등록과 attestation 발급)과 함께 추가한다.

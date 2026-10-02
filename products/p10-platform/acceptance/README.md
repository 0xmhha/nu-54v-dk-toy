# 12주 수용 도구

12주 수용 기록지([week12-log.md](../../../docs/content/acceptance/week12-log.md))를 채우는 증거 수집 도구다(WBS2-P10-03). 원본 증거는 git에 올리지 않는 `evidence/w12/`에 두고, 기록지에는 줄인 주소·tx hash(앞 6자리…뒤 4자리)와 64자리 `sha256:` checksum만 적는다 [N16][N18]. 그래서 도구가 쓴 기록지는 register 검사기의 redaction 검사를 그대로 통과한다.

```bash
A=products/p10-platform/acceptance/w12.py
python3 $A env [--firmware <이미지>] [--apk <APK>] --write   # 1절 시험 환경(펌웨어·APK 해시, 컨트랙트, chainId, 도구 버전, 키오스크 가스 잔액)
python3 $A add W12-01 kiosk.log phone.log                   # 증거 파일 보관(복사와 sha256)
python3 $A tx W12-03 0x<tx hash>                             # receipt 보관, finalized 여부와 PaymentSettled 요약
python3 $A timings W12-04 runs.csv                           # 20회, 각 10초 이내(열: ms, 선택 outcome)
python3 $A record W12-03 통과 [--note "..."]                 # 해당 행의 결과·증거 칸을 채운다
python3 $A check                                             # 증거 파일이 바뀌지 않았는지, 모든 행에 결과가 있는지
```

행 이름은 `W12-01`~`W12-12`와 거절 코드(`ATTESTATION_EXPIRED` 등)다. 결과는 통과, 실패, 조건부 통과, 미실행 중 하나이며, 증거 파일이 없는 행에는 통과를 쓸 수 없다. `manifest.json`이 파일별 checksum을 들고 있어 `check`가 사후 수정을 찾아낸다.

시험: `python3 acceptance/test/test_w12.py`(`make test`에 포함). 기록지 사본으로 실행하므로 실제 기록지와 증거 폴더는 건드리지 않는다.

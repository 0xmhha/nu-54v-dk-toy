# 로컬 sandbox

클라우드에 올리기 전에 로컬 docker로 제품을 검증하는 환경이다 [N25]. 지금은 체인과
DB 두 서비스만 있고, P07 indexer와 P05 운영 API는 구현되면 같은 compose 파일에 붙인다.

| 서비스 | 이미지 | 포트 | 역할 |
|---|---|---|---|
| `chain` | Foundry anvil | 8545 (`NU54_RPC_PORT`) | StableNet 8283 대신 쓰는 로컬 EVM 체인. chainId 8283, 1초 블록. `--slots-in-an-epoch 1`로 finalized가 최신 블록에서 약 2블록 뒤를 따른다(기본값은 64블록 뒤) |
| `db` | PostgreSQL 16 | 55432 (`NU54_DB_PORT`) | P07 영수증 저장, P05 백오피스 저장소 |

```bash
docker compose -f sandbox/compose.yaml up -d
cast chain-id --rpc-url http://127.0.0.1:8545   # 8283
docker compose -f sandbox/compose.yaml down
```

anvil은 StableNet의 합의와 finality 동작과 다르다. 그래서 게이트 증거(W6, W7, W9)는
StableNet 8283 테스트넷에서 만든다. `POSTGRES_PASSWORD`는 로컬 전용 값이고, 클라우드
환경에서는 secretRef로 주입한다.

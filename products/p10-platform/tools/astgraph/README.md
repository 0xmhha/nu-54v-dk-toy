# astgraph

Go 모듈을 타입 정보까지 포함해 AST로 읽고, 패키지·선언·정적 호출의 그래프를 JSON으로 쓴다. 외부 코드를 가져올지 판단할 때 "이 기능을 가져오면 무엇이 함께 딸려 오는가"를 숫자로 보려고 만들었다([indexer-go 재사용 분석](../../../../docs/content/products/p07/indexer-go-reuse.md)).

- 노드: package, func, method, type(인터페이스 표시). 각 노드에 줄 수, 파일, 첫 줄 doc.
- 간선: `imports`(모듈 안 패키지), `imports-module`(외부 모듈), `calls`(정적으로 결정되는 함수·메서드 호출), `uses`(타입 참조), `implements`, `external`(외부 패키지 호출), `declares`.
- 시험 파일(`_test.go`)은 뺀다. 대상 저장소는 `GOWORK=off GOFLAGS=-mod=readonly`로 읽으므로 대상의 go.mod를 바꾸지 않는다.

```bash
cd products/p10-platform/tools/astgraph
GOWORK=off go build -o /tmp/astgraph .
/tmp/astgraph <대상 모듈 디렉터리> <모듈 경로> > graph.json
python3 packages.py graph.json      # 패키지별 줄 수, fan-in/out, import 폐포와 그 안의 외부 모듈
python3 closures.py graph.json      # 기능(시작 선언)별 호출·타입 폐포: 함께 딸려 오는 선언 수와 줄 수
```

한계: 인터페이스를 거친 호출은 정적으로 대상을 알 수 없어 `calls`에 없다(타입 참조 `uses`로만 이어진다). 그래서 폐포는 실제보다 작게 나올 수 있다. 실행해 본 결과가 아니라 정적 분석이다.

import json,collections,sys,re
g=json.load(open(sys.argv[1])); M=g["module"]+"/"
nodes={n["id"]:n for n in g["nodes"]}
out=collections.defaultdict(set); ext=collections.defaultdict(set)
for e in g["edges"]:
    if e["kind"] in("calls","uses"): out[e["from"]].add(e["to"])
    if e["kind"]=="external": ext[e["from"]].add(e["to"][4:])
# a type pulls in its methods (they come along when the type is copied)
methods=collections.defaultdict(set)
for n in g["nodes"]:
    if n["kind"]=="method":
        t=n["id"].rsplit(".",1)[0]; methods[t].add(n["id"])
def closure(starts, with_methods=False):
    seen=set(); st=list(starts)
    while st:
        x=st.pop()
        if x in seen or x not in nodes: continue
        seen.add(x); st+=out.get(x,())
        if with_methods: st+=methods.get(x,())
    return seen
def report(label, starts, with_methods=False):
    c=closure(starts,with_methods)
    pk=collections.Counter(nodes[x]["pkg"].replace(M,"") for x in c)
    loc=sum(nodes[x]["loc"] for x in c)
    e=set().union(*(ext[x] for x in c)) if c else set()
    e={x for x in e if not x.startswith(("fmt","context","errors","strings","sync","time","sort","math","encoding","bytes","strconv","net/http","io","os","crypto","log","regexp","unicode","reflect","runtime","path","bufio","hash","container","slices","maps","net","text","html","os/","math/"))}
    print(f"\n## {label}: {len(c)} decls, {loc} LOC")
    print("   packages:", dict(pk.most_common(8)))
    print("   external:", sorted({re.sub(r'^github.com/ethereum/go-ethereum.*','geth',x) for x in e})[:12])
starts=lambda pat:[n["id"] for n in g["nodes"] if re.search(pat,n["id"])]
cases=[
 ("RPC client (client.Client and methods)", [M+"pkg/client.Client"], True),
 ("Fetcher: gap detection", starts(r"pkg/fetch\.Fetcher\.(DetectGaps|FillGaps|RunWithGapRecovery|ProcessGaps|fillGap|detectGap)"), False),
 ("Fetcher: whole type", [M+"pkg/fetch.Fetcher"], True),
 ("ABI decoder (abi.Decoder)", [M+"pkg/abi.Decoder"], True),
 ("Dynamic event parser (events.DynamicEventParser)", [M+"pkg/events.DynamicEventParser"], True),
 ("Contract registration service", [M+"pkg/events.ContractRegistrationService"], True),
 ("Log storage write+filter (storage.PebbleStorage log funcs)", starts(r"pkg/storage\.PebbleStorage\.(GetLogs|SaveLogs|indexLog|filterLogsByTopics|filterLogs)"), False),
 ("Local event bus", [M+"pkg/eventbus.LocalEventBus"], True),
 ("Webhook notifications", [M+"pkg/notifications.WebhookHandler", M+"pkg/notifications.VerifyWebhookSignature"], True),
 ("HTTP middleware (rate limit, recovery, logger, API key)", starts(r"pkg/api/middleware\.(RateLimit|Recovery|Logger|APIKeyAuth|NewRateLimiter|RateLimiter)$"), True),
 ("WebSocket hub", [M+"pkg/api/websocket.Hub", M+"pkg/api/websocket.Server"], True),
 ("Node type detector", [M+"pkg/adapters/detector.Detector"], True),
]
for c in cases:
    if not c[1]: print("\n## (no match)", c[0]); continue
    report(*c)

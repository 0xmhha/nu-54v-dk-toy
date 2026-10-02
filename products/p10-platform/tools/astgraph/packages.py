import json,collections,sys
g=json.load(open(sys.argv[1])); M=g["module"]
nodes={n["id"]:n for n in g["nodes"]}
imp=collections.defaultdict(set); mods=collections.defaultdict(set)
for e in g["edges"]:
    if e["kind"]=="imports": imp[e["from"]].add(e["to"])
    if e["kind"]=="imports-module": mods[e["from"]].add(e["to"][4:])
pk=[n for n in g["nodes"] if n["kind"]=="package"]
def closure(p):
    seen=set(); st=[p]
    while st:
        x=st.pop()
        if x in seen: continue
        seen.add(x); st+=imp.get(x,())
    return seen
fanin=collections.Counter(t for f,ts in imp.items() for t in ts)
rows=[]
for p in pk:
    c=closure(p["id"]); loc=sum(nodes[x]["loc"] for x in c if x in nodes)
    m=set().union(*(mods[x] for x in c))
    rows.append((p["name"],p["loc"],len(imp[p["id"]]),fanin[p["id"]],len(c)-1,loc,sorted(m)))
rows.sort(key=lambda r:r[5])
print(f"{'package':28}{'LOC':>6}{'out':>4}{'in':>4}{'clos':>5}{'closLOC':>8}  external modules in closure")
for r in rows:
    short=[x.split('/')[-1] if 'ethereum' not in x else 'geth' for x in r[6] if x!='github.com/ethereum/go-ethereum'] + (['geth'] if 'github.com/ethereum/go-ethereum' in r[6] else [])
    print(f"{r[0]:28}{r[1]:>6}{r[2]:>4}{r[3]:>4}{r[4]:>5}{r[5]:>8}  {','.join(sorted(set(short)))}")

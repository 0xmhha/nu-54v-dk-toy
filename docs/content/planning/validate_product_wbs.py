#!/usr/bin/env python3
from __future__ import annotations
import argparse, csv, re
from collections import Counter
from pathlib import Path

HERE=Path(__file__).resolve().parent
ROOT=HERE.parent.parent
CSV_PATH=HERE/'product-worklist-and-12week-wbs.csv'
HANDOFF=HERE/'preimplementation-task-handoffs.md'
REPORT=HERE/'product-worklist-and-12week-wbs-validation.txt'
REQUIRED={'wbs_id','product_id','product','work_package','detail','task_refs','priority','stream','start_week','end_week','predecessors','milestone','completion_evidence','status','assignee','actual_start','actual_end','evidence_links','blocker'}
STATUSES={'planned','in_progress','blocked','ready_for_review','done'}
PRIORITIES={'C0','C1','C2','C3'}
MILESTONES={f'M{i}' for i in range(8)}

def fail(message: str) -> None:
    raise SystemExit('FAIL: '+message)

def main() -> None:
    ap=argparse.ArgumentParser()
    ap.add_argument('--write-report',action='store_true')
    args=ap.parse_args()
    with CSV_PATH.open(encoding='utf-8-sig',newline='') as f:
        rows=list(csv.DictReader(f))
    if not rows: fail('empty CSV')
    missing_columns=REQUIRED-set(rows[0])
    if missing_columns: fail('missing columns '+','.join(sorted(missing_columns)))
    ids=[r['wbs_id'] for r in rows]
    if len(ids)!=len(set(ids)): fail('duplicate WBS id')
    idset=set(ids)
    authoritative=[x for x,_ in re.findall(r'^## ([A-Z0-9-]+) (.+)$',HANDOFF.read_text(),re.M)]
    assigned=[x for r in rows for x in r['task_refs'].split()]
    counts=Counter(assigned)
    missing=sorted(set(authoritative)-set(assigned))
    extra=sorted(set(assigned)-set(authoritative))
    duplicated=sorted(x for x,n in counts.items() if n!=1)
    if len(authoritative)!=104: fail(f'authoritative task count {len(authoritative)}')
    if missing or extra or duplicated: fail(f'task coverage missing={missing} extra={extra} duplicated={duplicated}')
    graph={i:[] for i in ids}
    end_week={r['wbs_id']:int(r['end_week']) for r in rows}
    for r in rows:
        try: start,end=int(r['start_week']),int(r['end_week'])
        except ValueError: fail(f"invalid week {r['wbs_id']}")
        if not (1<=start<=end<=12): fail(f"week range {r['wbs_id']}={start}..{end}")
        if r['priority'] not in PRIORITIES: fail(f"priority {r['wbs_id']}={r['priority']}")
        if r['status'] not in STATUSES: fail(f"status {r['wbs_id']}={r['status']}")
        if r['milestone'] not in MILESTONES: fail(f"milestone {r['wbs_id']}={r['milestone']}")
        for dep in r['predecessors'].split():
            if dep=='-': continue
            if dep not in idset: fail(f"unknown predecessor {r['wbs_id']}->{dep}")
            if end_week[dep] > end: fail(f"predecessor completes later {r['wbs_id']}({end}) <- {dep}({end_week[dep]})")
            graph[r['wbs_id']].append(dep)
    visiting=set(); visited=set()
    def visit(node: str, path: list[str]) -> None:
        if node in visiting: fail('dependency cycle '+' -> '.join(path+[node]))
        if node in visited: return
        visiting.add(node)
        for dep in graph[node]: visit(dep,path+[node])
        visiting.remove(node); visited.add(node)
    for node in ids: visit(node,[])
    products=sorted({r['product_id'] for r in rows})
    if products != [f'P{i:02d}' for i in range(1,11)]: fail(f'product ids {products}')
    summary=(
        'status=PASS\n'
        f'products={len(products)}\n'
        f'wbs_packages={len(rows)}\n'
        f'authoritative_tasks={len(authoritative)}\n'
        f'assigned_tasks={len(assigned)}\n'
        'missing=0\nextra=0\nduplicated=0\ncycles=0\nlate_predecessors=0\nweeks=1..12\n'
    )
    if args.write_report: REPORT.write_text(summary)
    print(summary,end='')
if __name__=='__main__': main()

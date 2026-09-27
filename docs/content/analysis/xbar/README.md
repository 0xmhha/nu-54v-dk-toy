# X-bar document graph

The current design corpus (137 documents) read as X-bar phrases: each document's
head proposition, its specifier (authority, product scope, date), the
complements it cannot stand without, and removable adjuncts. Method and record
schema: [METHOD.md](METHOD.md).

| File | Content |
|---|---|
| `records/*.jsonl` | One X-bar record per document (7 batches) |
| `build_xbar_graph.py` | Builds everything below from the records |
| `graph.json` | 349 nodes (137 documents, 10 products, concepts, artifacts), 1,409 typed edges |
| `graph.html` | Interactive force-directed view |
| `findings.json`, `FINDINGS.md` | Raw machine findings and per-product Mermaid graphs |

```bash
python3 docs/content/analysis/xbar/build_xbar_graph.py
```

## Corpus at a glance

| Authority | Count | Head type | Count |
|---|---:|---|---:|
| current | 52 | design | 36 |
| draft | 37 | contract | 20 |
| superseded | 23 | plan | 18 |
| reference | 14 | index | 16 |
| historical | 9 | research | 16 |
| withdrawn | 2 | verification | 12 |
| | | communication | 11 |
| | | decision | 5 |
| | | requirement | 3 |

Only 3 documents have a requirement head, while 36 have a design head and 20 a
contract head: the corpus designs and contracts far more than it states what
each product must do. This is the gap the per-product SRS work fills.

## Reviewed findings

`FINDINGS.md` lists 23 raw claim conflicts; most are formatting differences
(`8283` vs `8283(0x205b)`). The table keeps only the ones that change a
decision. Severity follows the design-freeze rule that `DF-20260920-01`
overrides earlier text.

| # | Finding | Documents | Severity |
|---|---|---|---|
| F1 | Two scope baselines coexist: the freeze fixes 15 requirements with full app integration, while the week 3 memo narrows to one-button testnet payment approval and the payment hardware wallet research recommends a merchant-authenticated signing device with an external secure element | `planning/design-freeze-checkpoint.md`, `twelve-week-completion-scope-v3.md`, `week-03-maker-team-meeting-memo.md`, `research/payment-hw-wallet-direction/README.md` | Blocking |
| F2 | No current document requires an external secure element, although the research concludes nRF54L15 alone cannot protect a seed (PSA Certified Level 1 only) | `research/payment-hw-wallet-direction/README.md` vs `planning/design-freeze-checkpoint.md` | Blocking |
| F3 | First real payment gate differs: week 4 (M2) vs weeks 5-6 vs week 8 | `planning/product-wbs-overview.md`, `payment-integration-workplan.md`, `medium-week-01-payment-wallet.md` | High |
| F4 | Current documents still say the firmware SDK and board target are unselected; the freeze fixed NCS v3.4.0 / Zephyr 4.4 | `planning/implementation-sequence.md`, `specifications/implementation-interfaces.md`, `specifications/device-coexistence-design.md` | High |
| F5 | Return completion and re-rental eligibility are one transaction in the baseline but two gated steps in the unmerged return candidates | `specifications/storage-operations.md` TX-07 vs `specifications/return-recovery-design.md`, `specifications/return-route-contracts.md` | High |
| F6 | Ten documents still block on RR-DEC-01 (late assets on a new travel wallet); the freeze selected an encrypted recovery package | `specifications/lifecycle-*.md`, `specifications/return-*.md`, `specifications/wallet-*.md` | Medium |
| F7 | Stale counts: logical security storage 15 vs 28, API 107 vs 110, decisions 19 vs 20, "task" meaning 57 / 104 / 320 depending on the document | `specifications/data-model.md`, `specifications/extended-contracts.md`, `planning/product-wbs-overview.md` | Medium |
| F8 | 11 live documents require superseded or historical documents (for example `planning/functional-execution-spec.md` requires `planning/decisions.md`) | see `FINDINGS.md` | Medium |
| F9 | Approval candidates replaced by `specifications/approval-baseline.md` carry no superseded banner in their own headers | `specifications/approval-*.md`, `specifications/wallet-api-contracts.md` | Low |
| F10 | Two parallel 104-card sets (`planning/execution-readiness.md`, `planning/preimplementation-task-handoffs.md`); only the latter is in the reading order | `planning/` | Low |

## Questions this graph hands to the requirement interview

1. Which baseline governs product definition: the freeze (15 requirements,
   10 products) or the narrowed payment hardware wallet direction (F1)?
2. Is an external secure element in scope, and which one (F2)?
3. What is the first end-to-end payment gate and its week (F3)?
4. Which products survive the baseline decision, and which become
   non-goals (P08 market services and P09 travel AI carry the fewest live
   requirements)?
5. Should return completion and re-rental eligibility be separate states (F5)?

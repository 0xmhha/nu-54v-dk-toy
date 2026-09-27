# X-bar document classification method

Each current document is treated as one phrase (XP) and decomposed with
X-bar theory so that the design corpus can be read as a dependency graph.

```
XP (document)
├── Specifier   authority, product scope, as-of date   -> scopes / supersedes
└── X'
    ├── X (Head)      the single proposition the document settles
    ├── Complement    inputs the head cannot stand without  -> requires
    └── Adjunct       removable support: rationale, example  -> elaborates
                      history, alternative, evidence
```

## Scope

- In: repository root docs, `products/*/README.md`, `packages/README.md`,
  and every `docs/**/*.md` except `docs/design-history/` and
  `docs/content/analysis/document-logic/before-*` (frozen snapshots).
- Out: JSON/SQL companions are not phrases; they appear only as
  complement or adjunct targets.

## Record schema (one JSON object per line)

```json
{
  "path": "docs/content/specifications/ble-catalog.md",
  "title": "document H1",
  "head": {
    "type": "requirement | decision | contract | design | plan | verification | research | communication | index",
    "proposition": "One Korean sentence: what this document settles or asserts.",
    "key_terms": ["BLE", "session"]
  },
  "specifier": {
    "authority": "current | superseded | draft | historical | withdrawn | reference",
    "superseded_by": ["repo-relative path"],
    "products": ["P01", "P04"],
    "as_of": "2026-09-20",
    "scope_note": "short qualifier, e.g. 'testnet PoC only'"
  },
  "complements": [
    {"target": "repo-relative path or concept:<name>", "role": "why the head needs it"}
  ],
  "adjuncts": [
    {"target": "repo-relative path or null", "kind": "rationale | example | history | alternative | evidence", "note": "..."}
  ],
  "claims": [
    {"subject": "canonical subject, e.g. 'ble.session.expiry'", "value": "stated value"}
  ],
  "open_questions": ["items the document itself leaves undecided"],
  "evidence": "path:line of the sentence that states the head"
}
```

### Rules

1. **Head** is exactly one proposition. If a document settles several, pick
   the one the rest of the document serves; list others as claims.
2. **Complement** means removal breaks the head. A link that only supports or
   illustrates is an **adjunct**.
3. **Specifier.authority**: `current` only when the document or a current
   index says it is the baseline; documents replaced by a later one are
   `superseded` with `superseded_by`; withdrawn ideas are `withdrawn`;
   research and external facts are `reference`.
4. **products** uses P01..P10 as defined in `products/README.md`; use `[]`
   for cross-cutting documents and `["ALL"]` for whole-system documents.
5. **claims.subject** uses lower-case dotted names
   (`<domain>.<object>.<attribute>`), e.g. `chain.id`, `ble.session.expiry`,
   `wallet.mpc.threshold`, `scope.requirement.count`, so values can be
   compared across documents. At most 8 claims per document, prefer values
   that other documents also state.
6. Paths must exist in the repository. Unknown targets use `concept:<name>`.
7. Never invent content; if a field is not stated, leave it empty.

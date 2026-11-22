# Article 08 illustrations

Original diagrams created on 2026-11-20 from the current firmware, renter app, kiosk code, and existing tests. Run `python3 build-diagrams.py` to regenerate. The script reuses the series drawing helpers in `../06/build-diagrams.py` and outputs six 1600 × 1100 PNG files.

| Korean | English | Meaning |
|---|---|---|
| 01-display-and-approval.png | 01-display-and-approval-en.png | Validated request → phone details → physical approval → finalized settlement outcome. The diagram shows the final-outcome path; early returns still have phone notification gaps. |
| 02-amount-precision.png | 02-amount-precision-en.png | Existing test amount 5,009,999 at six decimals: exact 5.009999, display 5.00. Digit cells show character positions, not quantities. The requested amount is unchanged. |
| 03-timeout-boundaries.png | 03-timeout-boundaries-en.png | Proportional 10-second kiosk wait and 120-second phone display timeout. Each has a different starting event; the phone timer does not cancel payment. |

These are code diagrams, not app screenshots or completed hardware demonstration evidence. No actual keys, full addresses, transaction hashes, account names, private paths, or project repository links appear in the images.

Captions and source statements are included immediately below each image in both manuscripts. Source details and testing boundaries are in `../../08-source-notes.md`, which is internal and excluded from Medium.

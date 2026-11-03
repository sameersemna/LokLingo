# Long-Document Validation Summary

Synthetic sparse pages (page number + one line), English, rendered at 150 DPI. This is a
stress test for page-count handling and degeneration, not a quality benchmark.

## Results

| Run | Pages | Page count | Empty pages | Latency | Notes |
| --- | ---: | ---: | ---: | ---: | --- |
| 10-page baseline | 10 | 10/10 | 0 | ~25.8 s | Required adaptive window splitting |
| 100-page | 100 | 100/100 | 0 | ~300 s | All page numbers present |
| 200-page first pass | 200 | 200/200 | 0 | ~8 min | Final page repeated; page 1 content mismatch |
| 200-page after retry tuning | 200 | 200/200 | 0 | ~580 s | Correct first/last page; retries on 26, 58, 200 |

## Fixes driven by the runs

- Adaptive window splitting when the model returns fewer `<PAGE>` markers than inputs.
- Repeated-line detection and single-page retry.
- Default window size reduced from 10 to 5 pages.
- Offline model-cache loading for compose restarts.

## Caveats

- Sparse synthetic pages are easier than real scans/tables.
- The 200-page first pass shows that page-count success alone is not a quality signal.
- Real-document 100/200-page validation and human review remain required before enabling
  automatic routing.

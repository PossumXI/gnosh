
### Lines, against crush

| set | files | with 10+ significant lines | share p50 | p95 | max | files with a counted run | longest | files with any run of 4+ | longest | raw share p95 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 631 | 598 | 0.0% | 1.2% | 10.0% | 0 | 0 | 4 | 5 | 3.9% | 10.0% |
| new: prompt texts | 51 | 7 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| new: test data | 104 | 48 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| new: documents | 45 | 44 | 0.0% | 0.0% | 0.2% | 0 | 0 | 0 | 0 | 0.0% | 0.2% |
| new: go.mod, go.sum and the like | 7 | 5 | 0.0% | 41.2% | 41.2% | 1 | 30 | 1 | 30 | 41.2% | 41.2% |
| carried | 123 | 120 | 0.0% | 1.4% | 7.5% | 0 | 0 | 0 | 0 | 3.9% | 8.2% |
| carried with larger changes | 17 | 17 | 0.0% | 0.8% | 3.1% | 0 | 0 | 0 | 0 | 1.8% | 3.1% |
| third-party | 163 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 0.0% | 2.6% |
| carried with larger changes: doc | 2 | 2 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| carried: doc | 4 | 4 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| carried: other | 7 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 0% | 0% |
| carried: testdata | 84 | 52 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 3.3% | 5.6% |
| third-party: doc | 2 | 2 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| third-party: other | 4 | 2 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| third-party: testdata | 4 | 4 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:fzf | 91 | 73 | 0.0% | 1.0% | 4.7% | 0 | 0 | 2 | 5 | 6.7% | 11.5% |
| ctl:gh | 985 | 899 | 0.0% | 4.2% | 12.5% | 0 | 0 | 5 | 14 | 7.6% | 22.5% |
| ctl:go-net-http | 157 | 0 | 0% | 0% | 0% | 0 | 0 | 1 | 7 | 4.5% | 11.1% |
| ctl:go-os-exec | 29 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 9.3% | 11.3% |
| ctl:go-text-template | 17 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 1.3% | 1.5% |
| ctl:mvdan-sh | 77 | 65 | 0.0% | 2.7% | 9.7% | 0 | 0 | 1 | 4 | 9.1% | 18.2% |
| ctl:ollama | 1076 | 984 | 0.0% | 2.6% | 17.6% | 0 | 0 | 16 | 6 | 8.3% | 21.2% |
| cal:crush-modified | 299 | 294 | 85.9% | 100.0% | 100.0% | 292 | 1016 | 292 | 1016 | 100.0% | 100.0% |
| cal:crush-unchanged | 443 | 396 | 100.0% | 100.0% | 100.0% | 434 | 695 | 434 | 695 | 100.0% | 100.0% |
| cal:old-rewrite | 28 | 26 | 20.9% | 100.0% | 100.0% | 16 | 377 | 16 | 377 | 100.0% | 100.0% |
| cal:old-shortblock | 20 | 20 | 5.6% | 11.9% | 13.9% | 18 | 7 | 19 | 11 | 14.8% | 16.2% |

### Lines, against hist

| set | files | with 10+ significant lines | share p50 | p95 | max | files with a counted run | longest | files with any run of 4+ | longest | raw share p95 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 631 | 598 | 0.0% | 2.2% | 10.0% | 0 | 0 | 4 | 5 | 5.2% | 12.0% |
| new: prompt texts | 51 | 7 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| new: test data | 104 | 48 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| new: documents | 45 | 44 | 0.0% | 0.0% | 0.2% | 0 | 0 | 0 | 0 | 0.2% | 0.2% |
| new: go.mod, go.sum and the like | 7 | 5 | 0.0% | 56.7% | 56.7% | 1 | 30 | 1 | 30 | 56.7% | 56.7% |
| carried | 123 | 120 | 0.0% | 2.7% | 9.1% | 0 | 0 | 0 | 0 | 5.2% | 11.9% |
| carried with larger changes | 17 | 17 | 0.0% | 2.6% | 3.1% | 0 | 0 | 0 | 0 | 3.1% | 3.4% |
| third-party | 163 | 0 | 0% | 0% | 0% | 0 | 0 | 14 | 2238 | 27.3% | 99.9% |
| carried with larger changes: doc | 2 | 2 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| carried: doc | 4 | 4 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.2% | 0.2% |
| carried: other | 7 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 0% | 0% |
| carried: testdata | 84 | 52 | 0.0% | 0.0% | 5.0% | 0 | 0 | 0 | 0 | 5.0% | 11.1% |
| third-party: doc | 2 | 2 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.2% | 0.2% |
| third-party: other | 4 | 2 | 0.0% | 100.0% | 100.0% | 1 | 23 | 1 | 23 | 100.0% | 100.0% |
| third-party: testdata | 4 | 4 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:fzf | 91 | 73 | 0.0% | 3.0% | 7.0% | 0 | 0 | 2 | 5 | 6.9% | 11.5% |
| ctl:gh | 985 | 899 | 0.0% | 4.8% | 16.4% | 3 | 6 | 15 | 15 | 8.8% | 25.4% |
| ctl:go-net-http | 157 | 0 | 0% | 0% | 0% | 0 | 0 | 31 | 631 | 94.9% | 100.0% |
| ctl:go-os-exec | 29 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 27.3% | 27.3% |
| ctl:go-text-template | 17 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 10.3% | 18.8% |
| ctl:mvdan-sh | 77 | 65 | 14.3% | 81.1% | 97.2% | 31 | 86 | 33 | 86 | 81.1% | 97.3% |
| ctl:ollama | 1076 | 984 | 0.0% | 4.3% | 17.6% | 2 | 25 | 25 | 25 | 10.4% | 50.0% |
| cal:crush-modified | 299 | 294 | 86.2% | 100.0% | 100.0% | 292 | 1016 | 292 | 1016 | 100.0% | 100.0% |
| cal:crush-unchanged | 443 | 396 | 100.0% | 100.0% | 100.0% | 434 | 695 | 434 | 695 | 100.0% | 100.0% |
| cal:old-rewrite | 28 | 26 | 22.4% | 100.0% | 100.0% | 16 | 377 | 16 | 377 | 100.0% | 100.0% |
| cal:old-shortblock | 20 | 20 | 5.6% | 13.0% | 13.9% | 19 | 7 | 20 | 11 | 14.9% | 16.2% |

### Lines, against libs

| set | files | with 10+ significant lines | share p50 | p95 | max | files with a counted run | longest | files with any run of 4+ | longest | raw share p95 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 631 | 598 | 0.0% | 0.9% | 7.4% | 0 | 0 | 0 | 0 | 2.8% | 11.8% |
| new: prompt texts | 51 | 7 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| new: test data | 104 | 48 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| new: documents | 45 | 44 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.2% |
| new: go.mod, go.sum and the like | 7 | 5 | 0.0% | 17.6% | 17.6% | 1 | 4 | 1 | 4 | 17.6% | 17.6% |
| carried | 123 | 120 | 0.0% | 0.7% | 2.9% | 0 | 0 | 0 | 0 | 3.0% | 5.5% |
| carried with larger changes | 17 | 17 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 1.1% |
| third-party | 163 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 16.7% | 30.0% |
| carried with larger changes: doc | 2 | 2 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| carried: doc | 4 | 4 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.2% | 0.2% |
| carried: other | 7 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 0% | 0% |
| carried: testdata | 84 | 52 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 3.3% | 6.7% |
| third-party: doc | 2 | 2 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.2% | 0.2% |
| third-party: other | 4 | 2 | 0.0% | 100.0% | 100.0% | 1 | 23 | 1 | 23 | 100.0% | 100.0% |
| third-party: testdata | 4 | 4 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:fzf | 91 | 73 | 0.0% | 0.2% | 3.1% | 0 | 0 | 0 | 0 | 4.9% | 7.1% |
| ctl:gh | 985 | 899 | 0.0% | 1.1% | 9.3% | 0 | 0 | 2 | 14 | 7.3% | 22.5% |
| ctl:go-net-http | 157 | 0 | 0% | 0% | 0% | 0 | 0 | 1 | 5 | 20.0% | 36.4% |
| ctl:go-os-exec | 29 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 23.1% | 27.3% |
| ctl:go-text-template | 17 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 9.4% | 12.5% |
| ctl:mvdan-sh | 77 | 65 | 0.0% | 0.0% | 1.9% | 0 | 0 | 0 | 0 | 7.7% | 11.8% |
| ctl:ollama | 1076 | 984 | 0.0% | 2.6% | 12.0% | 1 | 4 | 5 | 5 | 7.9% | 41.7% |
| cal:crush-modified | 299 | 294 | 0.0% | 6.0% | 25.0% | 4 | 45 | 7 | 45 | 7.8% | 31.2% |
| cal:crush-unchanged | 443 | 396 | 0.0% | 5.9% | 83.3% | 1 | 5 | 3 | 5 | 11.1% | 83.3% |
| cal:old-rewrite | 28 | 26 | 0.0% | 2.0% | 2.3% | 0 | 0 | 0 | 0 | 5.9% | 7.1% |
| cal:old-shortblock | 20 | 20 | 0.0% | 2.8% | 5.4% | 0 | 0 | 0 | 0 | 3.3% | 6.4% |

### Lines, against since

| set | files | with 10+ significant lines | share p50 | p95 | max | files with a counted run | longest | files with any run of 4+ | longest | raw share p95 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 631 | 598 | 0.0% | 0.0% | 0.8% | 0 | 0 | 1 | 4 | 1.4% | 6.7% |
| new: prompt texts | 51 | 7 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| new: test data | 104 | 48 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| new: documents | 45 | 44 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| new: go.mod, go.sum and the like | 7 | 5 | 0.0% | 26.5% | 26.5% | 1 | 8 | 1 | 8 | 26.5% | 26.5% |
| carried | 123 | 120 | 0.0% | 0.0% | 0.2% | 0 | 0 | 0 | 0 | 1.2% | 2.4% |
| carried with larger changes | 17 | 17 | 0.0% | 0.0% | 0.3% | 0 | 0 | 0 | 0 | 0.0% | 0.5% |
| third-party | 163 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 0.0% | 0.3% |
| carried with larger changes: doc | 2 | 2 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| carried: doc | 4 | 4 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| carried: other | 7 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 0% | 0% |
| carried: testdata | 84 | 52 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 1.4% | 1.7% |
| third-party: doc | 2 | 2 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| third-party: other | 4 | 2 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| third-party: testdata | 4 | 4 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:fzf | 91 | 73 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 3.3% | 9.5% |
| ctl:gh | 985 | 899 | 0.0% | 2.4% | 8.3% | 0 | 0 | 0 | 0 | 6.2% | 20.0% |
| ctl:go-net-http | 157 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 2.5% | 10.0% |
| ctl:go-os-exec | 29 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 2.8% | 3.8% |
| ctl:go-text-template | 17 | 0 | 0% | 0% | 0% | 0 | 0 | 0 | 0 | 0.3% | 0.5% |
| ctl:mvdan-sh | 77 | 65 | 0.0% | 0.0% | 0.0% | 0 | 0 | 0 | 0 | 4.5% | 7.7% |
| ctl:ollama | 1076 | 984 | 0.0% | 0.0% | 3.1% | 0 | 0 | 0 | 0 | 4.3% | 16.7% |
| cal:crush-modified | 299 | 294 | 0.0% | 50.5% | 97.8% | 29 | 338 | 30 | 338 | 50.5% | 97.8% |
| cal:crush-unchanged | 443 | 396 | 0.0% | 11.1% | 100.0% | 13 | 217 | 14 | 217 | 12.5% | 97.7% |
| cal:old-rewrite | 28 | 26 | 2.3% | 28.6% | 30.8% | 4 | 7 | 4 | 7 | 28.6% | 35.7% |
| cal:old-shortblock | 20 | 20 | 0.3% | 3.1% | 3.6% | 1 | 7 | 1 | 7 | 3.6% | 3.8% |

### Exact tokens (windows of 40), against crush

| set | files of 100+ tokens | with a varied shared window | longest varied run p95 | p99 | max | varied cover p99 | max | with any shared window | longest run p99 | max | cover p99 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 553 | 3 | 0 | 0 | 53 | 0.0% | 3.7% | 101 | 134 | 321 | 24.8% | 99.5% |
| carried | 102 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 9 | 109 | 115 | 11.2% | 12.1% |
| carried with larger changes | 6 | 1 | 43 | 43 | 43 | 9.5% | 9.5% | 2 | 102 | 102 | 12.6% | 12.6% |
| third-party | 133 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:fzf | 81 | 1 | 0 | 0 | 41 | 0.0% | 1.4% | 8 | 75 | 100 | 36.9% | 54.0% |
| ctl:gh | 916 | 17 | 0 | 43 | 56 | 1.1% | 28.0% | 40 | 64 | 135 | 5.6% | 28.0% |
| ctl:go-net-http | 144 | 5 | 0 | 51 | 117 | 1.8% | 8.1% | 26 | 129 | 357 | 19.5% | 21.5% |
| ctl:go-os-exec | 17 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:go-text-template | 16 | 1 | 0 | 40 | 40 | 0.6% | 0.6% | 3 | 189 | 189 | 9.5% | 9.5% |
| ctl:mvdan-sh | 67 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 8 | 91 | 203 | 18.1% | 20.5% |
| ctl:ollama | 1001 | 38 | 0 | 48 | 75 | 3.7% | 27.5% | 93 | 80 | 501 | 15.4% | 92.0% |
| cal:crush-modified | 291 | 288 | 3169 | 4261 | 6677 | 100.0% | 100.0% | 289 | 5615 | 8663 | 100.0% | 100.0% |
| cal:crush-unchanged | 391 | 391 | 2069 | 3479 | 5457 | 100.0% | 100.0% | 391 | 3700 | 5457 | 100.0% | 100.0% |
| cal:old-rewrite | 24 | 19 | 1774 | 2923 | 2923 | 93.6% | 93.6% | 19 | 2923 | 2923 | 93.6% | 93.6% |
| cal:old-shortblock | 20 | 15 | 64 | 85 | 85 | 29.5% | 29.5% | 16 | 89 | 89 | 29.5% | 29.5% |

### Exact tokens (windows of 40), against hist

| set | files of 100+ tokens | with a varied shared window | longest varied run p95 | p99 | max | varied cover p99 | max | with any shared window | longest run p99 | max | cover p99 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 553 | 6 | 0 | 0 | 53 | 0.0% | 3.7% | 109 | 239 | 1770 | 27.4% | 99.6% |
| carried | 102 | 1 | 0 | 0 | 44 | 0.0% | 0.5% | 10 | 109 | 119 | 11.2% | 12.3% |
| carried with larger changes | 6 | 1 | 43 | 43 | 43 | 9.5% | 9.5% | 2 | 102 | 102 | 12.6% | 12.6% |
| third-party | 133 | 9 | 51 | 369 | 1311 | 65.6% | 97.6% | 10 | 520 | 9309 | 65.6% | 100.0% |
| ctl:fzf | 81 | 1 | 0 | 0 | 41 | 0.0% | 1.4% | 8 | 100 | 139 | 36.9% | 55.6% |
| ctl:gh | 916 | 25 | 0 | 44 | 101 | 4.7% | 28.0% | 54 | 83 | 195 | 9.7% | 28.0% |
| ctl:go-net-http | 144 | 33 | 626 | 1718 | 1881 | 100.0% | 100.0% | 53 | 1718 | 1881 | 100.0% | 100.0% |
| ctl:go-os-exec | 17 | 1 | 0 | 43 | 43 | 2.7% | 2.7% | 1 | 43 | 43 | 2.7% | 2.7% |
| ctl:go-text-template | 16 | 1 | 0 | 49 | 49 | 0.8% | 0.8% | 3 | 189 | 189 | 9.5% | 9.5% |
| ctl:mvdan-sh | 67 | 30 | 539 | 650 | 660 | 93.9% | 100.0% | 35 | 660 | 898 | 93.9% | 100.0% |
| ctl:ollama | 1001 | 58 | 41 | 51 | 97 | 7.2% | 52.4% | 123 | 106 | 501 | 21.6% | 92.9% |
| cal:crush-modified | 291 | 288 | 3169 | 4261 | 6677 | 100.0% | 100.0% | 289 | 5615 | 8663 | 100.0% | 100.0% |
| cal:crush-unchanged | 391 | 391 | 2069 | 3479 | 5457 | 100.0% | 100.0% | 391 | 3700 | 5457 | 100.0% | 100.0% |
| cal:old-rewrite | 24 | 19 | 1774 | 2923 | 2923 | 93.6% | 93.6% | 20 | 2923 | 2923 | 93.6% | 93.6% |
| cal:old-shortblock | 20 | 15 | 64 | 85 | 85 | 31.3% | 31.3% | 16 | 97 | 97 | 42.0% | 42.0% |

### Exact tokens (windows of 40), against libs

| set | files of 100+ tokens | with a varied shared window | longest varied run p95 | p99 | max | varied cover p99 | max | with any shared window | longest run p99 | max | cover p99 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 553 | 8 | 0 | 43 | 55 | 1.4% | 7.2% | 96 | 117 | 434 | 22.3% | 99.6% |
| carried | 102 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 6 | 62 | 109 | 10.8% | 16.4% |
| carried with larger changes | 6 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 1 | 90 | 90 | 3.4% | 3.4% |
| third-party | 133 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:fzf | 81 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 7 | 77 | 181 | 24.2% | 55.6% |
| ctl:gh | 916 | 15 | 0 | 43 | 56 | 1.1% | 18.5% | 38 | 63 | 98 | 4.7% | 18.5% |
| ctl:go-net-http | 144 | 4 | 0 | 49 | 50 | 1.5% | 2.0% | 23 | 123 | 123 | 19.2% | 21.5% |
| ctl:go-os-exec | 17 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:go-text-template | 16 | 1 | 0 | 48 | 48 | 0.8% | 0.8% | 2 | 124 | 124 | 2.6% | 2.6% |
| ctl:mvdan-sh | 67 | 1 | 0 | 0 | 41 | 0.0% | 2.4% | 5 | 94 | 121 | 18.1% | 21.0% |
| ctl:ollama | 1001 | 37 | 0 | 49 | 62 | 3.8% | 59.4% | 87 | 78 | 343 | 14.8% | 91.8% |
| cal:crush-modified | 291 | 13 | 0 | 59 | 408 | 9.5% | 16.6% | 21 | 91 | 408 | 12.6% | 67.5% |
| cal:crush-unchanged | 391 | 20 | 0 | 51 | 65 | 14.0% | 31.1% | 23 | 60 | 72 | 19.8% | 41.0% |
| cal:old-rewrite | 24 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 1 | 45 | 45 | 1.4% | 1.4% |
| cal:old-shortblock | 20 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 2 | 91 | 91 | 31.1% | 31.1% |

### Exact tokens (windows of 40), against since

| set | files of 100+ tokens | with a varied shared window | longest varied run p95 | p99 | max | varied cover p99 | max | with any shared window | longest run p99 | max | cover p99 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 553 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| carried | 102 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| carried with larger changes | 6 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| third-party | 133 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:fzf | 81 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:gh | 916 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:go-net-http | 144 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:go-os-exec | 17 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:go-text-template | 16 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:mvdan-sh | 67 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:ollama | 1001 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 1 | 0 | 41 | 0.0% | 0.4% |
| cal:crush-modified | 291 | 34 | 323 | 1369 | 1670 | 91.9% | 97.7% | 34 | 1380 | 1670 | 91.9% | 97.7% |
| cal:crush-unchanged | 391 | 17 | 0 | 81 | 2274 | 77.5% | 96.8% | 17 | 81 | 2274 | 77.5% | 96.8% |
| cal:old-rewrite | 24 | 3 | 43 | 43 | 43 | 9.4% | 9.4% | 3 | 43 | 43 | 9.4% | 9.4% |
| cal:old-shortblock | 20 | 2 | 43 | 43 | 43 | 2.9% | 2.9% | 2 | 43 | 43 | 2.9% | 2.9% |

### Identifier-insensitive tokens (windows of 60), against crush

| set | files of 100+ tokens | with a varied shared window | longest varied run p95 | p99 | max | varied cover p99 | max | with any shared window | longest run p99 | max | cover p99 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 553 | 47 | 63 | 73 | 83 | 5.5% | 47.2% | 108 | 143 | 339 | 25.7% | 99.5% |
| carried | 102 | 7 | 63 | 69 | 74 | 3.0% | 3.5% | 10 | 111 | 118 | 10.8% | 20.8% |
| carried with larger changes | 6 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 2 | 102 | 102 | 7.1% | 7.1% |
| third-party | 133 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 2 | 60 | 110 | 0.6% | 1.6% |
| ctl:fzf | 81 | 1 | 0 | 0 | 61 | 0.0% | 2.6% | 7 | 106 | 107 | 42.4% | 59.3% |
| ctl:gh | 916 | 91 | 75 | 83 | 106 | 21.4% | 44.8% | 156 | 150 | 195 | 31.2% | 57.9% |
| ctl:go-net-http | 144 | 13 | 65 | 109 | 117 | 44.0% | 44.0% | 26 | 246 | 358 | 61.0% | 61.0% |
| ctl:go-os-exec | 17 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:go-text-template | 16 | 1 | 0 | 62 | 62 | 1.0% | 1.0% | 3 | 241 | 241 | 10.4% | 10.4% |
| ctl:mvdan-sh | 67 | 7 | 69 | 76 | 85 | 6.6% | 15.2% | 11 | 133 | 212 | 18.7% | 24.4% |
| ctl:ollama | 1001 | 82 | 67 | 80 | 223 | 11.7% | 69.4% | 136 | 122 | 502 | 40.4% | 92.9% |
| cal:crush-modified | 291 | 286 | 1957 | 2709 | 3884 | 100.0% | 100.0% | 288 | 6031 | 8664 | 100.0% | 100.0% |
| cal:crush-unchanged | 391 | 391 | 1369 | 2625 | 3700 | 100.0% | 100.0% | 391 | 3700 | 5457 | 100.0% | 100.0% |
| cal:old-rewrite | 24 | 15 | 1046 | 2923 | 2923 | 100.0% | 100.0% | 15 | 3962 | 3962 | 100.0% | 100.0% |
| cal:old-shortblock | 20 | 7 | 72 | 86 | 86 | 36.5% | 36.5% | 12 | 94 | 94 | 38.8% | 38.8% |

### Identifier-insensitive tokens (windows of 60), against hist

| set | files of 100+ tokens | with a varied shared window | longest varied run p95 | p99 | max | varied cover p99 | max | with any shared window | longest run p99 | max | cover p99 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 553 | 69 | 71 | 94 | 117 | 11.8% | 48.2% | 131 | 250 | 1772 | 35.0% | 99.8% |
| carried | 102 | 9 | 65 | 69 | 74 | 3.0% | 3.7% | 16 | 111 | 124 | 20.8% | 26.3% |
| carried with larger changes | 6 | 1 | 70 | 70 | 70 | 7.8% | 7.8% | 2 | 102 | 102 | 25.1% | 25.1% |
| third-party | 133 | 18 | 81 | 146 | 370 | 32.5% | 63.1% | 27 | 4178 | 9309 | 98.6% | 100.0% |
| ctl:fzf | 81 | 9 | 75 | 120 | 120 | 10.1% | 18.2% | 17 | 1284 | 1284 | 97.1% | 100.0% |
| ctl:gh | 916 | 132 | 80 | 88 | 279 | 29.1% | 50.2% | 198 | 195 | 1748 | 41.9% | 98.0% |
| ctl:go-net-http | 144 | 39 | 504 | 1423 | 1718 | 100.0% | 100.0% | 53 | 1718 | 1881 | 100.0% | 100.0% |
| ctl:go-os-exec | 17 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:go-text-template | 16 | 2 | 63 | 72 | 72 | 3.9% | 3.9% | 4 | 241 | 241 | 11.0% | 11.0% |
| ctl:mvdan-sh | 67 | 33 | 573 | 661 | 782 | 87.3% | 93.9% | 36 | 898 | 1284 | 96.4% | 100.0% |
| ctl:ollama | 1001 | 135 | 79 | 97 | 223 | 26.2% | 97.5% | 201 | 229 | 2256 | 58.2% | 97.5% |
| cal:crush-modified | 291 | 286 | 1957 | 2709 | 4408 | 100.0% | 100.0% | 288 | 6031 | 8664 | 100.0% | 100.0% |
| cal:crush-unchanged | 391 | 391 | 1369 | 2625 | 3700 | 100.0% | 100.0% | 391 | 3700 | 5457 | 100.0% | 100.0% |
| cal:old-rewrite | 24 | 16 | 1046 | 2923 | 2923 | 100.0% | 100.0% | 16 | 3962 | 3962 | 100.0% | 100.0% |
| cal:old-shortblock | 20 | 12 | 82 | 86 | 86 | 36.5% | 36.5% | 15 | 104 | 104 | 38.8% | 38.8% |

### Identifier-insensitive tokens (windows of 60), against libs

| set | files of 100+ tokens | with a varied shared window | longest varied run p95 | p99 | max | varied cover p99 | max | with any shared window | longest run p99 | max | cover p99 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 553 | 54 | 63 | 76 | 121 | 7.2% | 16.2% | 119 | 162 | 434 | 30.6% | 44.8% |
| carried | 102 | 3 | 0 | 63 | 63 | 2.6% | 3.1% | 7 | 110 | 122 | 8.5% | 20.6% |
| carried with larger changes | 6 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 1 | 90 | 90 | 2.3% | 2.3% |
| third-party | 133 | 1 | 0 | 0 | 68 | 0.0% | 4.4% | 12 | 85 | 582 | 68.8% | 97.8% |
| ctl:fzf | 81 | 2 | 0 | 74 | 75 | 0.7% | 1.2% | 12 | 182 | 194 | 42.4% | 59.7% |
| ctl:gh | 916 | 89 | 75 | 84 | 115 | 27.9% | 57.3% | 129 | 133 | 210 | 47.2% | 61.1% |
| ctl:go-net-http | 144 | 13 | 70 | 88 | 106 | 61.6% | 61.6% | 28 | 193 | 230 | 85.1% | 98.8% |
| ctl:go-os-exec | 17 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:go-text-template | 16 | 2 | 62 | 73 | 73 | 3.9% | 3.9% | 2 | 142 | 142 | 4.7% | 4.7% |
| ctl:mvdan-sh | 67 | 4 | 64 | 76 | 113 | 5.5% | 11.3% | 13 | 122 | 166 | 20.4% | 22.5% |
| ctl:ollama | 1001 | 112 | 75 | 89 | 125 | 21.6% | 97.5% | 163 | 127 | 465 | 46.4% | 97.5% |
| cal:crush-modified | 291 | 31 | 63 | 82 | 138 | 10.2% | 15.4% | 48 | 170 | 408 | 18.9% | 67.5% |
| cal:crush-unchanged | 391 | 32 | 64 | 80 | 177 | 30.9% | 73.9% | 43 | 133 | 296 | 45.1% | 75.6% |
| cal:old-rewrite | 24 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 1 | 60 | 60 | 1.5% | 1.5% |
| cal:old-shortblock | 20 | 5 | 73 | 82 | 82 | 14.0% | 14.0% | 7 | 99 | 99 | 31.1% | 31.1% |

### Identifier-insensitive tokens (windows of 60), against since

| set | files of 100+ tokens | with a varied shared window | longest varied run p95 | p99 | max | varied cover p99 | max | with any shared window | longest run p99 | max | cover p99 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| new code | 553 | 1 | 0 | 0 | 61 | 0.0% | 12.2% | 7 | 62 | 126 | 1.6% | 25.2% |
| carried | 102 | 1 | 0 | 0 | 66 | 0.0% | 0.6% | 2 | 66 | 71 | 0.6% | 1.1% |
| carried with larger changes | 6 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| third-party | 133 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:fzf | 81 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 1 | 0 | 81 | 0.0% | 0.2% |
| ctl:gh | 916 | 2 | 0 | 0 | 63 | 0.0% | 39.9% | 5 | 0 | 104 | 0.0% | 39.9% |
| ctl:go-net-http | 144 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 3 | 74 | 97 | 5.2% | 18.5% |
| ctl:go-os-exec | 17 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:go-text-template | 16 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |
| ctl:mvdan-sh | 67 | 1 | 0 | 0 | 69 | 0.0% | 1.7% | 2 | 69 | 133 | 1.7% | 6.7% |
| ctl:ollama | 1001 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 5 | 0 | 95 | 0.0% | 2.6% |
| cal:crush-modified | 291 | 34 | 332 | 1082 | 1387 | 86.6% | 97.8% | 39 | 1498 | 1677 | 91.2% | 97.8% |
| cal:crush-unchanged | 391 | 13 | 0 | 154 | 762 | 60.4% | 97.7% | 16 | 154 | 2275 | 60.4% | 97.7% |
| cal:old-rewrite | 24 | 2 | 61 | 61 | 61 | 8.4% | 8.4% | 2 | 61 | 61 | 8.4% | 8.4% |
| cal:old-shortblock | 20 | 0 | 0 | 0 | 0 | 0.0% | 0.0% | 0 | 0 | 0 | 0.0% | 0.0% |

### Words, against crush

| set | units of 20+ words | longest shared word run p50 | p95 | p99 | max | units with a run of 12+ words | sentences | of them also in the corpus |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| carried | 13 | 0 | 7 | 8 | 8 | 0 | 495 | 0 |
| carried (string literals) | 82 | 5 | 5 | 5 | 5 | 0 | 1773 | 39 |
| carried with larger changes | 10 | 0 | 8 | 8 | 8 | 0 | 296 | 1 |
| carried with larger changes (string literals) | 6 | 0 | 8 | 8 | 8 | 0 | 197 | 0 |
| carried with larger changes: doc | 2 | 0 | 8 | 8 | 8 | 0 | 325 | 0 |
| carried: doc | 4 | 5 | 11 | 11 | 11 | 0 | 1175 | 0 |
| carried: other | 4 | 0 | 0 | 0 | 0 | 0 | 24 | 0 |
| carried: testdata | 78 | 0 | 7 | 9 | 9 | 0 | 1583 | 0 |
| new code | 22 | 0 | 7 | 8 | 8 | 0 | 1016 | 0 |
| new code (string literals) | 423 | 0 | 6 | 9 | 13 | 3 | 16534 | 27 |
| new: documents | 45 | 5 | 8 | 11 | 11 | 0 | 11145 | 1 |
| new: go.mod, go.sum and the like | 6 | 8 | 22 | 22 | 22 | 1 | 19 | 0 |
| new: prompt texts | 45 | 0 | 5 | 6 | 6 | 0 | 334 | 0 |
| new: test data | 103 | 0 | 6 | 9 | 10 | 0 | 1374 | 0 |
| third-party (string literals) | 22 | 0 | 0 | 6 | 6 | 0 | 113 | 1 |
| third-party: doc | 2 | 11 | 11 | 11 | 11 | 0 | 403 | 0 |
| third-party: other | 3 | 6 | 27 | 27 | 27 | 1 | 16 | 0 |
| third-party: testdata | 4 | 0 | 0 | 0 | 0 | 0 | 260 | 0 |
| ctl:fzf | 10 | 0 | 8 | 8 | 8 | 0 | 2610 | 1 |
| ctl:fzf (string literals) | 30 | 0 | 6 | 7 | 7 | 0 | 492 | 4 |
| ctl:gh | 75 | 0 | 8 | 9 | 9 | 0 | 5063 | 0 |
| ctl:gh (string literals) | 829 | 5 | 5 | 7 | 163 | 3 | 19573 | 621 |
| ctl:go-net-http (string literals) | 100 | 0 | 7 | 13 | 13 | 4 | 2971 | 26 |
| ctl:go-os-exec (string literals) | 11 | 0 | 5 | 5 | 5 | 0 | 146 | 0 |
| ctl:go-text-template (string literals) | 13 | 0 | 0 | 11 | 11 | 0 | 429 | 0 |
| ctl:mvdan-sh | 2 | 0 | 5 | 5 | 5 | 0 | 668 | 0 |
| ctl:mvdan-sh (string literals) | 41 | 5 | 6 | 10 | 10 | 0 | 1993 | 33 |
| ctl:ollama | 22 | 0 | 6 | 8 | 8 | 0 | 1097 | 0 |
| ctl:ollama (string literals) | 799 | 0 | 6 | 9 | 59 | 1 | 20098 | 83 |
| cal:crush-modified (string literals) | 252 | 9 | 43 | 72 | 144 | 96 | 4846 | 3926 |
| cal:crush-unchanged (string literals) | 286 | 9 | 27 | 50 | 106 | 86 | 2637 | 2637 |
| cal:old-prompts | 47 | 32 | 725 | 775 | 775 | 31 | 2117 | 787 |
| cal:old-rewrite (string literals) | 23 | 7 | 47 | 47 | 47 | 4 | 185 | 103 |
| cal:old-shortblock (string literals) | 18 | 6 | 12 | 47 | 47 | 2 | 457 | 76 |

### Words, against hist

| set | units of 20+ words | longest shared word run p50 | p95 | p99 | max | units with a run of 12+ words | sentences | of them also in the corpus |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| carried | 13 | 5 | 8 | 8 | 8 | 0 | 495 | 0 |
| carried (string literals) | 82 | 5 | 7 | 8 | 8 | 0 | 1773 | 40 |
| carried with larger changes | 10 | 0 | 9 | 9 | 9 | 0 | 296 | 1 |
| carried with larger changes (string literals) | 6 | 0 | 8 | 8 | 8 | 0 | 197 | 0 |
| carried with larger changes: doc | 2 | 0 | 8 | 8 | 8 | 0 | 325 | 0 |
| carried: doc | 4 | 6 | 16 | 16 | 16 | 1 | 1175 | 1 |
| carried: other | 4 | 0 | 0 | 0 | 0 | 0 | 24 | 0 |
| carried: testdata | 78 | 0 | 8 | 9 | 9 | 0 | 1583 | 1 |
| new code | 22 | 5 | 8 | 8 | 8 | 0 | 1016 | 0 |
| new code (string literals) | 423 | 0 | 8 | 12 | 21 | 7 | 16534 | 47 |
| new: documents | 45 | 5 | 9 | 16 | 16 | 2 | 11145 | 2 |
| new: go.mod, go.sum and the like | 6 | 9 | 93 | 93 | 93 | 2 | 19 | 0 |
| new: prompt texts | 45 | 0 | 5 | 6 | 6 | 0 | 334 | 0 |
| new: test data | 103 | 0 | 6 | 9 | 10 | 0 | 1374 | 0 |
| third-party (string literals) | 22 | 0 | 6 | 9 | 9 | 0 | 113 | 4 |
| third-party: doc | 2 | 16 | 16 | 16 | 16 | 2 | 403 | 2 |
| third-party: other | 3 | 39 | 218 | 218 | 218 | 2 | 16 | 7 |
| third-party: testdata | 4 | 11 | 11 | 11 | 11 | 0 | 260 | 0 |
| ctl:fzf | 10 | 5 | 9 | 9 | 9 | 0 | 2610 | 2 |
| ctl:fzf (string literals) | 30 | 0 | 7 | 7 | 7 | 0 | 492 | 4 |
| ctl:gh | 75 | 5 | 9 | 15 | 209 | 2 | 5063 | 15 |
| ctl:gh (string literals) | 829 | 5 | 7 | 9 | 163 | 4 | 19573 | 697 |
| ctl:go-net-http (string literals) | 100 | 5 | 13 | 19 | 19 | 8 | 2971 | 191 |
| ctl:go-os-exec (string literals) | 11 | 0 | 5 | 5 | 5 | 0 | 146 | 0 |
| ctl:go-text-template (string literals) | 13 | 0 | 6 | 11 | 11 | 0 | 429 | 0 |
| ctl:mvdan-sh | 2 | 8 | 11 | 11 | 11 | 0 | 668 | 0 |
| ctl:mvdan-sh (string literals) | 41 | 5 | 11 | 35 | 35 | 2 | 1993 | 116 |
| ctl:ollama | 22 | 0 | 8 | 8 | 8 | 0 | 1097 | 0 |
| ctl:ollama (string literals) | 799 | 0 | 6 | 9 | 326 | 1 | 20098 | 278 |
| cal:crush-modified (string literals) | 252 | 9 | 43 | 72 | 144 | 96 | 4846 | 3933 |
| cal:crush-unchanged (string literals) | 286 | 9 | 27 | 50 | 106 | 86 | 2637 | 2637 |
| cal:old-prompts | 47 | 32 | 725 | 775 | 775 | 31 | 2117 | 787 |
| cal:old-rewrite (string literals) | 23 | 7 | 47 | 47 | 47 | 4 | 185 | 103 |
| cal:old-shortblock (string literals) | 18 | 7 | 15 | 47 | 47 | 2 | 457 | 76 |

### Words, against libs

| set | units of 20+ words | longest shared word run p50 | p95 | p99 | max | units with a run of 12+ words | sentences | of them also in the corpus |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| carried | 13 | 5 | 8 | 9 | 9 | 0 | 495 | 0 |
| carried (string literals) | 82 | 5 | 6 | 7 | 7 | 0 | 1773 | 38 |
| carried with larger changes | 10 | 6 | 9 | 9 | 9 | 0 | 296 | 1 |
| carried with larger changes (string literals) | 6 | 0 | 6 | 6 | 6 | 0 | 197 | 0 |
| carried with larger changes: doc | 2 | 0 | 6 | 6 | 6 | 0 | 325 | 0 |
| carried: doc | 4 | 6 | 16 | 16 | 16 | 1 | 1175 | 1 |
| carried: other | 4 | 0 | 0 | 0 | 0 | 0 | 24 | 0 |
| carried: testdata | 78 | 0 | 8 | 12 | 12 | 4 | 1583 | 0 |
| new code | 22 | 5 | 8 | 8 | 8 | 0 | 1016 | 0 |
| new code (string literals) | 423 | 0 | 8 | 10 | 21 | 4 | 16534 | 26 |
| new: documents | 45 | 5 | 8 | 16 | 16 | 2 | 11145 | 1 |
| new: go.mod, go.sum and the like | 6 | 8 | 50 | 50 | 50 | 3 | 19 | 0 |
| new: prompt texts | 45 | 0 | 0 | 5 | 5 | 0 | 334 | 0 |
| new: test data | 103 | 0 | 5 | 9 | 9 | 0 | 1374 | 0 |
| third-party (string literals) | 22 | 0 | 0 | 6 | 6 | 0 | 113 | 2 |
| third-party: doc | 2 | 16 | 16 | 16 | 16 | 2 | 403 | 2 |
| third-party: other | 3 | 27 | 218 | 218 | 218 | 2 | 16 | 7 |
| third-party: testdata | 4 | 8 | 8 | 8 | 8 | 0 | 260 | 0 |
| ctl:fzf | 10 | 5 | 9 | 9 | 9 | 0 | 2610 | 0 |
| ctl:fzf (string literals) | 30 | 0 | 5 | 7 | 7 | 0 | 492 | 1 |
| ctl:gh | 75 | 5 | 10 | 13 | 13 | 4 | 5063 | 0 |
| ctl:gh (string literals) | 829 | 5 | 6 | 9 | 163 | 4 | 19573 | 638 |
| ctl:go-net-http (string literals) | 100 | 5 | 6 | 8 | 9 | 0 | 2971 | 26 |
| ctl:go-os-exec (string literals) | 11 | 0 | 5 | 5 | 5 | 0 | 146 | 0 |
| ctl:go-text-template (string literals) | 13 | 0 | 6 | 11 | 11 | 0 | 429 | 2 |
| ctl:mvdan-sh | 2 | 6 | 6 | 6 | 6 | 0 | 668 | 0 |
| ctl:mvdan-sh (string literals) | 41 | 0 | 7 | 11 | 11 | 0 | 1993 | 1 |
| ctl:ollama | 22 | 0 | 8 | 9 | 9 | 0 | 1097 | 0 |
| ctl:ollama (string literals) | 799 | 0 | 6 | 9 | 11 | 0 | 20098 | 164 |
| cal:crush-modified (string literals) | 252 | 5 | 8 | 8 | 12 | 1 | 4846 | 291 |
| cal:crush-unchanged (string literals) | 286 | 5 | 6 | 8 | 12 | 1 | 2637 | 300 |
| cal:old-prompts | 47 | 0 | 6 | 8 | 8 | 0 | 2117 | 0 |
| cal:old-rewrite (string literals) | 23 | 5 | 5 | 6 | 6 | 0 | 185 | 28 |
| cal:old-shortblock (string literals) | 18 | 5 | 6 | 7 | 7 | 0 | 457 | 18 |

### Words, against since

| set | units of 20+ words | longest shared word run p50 | p95 | p99 | max | units with a run of 12+ words | sentences | of them also in the corpus |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| carried | 13 | 0 | 5 | 5 | 5 | 0 | 495 | 0 |
| carried (string literals) | 82 | 0 | 5 | 5 | 5 | 0 | 1773 | 37 |
| carried with larger changes | 10 | 0 | 6 | 6 | 6 | 0 | 296 | 0 |
| carried with larger changes (string literals) | 6 | 0 | 0 | 0 | 0 | 0 | 197 | 0 |
| carried with larger changes: doc | 2 | 0 | 5 | 5 | 5 | 0 | 325 | 0 |
| carried: doc | 4 | 0 | 5 | 5 | 5 | 0 | 1175 | 0 |
| carried: other | 4 | 0 | 0 | 0 | 0 | 0 | 24 | 0 |
| carried: testdata | 78 | 0 | 0 | 8 | 9 | 0 | 1583 | 0 |
| new code | 22 | 0 | 6 | 8 | 8 | 0 | 1016 | 0 |
| new code (string literals) | 423 | 0 | 5 | 5 | 6 | 0 | 16534 | 2 |
| new: documents | 45 | 0 | 7 | 8 | 8 | 0 | 11145 | 1 |
| new: go.mod, go.sum and the like | 6 | 8 | 17 | 17 | 17 | 1 | 19 | 0 |
| new: prompt texts | 45 | 0 | 0 | 5 | 5 | 0 | 334 | 0 |
| new: test data | 103 | 0 | 0 | 5 | 5 | 0 | 1374 | 0 |
| third-party (string literals) | 22 | 0 | 0 | 0 | 0 | 0 | 113 | 0 |
| third-party: doc | 2 | 0 | 0 | 0 | 0 | 0 | 403 | 0 |
| third-party: other | 3 | 0 | 0 | 0 | 0 | 0 | 16 | 0 |
| third-party: testdata | 4 | 0 | 0 | 0 | 0 | 0 | 260 | 0 |
| ctl:fzf | 10 | 0 | 5 | 5 | 5 | 0 | 2610 | 0 |
| ctl:fzf (string literals) | 30 | 0 | 0 | 0 | 0 | 0 | 492 | 0 |
| ctl:gh | 75 | 0 | 0 | 0 | 5 | 0 | 5063 | 0 |
| ctl:gh (string literals) | 829 | 0 | 5 | 5 | 6 | 0 | 19573 | 581 |
| ctl:go-net-http (string literals) | 100 | 0 | 0 | 5 | 5 | 0 | 2971 | 0 |
| ctl:go-os-exec (string literals) | 11 | 0 | 0 | 0 | 0 | 0 | 146 | 0 |
| ctl:go-text-template (string literals) | 13 | 0 | 0 | 5 | 5 | 0 | 429 | 0 |
| ctl:mvdan-sh | 2 | 0 | 0 | 0 | 0 | 0 | 668 | 0 |
| ctl:mvdan-sh (string literals) | 41 | 0 | 5 | 5 | 5 | 0 | 1993 | 0 |
| ctl:ollama | 22 | 0 | 5 | 8 | 8 | 0 | 1097 | 0 |
| ctl:ollama (string literals) | 799 | 0 | 5 | 5 | 8 | 0 | 20098 | 18 |
| cal:crush-modified (string literals) | 252 | 6 | 9 | 45 | 47 | 10 | 4846 | 1353 |
| cal:crush-unchanged (string literals) | 286 | 6 | 7 | 10 | 33 | 2 | 2637 | 750 |
| cal:old-prompts | 47 | 0 | 5 | 5 | 5 | 0 | 2117 | 0 |
| cal:old-rewrite (string literals) | 23 | 7 | 47 | 47 | 47 | 3 | 185 | 76 |
| cal:old-shortblock (string literals) | 18 | 6 | 7 | 47 | 47 | 1 | 457 | 67 |

### The control range, and new code above it

| measure | corpus | highest in the control set | new code: highest | files of new code above the control set's highest |
|---|---|---:|---:|---|
| line share (10+ significant lines) | crush | 17.6% | 10.0% | none |
| line share, no generic-Go filter (10+ long lines) | crush | 22.5% | 10.0% | none |
| longest counted line run | crush | 0 | 0 | none |
| longest line run, counted or not | crush | 14 | 5 | none |
| exact tokens: longest varied run | crush | 117 | 53 | none |
| exact tokens: varied cover | crush | 28.0% | 3.7% | none |
| exact tokens: longest run, lists and tables included | crush | 501 | 321 | none |
| exact tokens: cover, lists and tables included | crush | 92.0% | 99.5% | `internal/markdown/entity.go` (99.5%) |
| abstract tokens: longest varied run | crush | 223 | 83 | none |
| abstract tokens: varied cover | crush | 69.4% | 47.2% | none |
| abstract tokens: longest run, lists and tables included | crush | 502 | 339 | none |
| abstract tokens: cover, lists and tables included | crush | 92.9% | 99.5% | `internal/markdown/entity.go` (99.5%) |
| line share (10+ significant lines) | hist | 17.6% | 10.0% | none |
| line share, no generic-Go filter (10+ long lines) | hist | 22.5% | 12.0% | none |
| longest counted line run | hist | 0 | 0 | none |
| longest line run, counted or not | hist | 14 | 5 | none |
| exact tokens: longest varied run | hist | 117 | 53 | none |
| exact tokens: varied cover | hist | 28.0% | 3.7% | none |
| exact tokens: longest run, lists and tables included | hist | 501 | 1770 | `internal/text/tables.go` (1770) |
| exact tokens: cover, lists and tables included | hist | 92.0% | 99.6% | `internal/markdown/entity.go` (99.6%); `internal/text/tables.go` (93.6%) |
| abstract tokens: longest varied run | hist | 223 | 117 | none |
| abstract tokens: varied cover | hist | 69.4% | 48.2% | none |
| abstract tokens: longest run, lists and tables included | hist | 502 | 1772 | `internal/text/tables.go` (1772) |
| abstract tokens: cover, lists and tables included | hist | 92.9% | 99.8% | `internal/markdown/entity.go` (99.8%); `internal/text/tables.go` (94.3%) |
| line share (10+ significant lines) | libs | 12.0% | 7.4% | none |
| line share, no generic-Go filter (10+ long lines) | libs | 41.7% | 11.8% | none |
| longest counted line run | libs | 4 | 0 | none |
| longest line run, counted or not | libs | 14 | 0 | none |
| exact tokens: longest varied run | libs | 62 | 55 | none |
| exact tokens: varied cover | libs | 59.4% | 7.2% | none |
| exact tokens: longest run, lists and tables included | libs | 343 | 434 | `internal/diff/lines_test.go` (434) |
| exact tokens: cover, lists and tables included | libs | 91.8% | 99.6% | `internal/markdown/entity.go` (99.6%); `internal/text/tables.go` (93.4%) |
| abstract tokens: longest varied run | libs | 125 | 121 | none |
| abstract tokens: varied cover | libs | 97.5% | 16.2% | none |
| abstract tokens: longest run, lists and tables included | libs | 465 | 434 | none |
| abstract tokens: cover, lists and tables included | libs | 98.8% | 44.8% | none |
| line share (10+ significant lines) | since | 17.6% | 0.8% | none |
| line share, no generic-Go filter (10+ long lines) | since | 22.5% | 6.7% | none |
| longest counted line run | since | 0 | 0 | none |
| longest line run, counted or not | since | 14 | 4 | none |
| exact tokens: longest varied run | since | 117 | 0 | none |
| exact tokens: varied cover | since | 28.0% | 0.0% | none |
| exact tokens: longest run, lists and tables included | since | 501 | 0 | none |
| exact tokens: cover, lists and tables included | since | 92.0% | 0.0% | none |
| abstract tokens: longest varied run | since | 223 | 61 | none |
| abstract tokens: varied cover | since | 69.4% | 12.2% | none |
| abstract tokens: longest run, lists and tables included | since | 502 | 126 | none |
| abstract tokens: cover, lists and tables included | since | 92.9% | 25.2% | none |

Files of new code above the control set on any measure: 3

- `internal/markdown/entity.go`: exact tokens: cover, lists and tables included vs crush: 99.5% (control 92.0%); abstract tokens: cover, lists and tables included vs crush: 99.5% (control 92.9%); exact tokens: cover, lists and tables included vs hist: 99.6% (control 92.0%); abstract tokens: cover, lists and tables included vs hist: 99.8% (control 92.9%); exact tokens: cover, lists and tables included vs libs: 99.6% (control 91.8%)
- `internal/text/tables.go`: exact tokens: longest run, lists and tables included vs hist: 1770 (control 501); exact tokens: cover, lists and tables included vs hist: 93.6% (control 92.0%); abstract tokens: longest run, lists and tables included vs hist: 1772 (control 502); abstract tokens: cover, lists and tables included vs hist: 94.3% (control 92.9%); exact tokens: cover, lists and tables included vs libs: 93.4% (control 91.8%)
- `internal/diff/lines_test.go`: exact tokens: longest run, lists and tables included vs libs: 434 (control 343)

### The twenty highest of each measure (new code)


**line share, against crush** (20 files above zero shown)

- `internal/gnovm/racedetector_off_test.go`: 10.0% <- base:internal/backend/race_off_test.go
- `internal/core/message.go`: 5.9% <- base:internal/cmd/session.go
- `internal/app/apptest/model.go`: 4.0% <- up:internal/agent/shared_models_test.go
- `internal/tools/shelltool/progress.go`: 3.7% <- up:internal/pinentry/pinentry.go
- `internal/filelock/filelock_test.go`: 3.6% <- base:internal/lock/lock_test.go
- `internal/app/print.go`: 3.3% <- base:internal/proto/message.go
- `tools/dev/scriptmodel/main.go`: 3.2% <- base:internal/message/content.go
- `internal/tools/fstools/spec_test.go`: 3.0% <- base:internal/agent/channelreply.go
- `internal/app/log.go`: 2.8% <- base:internal/cmd/session.go
- `internal/filelock/filelock_windows.go`: 2.8% <- hist:internal/dirid/dirid_windows.go
- `internal/app/doctor.go`: 2.4% <- hist:vendor/github.com/anthropics/anthropic-sdk-go/internal/requestconfig/requestconfig.go
- `internal/term/probe.go`: 2.3% <- base:internal/ui/common/capabilities.go
- `internal/memjoin/namer.go`: 2.2% <- base:internal/ui/chat/question.go
- `internal/config/dump.go`: 1.9% <- base:internal/agent/tools/bash.go
- `internal/config/value.go`: 1.9% <- base:internal/csync/value.go
- `internal/filelock/filelock_unix_test.go`: 1.7% <- base:internal/lock/lock_test.go
- `internal/tools/fstools/editdiag.go`: 1.7% <- base:internal/agent/tools/edit_whitespace.go
- `internal/app/apptest/world.go`: 1.6% <- base:internal/cmd/session_test.go
- `internal/tools/fstools/ignore/git_test.go`: 1.6% <- base:internal/cmd/session_test.go
- `internal/app/askuser.go`: 1.5% <- base:internal/proto/proto.go

**longest line run of 4+ (counted or not), against crush** (4 files above zero shown)

- `internal/memjoin/tasks.go`: 5
- `internal/agent/sink.go`: 4
- `internal/app/texts.go`: 4
- `internal/tools/fstools/ignore/ignore.go`: 4

**longest run of lines each somewhere in the corpus, against crush** (20 files above zero shown)

- `internal/filelock/filelock_unix_test.go`: 10
- `internal/config/jsonc_test.go`: 9
- `internal/filelock/filelock_test.go`: 9
- `internal/memjoin/frames.go`: 8
- `internal/agent/agent.go`: 7
- `internal/app/apptest/model.go`: 7
- `internal/llm/anthropic.go`: 7
- `internal/text/truncate_test.go`: 7
- `internal/tools/fstools/ignore/ignore_test.go`: 7
- `main_test.go`: 6
- `internal/app/session_test.go`: 6
- `internal/cli/help_test.go`: 6
- `internal/memjoin/namer.go`: 6
- `internal/tools/fstools/ignore/git_test.go`: 6
- `internal/agent/guard.go`: 5
- `internal/app/apptest/world.go`: 5
- `internal/llm/anthropic_test.go`: 5
- `internal/memjoin/unit_test.go`: 5
- `internal/permission/decide.go`: 5
- `internal/tools/fstools/helpers_test.go`: 5

**exact tokens: longest varied run, against crush** (3 files above zero shown)

- `internal/tools/shelltool/host_test.go`: 53 tokens, lines 915-925 <- base:internal/discover/discover_test.go@6d630adb lines 204-214
- `internal/filelock/filelock_test.go`: 41 tokens, lines 125-128 <- base:internal/lock/lock_test.go@6d7b8de5 lines 49-53
- `internal/memjoin/tasks.go`: 41 tokens, lines 142-152 <- base:internal/backend/backend.go@046e4427 lines 1178-1188

**exact tokens: longest run, lists and tables included, against crush** (20 files above zero shown)

- `internal/tools/shelltool/rules_test.go`: 321 tokens (15 distinct windows), lines 22-79 <- up:internal/agent/tools/safe_test.go@7c704312 lines 141-209
- `internal/term/vtest/tmux_test.go`: 268 tokens (10 distinct windows), lines 40-84 <- up:internal/agent/tools/safe_test.go@7c704312 lines 17-73
- `internal/tools/fstools/wild/wild_test.go`: 142 tokens (10 distinct windows), lines 524-531 <- up:internal/agent/tools/safe_test.go@7c704312 lines 48-80
- `internal/markdown/gen_test.go`: 135 tokens (7 distinct windows), lines 26-33 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-157
- `internal/permission/bash_test.go`: 134 tokens (6 distinct windows), lines 150-215 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-157
- `internal/permission/danger_test.go`: 134 tokens (6 distinct windows), lines 14-38 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-157
- `internal/term/vtest/modes_test.go`: 134 tokens (6 distinct windows), lines 457-463 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-157
- `internal/tools/fstools/ignore/git_test.go`: 134 tokens (6 distinct windows), lines 141-147 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-157
- `internal/ui/keys.go`: 133 tokens (7 distinct windows), lines 715-737 <- up:internal/agent/subagent_name.go@45379eeb lines 17-39
- `internal/cli/table_test.go`: 130 tokens (6 distinct windows), lines 198-211 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-155
- `internal/permission/fuzz_test.go`: 129 tokens (9 distinct windows), lines 12-27 <- base:internal/agent/tools/bash.go@8bb4164c lines 76-149
- `internal/ui/editor/keys_test.go`: 128 tokens (6 distinct windows), lines 398-407 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-154
- `internal/config/table.go`: 121 tokens (7 distinct windows), lines 147-160 <- base:internal/agent/tools/bash.go@8bb4164c lines 76-143
- `internal/diff/unified_test.go`: 117 tokens (9 distinct windows), lines 250-256 <- up:internal/agent/tools/safe_test.go@7c704312 lines 17-37
- `internal/config/hosts_test.go`: 115 tokens (7 distinct windows), lines 157-160 <- up:internal/agent/subagent_name.go@45379eeb lines 17-36
- `internal/permission/danger.go`: 115 tokens (11 distinct windows), lines 16-21 <- base:internal/lsp/manager.go@e9600c28 lines 123-150
- `internal/permission/decideshell.go`: 115 tokens (11 distinct windows), lines 510-515 <- base:internal/lsp/manager.go@e9600c28 lines 123-150
- `internal/permission/protect.go`: 115 tokens (11 distinct windows), lines 23-33 <- base:internal/lsp/manager.go@e9600c28 lines 123-150
- `internal/tools/fstools/paths_test.go`: 114 tokens (48 distinct windows), lines 37-53 <- base:internal/agent/opencode_routing_test.go@f70b34fe lines 30-44
- `internal/permission/readonly.go`: 111 tokens (11 distinct windows), lines 470-474 <- base:internal/lsp/manager.go@e9600c28 lines 123-149

**abstract tokens: longest varied run, against crush** (20 files above zero shown)

- `internal/tools/shelltool/bash_test.go`: 83 tokens, lines 380-391 <- up:internal/server/theme_test.go@481beb6f lines 103-114
- `internal/tools/shelltool/host_test.go`: 79 tokens, lines 909-929 <- up:internal/agent/tools/safe_test.go@7c704312 lines 84-107
- `internal/tools/shelltool/rules_test.go`: 79 tokens, lines 10-30 <- up:internal/agent/tools/safe_test.go@7c704312 lines 4-27
- `internal/prompt/wrap_test.go`: 77 tokens, lines 55-64 <- base:internal/ui/diffview/util_test.go@3bba5070 lines 14-24
- `internal/instructions/load_test.go`: 76 tokens, lines 484-488 <- up:internal/proto/theme_test.go@e5784bb8 lines 29-33
- `internal/config/jsonc_test.go`: 76 tokens, lines 102-112 <- base:internal/filepathext/filepath_test.go@1de33104 lines 18-31
- `internal/tools/shelltool/job.go`: 73 tokens, lines 582-601 <- up:internal/agent/agent_dispatch.go@a99861f7 lines 514-539
- `internal/transcript/session.go`: 73 tokens, lines 46-66 <- up:internal/agent/agent_dispatch.go@a99861f7 lines 514-539
- `main_test.go`: 71 tokens, lines 78-92 <- base:internal/backend/backend_test.go@094fa47b lines 652-667
- `internal/app/harness_test.go`: 71 tokens, lines 34-49 <- base:internal/backend/backend_test.go@094fa47b lines 652-667
- `internal/memjoin/harness_test.go`: 70 tokens, lines 32-47 <- base:internal/shell/background.go@d401849d lines 36-51
- `internal/ui/editor/editor.go`: 68 tokens, lines 543-561 <- base:internal/ui/list/list.go@f50e4e57 lines 104-123
- `internal/ui/harness_test.go`: 67 tokens, lines 654-670 <- base:internal/workspace/client_workspace_test.go@0a0347b8 lines 652-664
- `internal/tools/fstools/state_test.go`: 67 tokens, lines 58-68 <- base:internal/agent/tools/mcp/channel_test.go@08eec34c lines 302-312
- `internal/ui/e2e_test.go`: 67 tokens, lines 67-81 <- base:internal/shell/background.go@d401849d lines 36-49
- `internal/prompt/replace_test.go`: 66 tokens, lines 4-16 <- base:internal/shell/dispatch_test.go@b1fb4dc6 lines 9-24
- `internal/permission/danger.go`: 65 tokens, lines 4-19 <- base:internal/fsext/ls.go@5c0d6abb lines 16-36
- `internal/ui/menu/menu.go`: 64 tokens, lines 149-158 <- base:internal/ui/xchroma/chroma.go@3ebb7879 lines 94-103
- `internal/permission/decideshell.go`: 63 tokens, lines 505-513 <- base:internal/lsp/manager.go@e9600c28 lines 119-137
- `internal/permission/protect.go`: 63 tokens, lines 20-28 <- base:internal/lsp/manager.go@e9600c28 lines 119-137

**abstract tokens: longest run, lists and tables included, against crush** (20 files above zero shown)

- `internal/tools/shelltool/rules_test.go`: 339 tokens (33 distinct windows), lines 15-77 <- up:internal/agent/tools/safe_test.go@7c704312 lines 91-167
- `internal/term/vtest/tmux_test.go`: 268 tokens (10 distinct windows), lines 40-84 <- up:internal/agent/tools/safe_test.go@7c704312 lines 17-73
- `internal/tools/fstools/wild/wild_test.go`: 250 tokens (18 distinct windows), lines 86-120 <- up:internal/agent/tools/safe_test.go@a9c59550 lines 17-52
- `internal/permission/path_test.go`: 166 tokens (14 distinct windows), lines 139-169 <- base:internal/agent/opencode_routing_test.go@f70b34fe lines 16-38
- `internal/permission/guard_test.go`: 158 tokens (14 distinct windows), lines 24-45 <- base:internal/agent/opencode_routing_test.go@f70b34fe lines 16-37
- `internal/agent/notes_test.go`: 150 tokens (22 distinct windows), lines 16-37 <- base:internal/agent/opencode_routing_test.go@f70b34fe lines 14-35
- `internal/tools/fstools/paths_test.go`: 143 tokens (23 distinct windows), lines 29-53 <- base:internal/agent/opencode_routing_test.go@f70b34fe lines 12-34
- `internal/permission/fuzz_test.go`: 139 tokens (19 distinct windows), lines 12-27 <- up:internal/agent/tools/bash.go@6076d9d2 lines 80-161
- `internal/markdown/gen_test.go`: 138 tokens (10 distinct windows), lines 21-33 <- up:internal/agent/tools/bash.go@9261ba18 lines 75-157
- `internal/ui/keys.go`: 135 tokens (9 distinct windows), lines 715-737 <- up:internal/agent/subagent_name.go@45379eeb lines 17-39
- `internal/permission/bash_test.go`: 134 tokens (6 distinct windows), lines 150-215 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-157
- `internal/permission/danger_test.go`: 134 tokens (6 distinct windows), lines 14-38 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-157
- `internal/term/vtest/modes_test.go`: 134 tokens (6 distinct windows), lines 457-463 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-157
- `internal/tools/fstools/ignore/git_test.go`: 134 tokens (6 distinct windows), lines 141-147 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-157
- `internal/cli/table_test.go`: 130 tokens (6 distinct windows), lines 198-211 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-155
- `internal/ui/editor/keys_test.go`: 128 tokens (6 distinct windows), lines 398-407 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-154
- `internal/tools/shelltool/text.go`: 126 tokens (14 distinct windows), lines 60-95 <- base:internal/ui/chat/tools.go@52c0ab04 lines 1801-1836
- `internal/config/table.go`: 124 tokens (10 distinct windows), lines 140-160 <- base:internal/agent/tools/bash.go@8bb4164c lines 74-143
- `internal/cli/cli.go`: 119 tokens (7 distinct windows), lines 21-74 <- base:internal/ui/styles/styles.go@04629639 lines 22-81
- `internal/diff/unified_test.go`: 118 tokens (10 distinct windows), lines 250-256 <- up:internal/agent/tools/safe_test.go@7c704312 lines 17-37

**line share, against hist** (20 files above zero shown)

- `internal/gnovm/racedetector_off_test.go`: 10.0% <- base:internal/backend/race_off_test.go
- `internal/core/message.go`: 8.8% <- base:internal/cmd/session.go
- `internal/term/pty_darwin_test.go`: 6.2% <- hist:vendor/github.com/ncruces/go-sqlite3/internal/util/mem.go
- `internal/filelock/filelock_windows.go`: 5.6% <- hist:internal/dirid/dirid_windows.go
- `internal/core/errors.go`: 5.1% <- hist:vendor/github.com/aws/aws-sdk-go-v2/credentials/processcreds/provider.go
- `internal/core/llm.go`: 5.1% <- hist:internal/llm/provider/provider.go
- `internal/app/print.go`: 4.9% <- base:internal/proto/message.go
- `internal/app/doctor.go`: 4.7% <- hist:vendor/github.com/anthropics/anthropic-sdk-go/internal/requestconfig/requestconfig.go
- `internal/llm/anthropic.go`: 4.5% <- hist:vendor/github.com/anthropics/anthropic-sdk-go/betamessage.go
- `internal/app/apptest/model.go`: 4.0% <- up:internal/agent/shared_models_test.go
- `internal/tools/shelltool/progress.go`: 3.7% <- up:internal/pinentry/pinentry.go
- `internal/filelock/filelock_test.go`: 3.6% <- base:internal/lock/lock_test.go
- `internal/tools/shelltool/text.go`: 3.3% <- hist:internal/cmd/root.go
- `internal/app/log.go`: 3.2% <- base:internal/cmd/session.go
- `tools/dev/scriptmodel/main.go`: 3.2% <- base:internal/message/content.go
- `internal/tools/fstools/spec_test.go`: 3.0% <- base:internal/agent/channelreply.go
- `internal/tools/shelltool/standin_test.go`: 3.0% <- base:internal/discover/litellm.go
- `internal/text/profile.go`: 2.9% <- hist:vendor/github.com/charmbracelet/colorprofile/profile.go
- `internal/config/dump.go`: 2.8% <- base:internal/agent/tools/bash.go
- `internal/app/run_test.go`: 2.7% <- base:internal/hooks/input.go

**longest line run of 4+ (counted or not), against hist** (4 files above zero shown)

- `internal/memjoin/tasks.go`: 5
- `internal/agent/sink.go`: 4
- `internal/app/texts.go`: 4
- `internal/tools/fstools/ignore/ignore.go`: 4

**longest run of lines each somewhere in the corpus, against hist** (20 files above zero shown)

- `internal/memjoin/erase.go`: 11
- `internal/filelock/filelock_unix_test.go`: 10
- `internal/config/jsonc_test.go`: 9
- `internal/filelock/filelock_test.go`: 9
- `internal/llm/anthropic.go`: 9
- `internal/transcript/large_test.go`: 9
- `internal/hooks/runner.go`: 8
- `internal/memjoin/frames.go`: 8
- `internal/ui/tool.go`: 8
- `internal/agent/agent.go`: 7
- `internal/app/misc.go`: 7
- `internal/app/moments.go`: 7
- `internal/app/apptest/model.go`: 7
- `internal/memjoin/goal.go`: 7
- `internal/term/terminal_unix.go`: 7
- `internal/text/truncate_test.go`: 7
- `internal/tools/fstools/helpers_test.go`: 7
- `internal/tools/fstools/ignore/ignore_test.go`: 7
- `internal/trust/folder_test.go`: 7
- `internal/trust/record_test.go`: 7

**exact tokens: longest varied run, against hist** (6 files above zero shown)

- `internal/tools/shelltool/host_test.go`: 53 tokens, lines 915-925 <- base:internal/discover/discover_test.go@6d630adb lines 204-214
- `internal/ui/tool.go`: 53 tokens, lines 120-124 <- hist:internal/tui/components/anim/anim.go@0bd7a775 lines 303-307
- `internal/app/print.go`: 45 tokens, lines 751-753 <- hist:internal/jq/jq.go@b7cf39d8 lines 81-83
- `internal/filelock/filelock_test.go`: 41 tokens, lines 125-128 <- base:internal/lock/lock_test.go@6d7b8de5 lines 49-53
- `internal/memjoin/tasks.go`: 41 tokens, lines 142-152 <- base:internal/backend/backend.go@046e4427 lines 1178-1188
- `internal/tools/shelltool/bash_test.go`: 41 tokens, lines 385-390 <- hist:internal/honcho/service_test.go@f170e523 lines 763-769

**exact tokens: longest run, lists and tables included, against hist** (20 files above zero shown)

- `internal/text/tables.go`: 1770 tokens (3 distinct windows), lines 381-1265 <- hist:vendor/golang.org/x/text/unicode/norm/tables10.0.0.go@1af161c7 lines 6715-7599
- `internal/permission/bash_test.go`: 336 tokens (6 distinct windows), lines 340-384 <- hist:vendor/github.com/alecthomas/chroma/v2/lexers/cl.go@3eb0c230 lines 8-40
- `internal/tools/fstools/ignore/git_test.go`: 331 tokens (7 distinct windows), lines 141-154 <- hist:vendor/github.com/alecthomas/chroma/v2/lexers/raku.go@8085bc7e lines 95-125
- `internal/tools/shelltool/rules_test.go`: 321 tokens (15 distinct windows), lines 22-79 <- up:internal/agent/tools/safe_test.go@7c704312 lines 141-209
- `internal/term/vtest/tmux_test.go`: 268 tokens (10 distinct windows), lines 40-84 <- up:internal/agent/tools/safe_test.go@7c704312 lines 17-73
- `internal/permission/decideshell.go`: 255 tokens (11 distinct windows), lines 520-530 <- hist:vendor/github.com/yuin/goldmark/parser/html_block.go@09dc21f4 lines 13-75
- `internal/markdown/entity.go`: 239 tokens (11 distinct windows), lines 8-66 <- hist:vendor/golang.org/x/net/html/foreign.go@e8515d8e lines 162-220
- `internal/permission/protect.go`: 171 tokens (11 distinct windows), lines 23-38 <- hist:vendor/github.com/yuin/goldmark/parser/html_block.go@09dc21f4 lines 13-54
- `internal/permission/danger_test.go`: 159 tokens (7 distinct windows), lines 14-42 <- hist:vendor/github.com/alecthomas/chroma/v2/lexers/raku.go@8085bc7e lines 56-64
- `internal/tools/shelltool/bash_test.go`: 151 tokens (11 distinct windows), lines 107-143 <- hist:internal/llm/tools/auto_vscode.go@4d2357d1 lines 136-172
- `internal/markdown/gen_test.go`: 145 tokens (7 distinct windows), lines 26-33 <- hist:vendor/github.com/alecthomas/chroma/v2/lexers/cl.go@3eb0c230 lines 8-22
- `internal/term/vtest/modes_test.go`: 143 tokens (7 distinct windows), lines 457-463 <- hist:vendor/github.com/alecthomas/chroma/v2/lexers/raku.go@8085bc7e lines 56-63
- `internal/tools/fstools/wild/wild_test.go`: 142 tokens (10 distinct windows), lines 524-531 <- up:internal/agent/tools/safe_test.go@7c704312 lines 48-80
- `internal/ui/keys.go`: 133 tokens (7 distinct windows), lines 715-737 <- up:internal/agent/subagent_name.go@45379eeb lines 17-39
- `internal/tools/fstools/filetypes.go`: 131 tokens (11 distinct windows), lines 101-132 <- hist:vendor/github.com/alecthomas/chroma/v2/colour.go@e33d0106 lines 11-46
- `internal/cli/table_test.go`: 130 tokens (6 distinct windows), lines 198-211 <- up:internal/agent/tools/bash.go@9261ba18 lines 80-155
- `internal/permission/fuzz_test.go`: 129 tokens (9 distinct windows), lines 12-27 <- base:internal/agent/tools/bash.go@8bb4164c lines 76-149
- `internal/ui/editor/keys_test.go`: 129 tokens (7 distinct windows), lines 398-407 <- hist:vendor/github.com/alecthomas/chroma/v2/lexers/raku.go@8085bc7e lines 56-62
- `internal/config/table.go`: 122 tokens (8 distinct windows), lines 147-161 <- hist:internal/llm/tools/bash.go@18533b76 lines 47-55
- `internal/permission/danger.go`: 119 tokens (11 distinct windows), lines 16-21 <- hist:vendor/github.com/yuin/goldmark/parser/html_block.go@09dc21f4 lines 13-41

**abstract tokens: longest varied run, against hist** (20 files above zero shown)

- `internal/permission/decideshell.go`: 117 tokens, lines 513-523 <- hist:internal/honcho/capture.go@7df2d890 lines 21-32
- `internal/permission/protect.go`: 111 tokens, lines 17-28 <- hist:internal/honcho/capture.go@7df2d890 lines 21-32
- `internal/memjoin/harness_test.go`: 97 tokens, lines 27-46 <- hist:internal/lsphost/lsphost_test.go@75baff33 lines 65-85
- `internal/ui/e2e_test.go`: 97 tokens, lines 59-81 <- hist:internal/lsphost/lsphost_test.go@75baff33 lines 62-84
- `internal/memjoin/frames.go`: 96 tokens, lines 210-217 <- hist:internal/honcho/capture.go@7df2d890 lines 21-30
- `internal/markdown/spec_test.go`: 94 tokens, lines 236-254 <- hist:vendor/golang.org/x/net/html/foreign.go@e8515d8e lines 104-128
- `main_test.go`: 94 tokens, lines 70-88 <- hist:internal/lsphost/lsphost_test.go@75baff33 lines 61-80
- `internal/app/harness_test.go`: 94 tokens, lines 25-44 <- hist:internal/lsphost/lsphost_test.go@75baff33 lines 61-80
- `internal/term/terminal_child_test.go`: 93 tokens, lines 460-478 <- hist:internal/lsphost/lsphost_test.go@75baff33 lines 62-80
- `internal/transcript/id.go`: 90 tokens, lines 57-65 <- hist:vendor/github.com/google/uuid/uuid.go@5232b486 lines 260-268
- `internal/permission/readonly.go`: 87 tokens, lines 472-480 <- hist:internal/shell/pty.go@ae5f88b4 lines 200-210
- `internal/term/event.go`: 86 tokens, lines 100-133 <- hist:vendor/github.com/gorilla/css/scanner/scanner.go@25a7c657 lines 46-87
- `internal/tools/shelltool/bash_test.go`: 83 tokens, lines 380-391 <- up:internal/server/theme_test.go@481beb6f lines 103-114
- `internal/tools/shelltool/host_test.go`: 79 tokens, lines 909-929 <- up:internal/agent/tools/safe_test.go@7c704312 lines 84-107
- `internal/transcript/session.go`: 79 tokens, lines 31-49 <- hist:vendor/github.com/tetratelabs/wazero/internal/wasm/function_definition.go@c5f6e912 lines 127-152
- `internal/tools/shelltool/rules_test.go`: 79 tokens, lines 10-30 <- up:internal/agent/tools/safe_test.go@7c704312 lines 4-27
- `internal/prompt/wrap_test.go`: 77 tokens, lines 55-64 <- base:internal/ui/diffview/util_test.go@3bba5070 lines 14-24
- `internal/instructions/load_test.go`: 76 tokens, lines 484-488 <- up:internal/proto/theme_test.go@e5784bb8 lines 29-33
- `internal/config/jsonc_test.go`: 76 tokens, lines 102-112 <- base:internal/filepathext/filepath_test.go@1de33104 lines 18-31
- `internal/agent/agent.go`: 75 tokens, lines 690-710 <- hist:internal/terminal/terminal.go@de7b97aa lines 284-302

**abstract tokens: longest run, lists and tables included, against hist** (20 files above zero shown)

- `internal/text/tables.go`: 1772 tokens (5 distinct windows), lines 381-1265 <- hist:vendor/golang.org/x/text/unicode/norm/tables10.0.0.go@1af161c7 lines 6715-7599
- `internal/tools/shelltool/rules_test.go`: 339 tokens (33 distinct windows), lines 15-77 <- up:internal/agent/tools/safe_test.go@7c704312 lines 91-167
- `internal/permission/bash_test.go`: 336 tokens (6 distinct windows), lines 340-384 <- hist:vendor/github.com/alecthomas/chroma/v2/lexers/cl.go@3eb0c230 lines 8-40
- `internal/tools/fstools/ignore/git_test.go`: 332 tokens (8 distinct windows), lines 141-154 <- hist:vendor/github.com/alecthomas/chroma/v2/lexers/raku.go@8085bc7e lines 95-125
- `internal/term/vtest/tmux_test.go`: 268 tokens (10 distinct windows), lines 40-84 <- up:internal/agent/tools/safe_test.go@7c704312 lines 17-73
- `internal/permission/decideshell.go`: 259 tokens (15 distinct windows), lines 515-530 <- hist:vendor/github.com/charmbracelet/ultraviolet/key.go@b6425cc7 lines 612-680
- `internal/tools/fstools/wild/wild_test.go`: 250 tokens (18 distinct windows), lines 86-120 <- up:internal/agent/tools/safe_test.go@a9c59550 lines 17-52
- `internal/markdown/entity.go`: 241 tokens (13 distinct windows), lines 8-66 <- hist:vendor/golang.org/x/net/html/foreign.go@e8515d8e lines 162-220
- `internal/permission/names.go`: 180 tokens (9 distinct windows), lines 97-113 <- hist:vendor/github.com/charmbracelet/ultraviolet/key_table.go@285ed70f lines 152-178
- `internal/permission/protect.go`: 175 tokens (15 distinct windows), lines 19-38 <- hist:vendor/github.com/charmbracelet/ultraviolet/key.go@b6425cc7 lines 612-658
- `internal/permission/path_test.go`: 166 tokens (14 distinct windows), lines 139-169 <- base:internal/agent/opencode_routing_test.go@f70b34fe lines 16-38
- `internal/permission/danger_test.go`: 162 tokens (10 distinct windows), lines 12-42 <- hist:vendor/github.com/alecthomas/chroma/v2/lexers/raku.go@8085bc7e lines 53-64
- `internal/permission/guard_test.go`: 158 tokens (14 distinct windows), lines 24-45 <- base:internal/agent/opencode_routing_test.go@f70b34fe lines 16-37
- `internal/prompt/prompt.go`: 153 tokens (9 distinct windows), lines 39-92 <- hist:internal/tui/theme/gruvbox.go@ed544b84 lines 13-69
- `internal/tools/fstools/paths_test.go`: 153 tokens (33 distinct windows), lines 29-56 <- hist:internal/agent/prompt/model_family_test.go@ac9764f5 lines 10-43
- `internal/tools/shelltool/bash_test.go`: 153 tokens (13 distinct windows), lines 106-143 <- hist:internal/llm/tools/auto_vscode.go@4d2357d1 lines 134-172
- `internal/agent/notes_test.go`: 150 tokens (22 distinct windows), lines 16-37 <- base:internal/agent/opencode_routing_test.go@f70b34fe lines 14-35
- `internal/markdown/gen_test.go`: 149 tokens (11 distinct windows), lines 27-37 <- hist:internal/llm/tools/bash.go@a78c0321 lines 46-75
- `internal/text/escape_test.go`: 146 tokens (18 distinct windows), lines 134-153 <- hist:internal/shell/shell_test.go@417743ca lines 97-121
- `internal/term/vtest/modes_test.go`: 145 tokens (9 distinct windows), lines 456-463 <- hist:vendor/github.com/alecthomas/chroma/v2/lexers/raku.go@8085bc7e lines 54-63

**line share, against libs** (20 files above zero shown)

- `internal/core/message.go`: 7.4% <- libs:charm.land/fantasy@v0.45.1/providers/openrouter/provider_options.go
- `internal/text/bench_test.go`: 5.3% <- libs:charm.land/lipgloss/v2@v2.0.6/style_test.go
- `internal/app/doctor.go`: 3.5% <- libs:charm.land/fantasy@v0.45.1/providers/kronk/provider_options.go
- `internal/tools/shelltool/text.go`: 3.3% <- libs:github.com/charmbracelet/ultraviolet@v0.0.0-20261008173134-6b8d4baf91b4/layout/layout_test.go
- `tools/dev/scriptmodel/main.go`: 3.2% <- libs:charm.land/catwalk@v0.52.49/cmd/openrouter/main.go
- `internal/tools/fstools/spec_test.go`: 3.0% <- libs:charm.land/fantasy@v0.45.1/agent.go
- `internal/app/print.go`: 2.8% <- libs:charm.land/fantasy@v0.45.1/content_json.go
- `internal/filelock/filelock_windows.go`: 2.8% <- libs:github.com/charmbracelet/ultraviolet@v0.0.0-20261008173134-6b8d4baf91b4/poll_windows.go
- `internal/llm/anthropic.go`: 2.8% <- libs:charm.land/fantasy@v0.45.1/providers/anthropic/anthropic.go
- `internal/core/errors.go`: 2.6% <- libs:charm.land/fantasy@v0.45.1/errors.go
- `internal/term/probe.go`: 2.3% <- libs:charm.land/catwalk@v0.52.49/cmd/neuralwatt/main.go
- `internal/term/seq.go`: 2.2% <- libs:github.com/charmbracelet/x/ansi@v0.11.9/cursor.go
- `internal/llm/stable_test.go`: 2.1% <- libs:charm.land/fantasy@v0.45.1/providers/openaicompat/strict_upstream_test.go
- `internal/app/apptest/model.go`: 1.6% <- libs:charm.land/fantasy@v0.45.1/providers/openaicompat/header_test.go
- `internal/llm/transport.go`: 1.5% <- libs:github.com/charmbracelet/openai-go@v0.0.0-20260921175203-216db9e71b83/README.md
- `internal/tools/shelltool/standin_test.go`: 1.5% <- libs:github.com/charmbracelet/openai-go@v0.0.0-20260921175203-216db9e71b83/betaagent.go
- `internal/ui/editor/item.go`: 1.5% <- libs:charm.land/bubbles/v2@v2.2.1/table/table_test.go
- `internal/llm/helpers_test.go`: 1.4% <- libs:charm.land/fantasy@v0.45.1/providers/openaicompat/header_test.go
- `internal/prompt/system.go`: 1.4% <- libs:charm.land/fantasy@v0.45.1/agent.go
- `internal/text/profile.go`: 1.4% <- libs:github.com/charmbracelet/colorprofile@v0.4.3/profile.go

**longest line run of 4+ (counted or not), against libs** (0 files above zero shown)


**longest run of lines each somewhere in the corpus, against libs** (16 files above zero shown)

- `internal/text/width_test.go`: 10
- `internal/text/style_test.go`: 7
- `internal/trust/record_test.go`: 7
- `internal/llm/sse.go`: 6
- `internal/llm/transport.go`: 6
- `internal/llm/anthropic_test.go`: 5
- `internal/text/bench_test.go`: 5
- `internal/text/escape_test.go`: 5
- `internal/transcript/store.go`: 5
- `internal/app/doctor.go`: 4
- `internal/app/print.go`: 4
- `internal/diff/render_test.go`: 4
- `internal/diff/spans.go`: 4
- `internal/llm/anthropic.go`: 4
- `internal/term/seq.go`: 4
- `tools/dev/scriptmodel/main.go`: 4

**exact tokens: longest varied run, against libs** (8 files above zero shown)

- `internal/tools/shelltool/bash_test.go`: 55 tokens, lines 383-389 <- libs:charm.land/lipgloss/v2@v2.0.6/borders_test.go lines 124-131
- `internal/config/jsonc_test.go`: 55 tokens, lines 105-111 <- libs:charm.land/lipgloss/v2@v2.0.6/borders_test.go lines 124-131
- `internal/llm/llamacpp_test.go`: 46 tokens, lines 397-401 <- libs:github.com/charmbracelet/openai-go@v0.0.0-20260921175203-216db9e71b83/agents_helpers_test.go lines 478-482
- `internal/llm/errors_test.go`: 45 tokens, lines 274-276 <- libs:charm.land/fantasy@v0.45.1/retry_test.go lines 429-431
- `internal/tools/shelltool/parse_test.go`: 45 tokens, lines 195-198 <- libs:github.com/charmbracelet/x/ansi@v0.11.9/parser_decode_test.go lines 374-377
- `internal/ui/editor/keys_test.go`: 44 tokens, lines 301-304 <- libs:github.com/charmbracelet/openai-go@v0.0.0-20260921175203-216db9e71b83/option/x509workloadidentity_test.go lines 57-61
- `internal/trust/question_test.go`: 43 tokens, lines 151-159 <- libs:github.com/charmbracelet/openai-go@v0.0.0-20260921175203-216db9e71b83/internal/requestconfig/requestconfig_test.go lines 145-153
- `internal/agent/fuzz_test.go`: 41 tokens, lines 46-51 <- libs:github.com/charmbracelet/ultraviolet@v0.0.0-20261008173134-6b8d4baf91b4/key_test.go lines 1370-1375

**exact tokens: longest run, lists and tables included, against libs** (20 files above zero shown)

- `internal/diff/lines_test.go`: 434 tokens (11 distinct windows), lines 32-94 <- libs:charm.land/lipgloss/v2@v2.0.6/table/table_test.go lines 1209-1253
- `internal/config/table.go`: 263 tokens (11 distinct windows), lines 327-357 <- libs:charm.land/lipgloss/v2@v2.0.6/table/table_test.go lines 1283-1310
- `internal/tools/fstools/wild/wild_test.go`: 124 tokens (10 distinct windows), lines 524-531 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 14-42
- `internal/tools/shelltool/rules_test.go`: 124 tokens (10 distinct windows), lines 56-79 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 14-42
- `internal/term/vtest/tmux_test.go`: 123 tokens (9 distinct windows), lines 40-60 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 12-40
- `internal/ui/keys.go`: 121 tokens (7 distinct windows), lines 715-735 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 13-40
- `internal/diff/unified_test.go`: 117 tokens (9 distinct windows), lines 250-256 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 12-39
- `internal/config/hosts_test.go`: 115 tokens (7 distinct windows), lines 157-160 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 13-39
- `internal/text/gen/main.go`: 109 tokens (7 distinct windows), lines 558-576 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 13-38
- `internal/config/jsonc_test.go`: 106 tokens (34 distinct windows), lines 99-111 <- libs:charm.land/lipgloss/v2@v2.0.6/borders_test.go lines 118-131
- `internal/tools/shelltool/bash_test.go`: 106 tokens (34 distinct windows), lines 377-389 <- libs:charm.land/lipgloss/v2@v2.0.6/borders_test.go lines 118-131
- `internal/llm/json_test.go`: 101 tokens (11 distinct windows), lines 13-23 <- libs:charm.land/lipgloss/v2@v2.0.6/table/table_test.go lines 1209-1219
- `internal/text/escape_test.go`: 100 tokens (10 distinct windows), lines 203-220 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 18-42
- `internal/memjoin/close_test.go`: 93 tokens (9 distinct windows), lines 239-254 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 12-33
- `internal/markdown/gen_test.go`: 91 tokens (7 distinct windows), lines 26-32 <- libs:charm.land/fantasy@v0.45.1/providers/openai/responses_options.go lines 207-250
- `internal/permission/fuzz_test.go`: 91 tokens (7 distinct windows), lines 12-22 <- libs:charm.land/fantasy@v0.45.1/providers/openai/responses_options.go lines 207-250
- `internal/tools/shelltool/host_test.go`: 91 tokens (13 distinct windows), lines 919-936 <- libs:charm.land/bubbles/v2@v2.2.1/internal/runeutil/runeutil_test.go lines 10-28
- `internal/cli/table_test.go`: 90 tokens (6 distinct windows), lines 198-206 <- libs:charm.land/fantasy@v0.45.1/providers/openai/responses_options.go lines 207-250
- `internal/config/load_test.go`: 90 tokens (6 distinct windows), lines 1204-1247 <- libs:charm.land/fantasy@v0.45.1/providers/openai/responses_options.go lines 207-250
- `internal/permission/bash_test.go`: 90 tokens (6 distinct windows), lines 317-326 <- libs:charm.land/fantasy@v0.45.1/providers/openai/responses_options.go lines 207-250

**abstract tokens: longest varied run, against libs** (20 files above zero shown)

- `internal/llm/errors_test.go`: 121 tokens, lines 272-277 <- libs:charm.land/fantasy@v0.45.1/retry_test.go lines 424-429
- `internal/term/vtest/screen_test.go`: 79 tokens, lines 239-247 <- libs:github.com/charmbracelet/x/ansi@v0.11.9/wcwidth_test.go lines 28-37
- `internal/transcript/session.go`: 77 tokens, lines 32-49 <- libs:charm.land/lipgloss/v2@v2.0.6/table/table.go lines 216-240
- `internal/tools/shelltool/host_test.go`: 77 tokens, lines 911-929 <- libs:charm.land/fantasy@v0.45.1/schema/schema_test.go lines 424-441
- `internal/tools/shelltool/bash_test.go`: 76 tokens, lines 380-390 <- libs:charm.land/lipgloss/v2@v2.0.6/borders_test.go lines 121-132
- `internal/config/jsonc_test.go`: 76 tokens, lines 102-112 <- libs:charm.land/lipgloss/v2@v2.0.6/borders_test.go lines 121-132
- `internal/tools/shelltool/rules_test.go`: 76 tokens, lines 15-30 <- libs:charm.land/fantasy@v0.45.1/schema/schema_test.go lines 426-441
- `internal/tools/shelltool/job.go`: 73 tokens, lines 582-601 <- libs:charm.land/log/v2@v2.0.1/logger.go lines 246-263
- `internal/text/grapheme_test.go`: 72 tokens, lines 155-162 <- libs:github.com/charmbracelet/x/ansi@v0.11.9/color_test.go lines 83-92
- `internal/text/escape_test.go`: 71 tokens, lines 172-184 <- libs:charm.land/bubbles/v2@v2.2.1/internal/runeutil/runeutil_test.go lines 8-20
- `internal/llm/json_test.go`: 71 tokens, lines 133-136 <- libs:charm.land/bubbles/v2@v2.2.1/internal/runeutil/runeutil_test.go lines 8-20
- `internal/tools/fstools/wild/wild_test.go`: 71 tokens, lines 131-144 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 9-23
- `internal/llm/think_test.go`: 71 tokens, lines 104-117 <- libs:charm.land/lipgloss/v2@v2.0.6/borders_test.go lines 181-194
- `internal/term/vtest/screen.go`: 71 tokens, lines 212-241 <- libs:charm.land/bubbles/v2@v2.2.1/tree/tree.go lines 222-242
- `internal/transcript/frames_spec_test.go`: 70 tokens, lines 122-131 <- libs:charm.land/bubbles/v2@v2.2.1/internal/runeutil/runeutil_test.go lines 8-19
- `internal/tools/shelltool/parse_test.go`: 70 tokens, lines 193-198 <- libs:github.com/charmbracelet/x/ansi@v0.11.9/parser_decode_test.go lines 372-377
- `internal/diff/unified_test.go`: 68 tokens, lines 339-345 <- libs:github.com/charmbracelet/openai-go@v0.0.0-20260921175203-216db9e71b83/bedrock/auth_test.go lines 79-85
- `internal/ui/editor/editor.go`: 67 tokens, lines 556-572 <- libs:charm.land/bubbles/v2@v2.2.1/tree/tree.go lines 219-237
- `internal/agent/context_test.go`: 67 tokens, lines 72-78 <- libs:github.com/charmbracelet/x/ansi@v0.11.9/wcwidth_test.go lines 44-50
- `internal/agent/side_test.go`: 67 tokens, lines 237-252 <- libs:github.com/charmbracelet/openai-go@v0.0.0-20260921175203-216db9e71b83/internal/requestconfig/requestconfig_test.go lines 141-157

**abstract tokens: longest run, lists and tables included, against libs** (20 files above zero shown)

- `internal/diff/lines_test.go`: 434 tokens (11 distinct windows), lines 32-94 <- libs:charm.land/lipgloss/v2@v2.0.6/table/table_test.go lines 1209-1253
- `internal/config/table.go`: 263 tokens (11 distinct windows), lines 327-357 <- libs:charm.land/lipgloss/v2@v2.0.6/table/table_test.go lines 1283-1310
- `internal/permission/decideshell.go`: 259 tokens (15 distinct windows), lines 515-530 <- libs:github.com/charmbracelet/ultraviolet@v0.0.0-20261008173134-6b8d4baf91b4/key.go lines 608-676
- `internal/term/vtest/screen_test.go`: 186 tokens (36 distinct windows), lines 229-247 <- libs:github.com/charmbracelet/x/ansi@v0.11.9/wcwidth_test.go lines 18-37
- `internal/permission/names.go`: 180 tokens (9 distinct windows), lines 97-113 <- libs:github.com/charmbracelet/ultraviolet@v0.0.0-20261008173134-6b8d4baf91b4/key_table.go lines 152-178
- `internal/permission/protect.go`: 175 tokens (15 distinct windows), lines 19-38 <- libs:github.com/charmbracelet/ultraviolet@v0.0.0-20261008173134-6b8d4baf91b4/key.go lines 608-654
- `internal/permission/bash_test.go`: 162 tokens (10 distinct windows), lines 469-508 <- libs:github.com/charmbracelet/ultraviolet@v0.0.0-20261008173134-6b8d4baf91b4/key.go lines 611-652
- `internal/prompt/prompt.go`: 150 tokens (6 distinct windows), lines 34-87 <- libs:github.com/charmbracelet/x/ansi@v0.11.9/style.go lines 442-493
- `internal/tools/fstools/wild/wild_test.go`: 134 tokens (20 distinct windows), lines 524-532 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 14-43
- `internal/tools/shelltool/rules_test.go`: 132 tokens (18 distinct windows), lines 17-41 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 10-40
- `internal/term/vtest/tmux_test.go`: 124 tokens (10 distinct windows), lines 40-60 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 12-40
- `internal/cli/cli.go`: 122 tokens (10 distinct windows), lines 14-73 <- libs:github.com/charmbracelet/openai-go@v0.0.0-20260921175203-216db9e71b83/adminorganizationauditlog.go lines 341-375
- `internal/llm/errors_test.go`: 121 tokens (46 distinct windows), lines 272-277 <- libs:charm.land/fantasy@v0.45.1/retry_test.go lines 424-429
- `internal/permission/danger.go`: 121 tokens (13 distinct windows), lines 16-21 <- libs:github.com/charmbracelet/ultraviolet@v0.0.0-20261008173134-6b8d4baf91b4/key.go lines 611-639
- `internal/ui/keys.go`: 121 tokens (7 distinct windows), lines 715-735 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 13-40
- `internal/text/width_test.go`: 120 tokens (10 distinct windows), lines 122-140 <- libs:github.com/charmbracelet/x/ansi@v0.11.9/wcwidth_test.go lines 18-30
- `internal/config/load_test.go`: 118 tokens (10 distinct windows), lines 674-679 <- libs:github.com/charmbracelet/ultraviolet@v0.0.0-20261008173134-6b8d4baf91b4/key.go lines 611-639
- `internal/diff/unified_test.go`: 118 tokens (10 distinct windows), lines 250-256 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 12-39
- `internal/text/truncate_test.go`: 117 tokens (28 distinct windows), lines 11-27 <- libs:github.com/charmbracelet/x/ansi@v0.11.9/wrap_test.go lines 49-64
- `internal/config/hosts_test.go`: 115 tokens (7 distinct windows), lines 157-160 <- libs:charm.land/glamour/v2@v2.0.1/internal/autolink/autolink_test.go lines 13-39

**line share, against since** (13 files above zero shown)

- `internal/agent/side_test.go`: 0.8% <- now:internal/agent/coordinator_test.go@ee174bc5
- `internal/app/apptest/model.go`: 0.8% <- nowhist:internal/ui/chat/tools.go@24b904c2
- `internal/ui/editor/item.go`: 0.8% <- nowhist:internal/ui/chat/assistant.go@dc958c6c
- `internal/diff/spans_test.go`: 0.6% <- nowhist:internal/ui/dialog/commands.go@310fb1ee
- `internal/hooks/answer.go`: 0.6% <- nowhist:internal/ui/chat/tools.go@24b904c2
- `internal/ui/menu/menu.go`: 0.6% <- nowhist:internal/ui/chat/tools.go@24b904c2
- `internal/permission/bash.go`: 0.5% <- nowhist:internal/ui/dialog/commands.go@310fb1ee
- `internal/tools/fstools/edit.go`: 0.5% <- nowhist:internal/ui/common/timer_test.go@f1c90a6f
- `internal/app/log.go`: 0.4% <- nowhist:internal/ui/chat/tools.go@24b904c2
- `internal/app/skills.go`: 0.4% <- nowhist:internal/ui/chat/tools.go@24b904c2
- `internal/memjoin/harness_test.go`: 0.4% <- nowhist:internal/ui/model/ui.go@c48c6c0a
- `internal/app/print.go`: 0.3% <- now:internal/agent/coordinator_test.go@ee174bc5
- `internal/tools/fstools/grep_test.go`: 0.2% <- nowhist:internal/ui/model/ui.go@c48c6c0a

**longest line run of 4+ (counted or not), against since** (1 files above zero shown)

- `internal/app/texts.go`: 4

**longest run of lines each somewhere in the corpus, against since** (4 files above zero shown)

- `internal/app/apptest/model.go`: 7
- `internal/app/log.go`: 4
- `internal/hooks/answer.go`: 4
- `internal/ui/editor/item.go`: 4

**exact tokens: longest varied run, against since** (0 files above zero shown)


**exact tokens: longest run, lists and tables included, against since** (0 files above zero shown)


**abstract tokens: longest varied run, against since** (1 files above zero shown)

- `internal/tools/shelltool/text.go`: 61 tokens, lines 60-76 <- nowhist:internal/ui/chat/tools.go@24b904c2 lines 1938-1954

**abstract tokens: longest run, lists and tables included, against since** (7 files above zero shown)

- `internal/tools/shelltool/text.go`: 126 tokens (14 distinct windows), lines 60-95 <- nowhist:internal/ui/chat/tools.go@24b904c2 lines 1938-1973
- `internal/ui/editor/editor.go`: 88 tokens (8 distinct windows), lines 339-354 <- nowhist:internal/ui/styles/themes.go@57ca619d lines 56-76
- `internal/agent/turn.go`: 75 tokens (16 distinct windows), lines 666-678 <- nowhist:internal/agent/agent.go@1c0b6202 lines 328-340
- `internal/app/startup.go`: 74 tokens (8 distinct windows), lines 214-226 <- nowhist:internal/ui/styles/themes.go@57ca619d lines 56-71
- `internal/transcript/frames.go`: 68 tokens (7 distinct windows), lines 683-687 <- nowhist:internal/ui/styles/themes.go@57ca619d lines 108-121
- `internal/hooks/runner.go`: 62 tokens (3 distinct windows), lines 164-174 <- nowhist:internal/ui/styles/themes.go@57ca619d lines 96-109
- `internal/memjoin/session.go`: 62 tokens (3 distinct windows), lines 159-170 <- nowhist:internal/ui/styles/themes.go@57ca619d lines 109-121

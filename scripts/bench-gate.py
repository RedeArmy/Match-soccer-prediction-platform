#!/usr/bin/env python3
"""
Reads a benchstat comparison file and exits 1 if any benchmark shows a
statistically significant regression above THRESHOLD percent.

Usage:
  go run golang.org/x/perf/cmd/benchstat baseline.txt current.txt | python3 scripts/bench-gate.py
  python3 scripts/bench-gate.py delta.txt

benchstat marks non-significant changes with "~" — those are always treated as
noise and skipped. Only positive deltas (regressions) strictly above the
threshold trigger a failure. Negative deltas (improvements) are always allowed.

Threshold rationale: GitHub Actions ubuntu runners exhibit ~±10 % timing
variance between runs. A 20 % threshold sits safely above that noise floor
while catching real hot-path regressions (e.g. an extra allocation in ScoreMatch
that would cause the scoring worker to fall behind during match-end events).
"""
import sys
import re

THRESHOLD = 20  # percent — update here and in the ci.yml comment if changed

src = open(sys.argv[1]) if len(sys.argv) > 1 else sys.stdin
regressions = []

for line in src:
    # benchstat prefixes noise lines with "~"; skip them.
    if "~" in line:
        continue
    # geomean is a summary aggregate across benchmarks in a package; it is not
    # a named benchmark and can drift >threshold from scheduler noise even when
    # every individual benchmark is stable. Gate on named benchmarks only.
    if line.lstrip().startswith("geomean"):
        continue
    m = re.search(r"\+(\d+(?:\.\d+)?)%", line)
    if m and float(m.group(1)) > THRESHOLD:
        regressions.append(line.rstrip())

if regressions:
    print(
        f"Benchmark regressions >{THRESHOLD}% (p<0.05) detected:",
        file=sys.stderr,
    )
    for r in regressions:
        print(f"  {r}", file=sys.stderr)
    print(
        "Update .bench/baseline.txt with `make bench-save` if the change is intentional.",
        file=sys.stderr,
    )
    sys.exit(1)

sys.exit(0)

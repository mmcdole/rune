#!/usr/bin/env python3
"""Record, compare, and profile Rune's rendering benchmarks (Python stdlib only)."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import statistics
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_BENCH = "^(BenchmarkRender|BenchmarkOutputLatency)"


def capture(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True).strip()


def run(args, output):
    print("Running:", " ".join(args), flush=True)
    with output.open("w") as log:
        process = subprocess.Popen(args, cwd=ROOT, stdout=subprocess.PIPE,
                                   stderr=subprocess.STDOUT, text=True)
        for line in process.stdout:
            print(line, end="", flush=True)
            log.write(line)
        if process.wait():
            raise RuntimeError(f"command failed; see {output}")


def prepare(path, args):
    path.mkdir(parents=True, exist_ok=False)  # never overwrite a baseline
    sources = {}
    for name in capture("git", "ls-files", "--cached", "--others", "--exclude-standard").splitlines():
        source = ROOT / name
        if source.is_file() and (name.endswith(".go") or name in ("go.mod", "go.sum")):
            sources[name] = hashlib.sha256(source.read_bytes()).hexdigest()
    metadata = {
        "commit": capture("git", "rev-parse", "HEAD"),
        "status": capture("git", "status", "--short"),
        "go": capture("go", "version"),
        "go_env": json.loads(capture("go", "env", "-json", "GOOS", "GOARCH", "GOAMD64", "GOARM64", "CGO_ENABLED", "GOFLAGS")),
        "environment": {k: os.environ.get(k) for k in ("GOMAXPROCS", "GOGC", "GOMEMLIMIT", "TERM")},
        "command": vars(args),
        "source_sha256": sources,
    }
    (path / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
    (path / "changes.patch").write_text(capture("git", "diff", "HEAD") + "\n")


def measurements(path):
    results = {}
    package = ""
    for line in (path / "bench.txt").read_text().splitlines():
        if line.startswith("pkg: "):
            package = line[5:]
        fields = line.split()
        if not fields or not fields[0].startswith("Benchmark") or len(fields) < 4:
            continue
        name = re.sub(r"-\d+$", "", fields[0])
        for value, unit in zip(fields[2::2], fields[3::2]):
            if unit in ("ns/op", "B/op", "allocs/op", "p50-ns", "p95-ns", "p99-ns", "terminal-B/op"):
                results.setdefault((package, name, unit), []).append(float(value))
    if not results:
        raise RuntimeError(f"no benchmark measurements in {path}")
    return results


def compare(before, after):
    left, right = measurements(before), measurements(after)
    print("Descriptive medians; negative delta is better. Ranges show run-to-run noise, not confidence intervals.")
    print("For latency tails, use sufficiently long runs; inspect the 'samples' metric in bench.txt.")
    for key in sorted(left.keys() | right.keys()):
        package, name, unit = key
        print(f"\n{package}/{name} [{unit}]")
        if key not in left or key not in right:
            print("  MISSING from", "before" if key not in left else "after")
            continue
        a, b = left[key], right[key]
        old, new = statistics.median(a), statistics.median(b)
        delta = f"{(new / old - 1) * 100:+.1f}%" if old else ("unchanged" if not new else "from zero")
        print(f"  {old:.3g} -> {new:.3g} ({delta}); n={len(a)}/{len(b)}; ranges {min(a):.3g}..{max(a):.3g} / {min(b):.3g}..{max(b):.3g}")
    if left.keys() != right.keys():
        raise RuntimeError("workload sets differ; missing results are not a successful comparison")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="mode", required=True)
    record = commands.add_parser("record", help="run correctness checks and save repeated benchmarks")
    record.add_argument("directory")
    record.add_argument("--count", type=int, default=5)
    record.add_argument("--time", default="1s", help="Go benchtime, e.g. 1s or 100x")
    record.add_argument("--bench", default=DEFAULT_BENCH)
    record.add_argument("--skip-tests", action="store_true", help="use only after this revision's correctness checks passed")
    diff = commands.add_parser("compare", help="compare medians and ranges without extra dependencies")
    diff.add_argument("before")
    diff.add_argument("after")
    profile = commands.add_parser("profile", help="profile one exact benchmark independently of timing runs")
    profile.add_argument("directory")
    profile.add_argument("--package", default="./ui/tui")
    profile.add_argument("--bench", default="^BenchmarkRenderFlood$")
    profile.add_argument("--time", default="5s")
    args = parser.parse_args()
    if args.mode == "compare":
        compare(Path(args.before).resolve(), Path(args.after).resolve())
        return
    if args.mode == "record" and args.count < 1:
        parser.error("--count must be positive")
    path = Path(args.directory).resolve()
    prepare(path, args)
    if args.mode == "record":
        if not args.skip_tests:
            run(["go", "test", "-race", "-shuffle=on", "./ui/..."], path / "tests.txt")
        run(["go", "test", "./ui/tui", "./ui/tui/widget", "-run", "^$", "-bench", args.bench,
             "-benchmem", f"-benchtime={args.time}", f"-count={args.count}"], path / "bench.txt")
        measurements(path)  # an accidental empty selector must fail
    else:
        run(["go", "test", args.package, "-run", "^$", "-bench", args.bench,
             "-benchmem", f"-benchtime={args.time}", f"-cpuprofile={path / 'cpu.pprof'}",
             f"-memprofile={path / 'heap.pprof'}", "-o", str(path / "bench.test")], path / "bench.txt")
        measurements(path)
        for profile_name, flags in (("cpu", []), ("heap", ["-alloc_space"])):
            run(["go", "tool", "pprof", "-top", "-nodecount=20", *flags, str(path / "bench.test"),
                 str(path / f"{profile_name}.pprof")], path / f"{profile_name}.txt")
    print(f"Saved {path}")


if __name__ == "__main__":
    try:
        main()
    except (OSError, RuntimeError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))

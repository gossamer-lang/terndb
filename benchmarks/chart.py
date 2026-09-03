# /// script
# requires-python = ">=3.11"
# dependencies = ["matplotlib>=3.8"]
# ///
"""Charts the JSON the benchmark runners write.

    uv run chart.py [results.json] [--embedded embedded.json] [--out charts]

Produces five charts and an index.html that puts them beside the numbers they
came from. Over HTTP a target is described by three numbers - response rate,
CPU per request and peak memory - and with no HTTP by two: queries per second
and peak memory. Nothing else is charted or tabulated, because nothing else
decides between two stores.
"""

from __future__ import annotations

import argparse
import json
import pathlib
import sys

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402
import matplotlib.ticker  # noqa: E402

# Three HTTP charts per deployment - response rate, CPU per request, peak
# memory. Every bar in a chart is doing the same work against the same seeded
# dataset, so the compiled and `gos run` tiers stand beside each other rather
# than in charts of their own. A target the run did not produce is left out, so
# ONLY=... runs still chart.
# A target is named for what it is: language, tool, deployment, query. The id
# spells the four in that order - `gos-terndb-embedded-kv` - so a label is read
# out of the id rather than kept in a table beside it, and a target added to a
# run cannot drift from what it is called here.
WEB_EMBEDDED_DB = [
    "gos-terndb-embedded-sql",
    "gos-terndb-embedded-kv",
    "rust-redb-embedded-kv",
    "rust-sqlite-embedded-sql",
    "go-sqlite-embedded-sql",
    "python-sqlite-embedded-sql",
    "gos-terndb-embedded-sql-gos-run",
    "gos-terndb-embedded-kv-gos-run",
]
# The query-only chart, in the order embedded.sh measures.
QUERY_ONLY = [
    "gos-terndb-embedded-sql",
    "gos-terndb-embedded-kv",
    "rust-sqlite-embedded-sql",
    "go-sqlite-embedded-sql",
    "rust-redb-embedded-kv",
    "rust-redb-embedded-kv-typed",
]

WEB_EMBEDDED_DB_TITLE = "Web server, embedded DB: the store in the web server's own process"
QUERY_ONLY_TITLE = "Query only: one thread querying the store it is linked against, no HTTP"

CHARTS = [
    ("web-embedded-db-response-rate", WEB_EMBEDDED_DB_TITLE, WEB_EMBEDDED_DB,
     "response rate, HTTP requests / second (higher is better)", "rps", "{:,.0f}"),
    ("web-embedded-db-cpu", WEB_EMBEDDED_DB_TITLE, WEB_EMBEDDED_DB,
     "CPU, microseconds / request (lower is better)", "cpu_us_per_request", "{:,.1f}"),
    ("web-embedded-db-memory", WEB_EMBEDDED_DB_TITLE, WEB_EMBEDDED_DB,
     "memory, peak resident MiB under load (lower is better)", "rss_peak_mb", "{:,.0f}"),
]
QUERY_ONLY_CHARTS = [
    ("query-only-queries", QUERY_ONLY_TITLE, QUERY_ONLY,
     "queries / second (higher is better)", "qps", "{:,.0f}"),
    ("query-only-memory", QUERY_ONLY_TITLE, QUERY_ONLY,
     "memory, peak resident MiB (lower is better)", "rss_peak_mb", "{:,.0f}"),
]

# How each part of an id is spelled once it is read by a person.
LANGUAGES = {"gos": "Gos", "rust": "Rust", "go": "Go", "python": "Python"}
TOOLS = {"terndb": "terndb", "redb": "redb", "sqlite": "SQLite"}
DEPLOYMENTS = {"embedded": "Embedded"}
QUERIES = {"kv": "KV", "sql": "SQL"}
# A trailing part says a target is another target's read path under a different
# condition - `gos run` for the interpreted tier, `typed` for redb holding typed
# columns - and it is displayed as the words the id spells.


def parts(target: str):
    """The language, tool, deployment, query and variant an id spells."""
    bits = target.split("-")
    if len(bits) < 4:
        return None
    lang, tool, deployment, query = bits[:4]
    return lang, tool, deployment, query, bits[4:]


def stack(target: str) -> str:
    """The language and tool, with what makes this target a variant of it."""
    p = parts(target)
    if p is None:
        return target
    lang, tool, _, _, variants = p
    name = f"{LANGUAGES.get(lang, lang)} {TOOLS.get(tool, tool)}"
    extra = " ".join(variants)
    return f"{name} ({extra})" if extra else name


def deployment_of(target: str) -> str:
    p = parts(target)
    return DEPLOYMENTS.get(p[2], p[2]) if p else "Embedded"


def query_of(target: str) -> str:
    p = parts(target)
    return QUERIES.get(p[3], p[3].upper()) if p else ""


def label(target: str) -> str:
    """`Gos terndb` over `Embedded: KV`, so a chart lifted out of the page still
    says which stack, which deployment and which read path it measured."""
    return f"{stack(target)}\n{deployment_of(target)}: {query_of(target)}"


def one_line(target: str) -> str:
    return f"{stack(target)} {deployment_of(target)}: {query_of(target)}"


# One colour per read path, so a path keeps its colour across every chart. The
# hues are the reference categorical order, validated for the pair separation a
# reader needs; a variant repeats its path's hue under a hatch rather than
# taking a hue of its own, because it is the same path under another condition.
PATH_COLOURS = {
    ("gos", "terndb", "sql"): "#2a78d6",
    ("gos", "terndb", "kv"): "#eb6834",
    ("rust", "sqlite", "sql"): "#1baf7a",
    ("rust", "redb", "kv"): "#4a3aa7",
    ("go", "sqlite", "sql"): "#e34948",
    ("python", "sqlite", "sql"): "#eda100",
}
FALLBACK = "#999999"


def colour(target: str) -> str:
    p = parts(target)
    return PATH_COLOURS.get((p[0], p[1], p[3]), FALLBACK) if p else FALLBACK


def hatched(target: str) -> bool:
    """A bar that repeats another bar's read path: the same store on the
    `gos run` tier, or asked the question the SQL stores are asked."""
    p = parts(target)
    return bool(p and p[4])


def bar_chart(runs, order, title, ylabel, key, fmt, path):
    picked = [r for name in order for r in runs if r["target"] == name]
    picked = [r for r in picked if r.get(key)]
    if not picked:
        return False
    names = [label(r["target"]) for r in picked]
    values = [r[key] for r in picked]
    fig, ax = plt.subplots(figsize=(max(6, 1.6 * len(names)), 4.6), dpi=140)
    bars = ax.bar(names, values, color=[colour(r["target"]) for r in picked],
                  width=0.62, edgecolor="#fcfcfb", linewidth=2)
    # The interpreted tier shares its path's hue, so the texture is what tells
    # the two apart without asking a reader to separate two shades of one colour.
    for bar, r in zip(bars, picked):
        if hatched(r["target"]):
            bar.set_hatch("///")
    ax.set_title(title, fontsize=11, pad=12)
    ax.set_ylabel(ylabel, fontsize=9)
    ax.tick_params(axis="x", labelsize=8.5)
    ax.tick_params(axis="y", labelsize=8)
    ax.spines[["top", "right"]].set_visible(False)
    ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(lambda v, _: f"{v:,.0f}"))
    ax.grid(axis="y", alpha=0.25, linewidth=0.6)
    ax.set_axisbelow(True)
    top = max(values)
    # Every bar is labelled with its own value: identity and magnitude are both
    # readable without resolving one bar's colour against its neighbour's.
    for bar, v in zip(bars, values):
        ax.text(bar.get_x() + bar.get_width() / 2, v + top * 0.02, fmt.format(v),
                ha="center", va="bottom", fontsize=8, color="#52514e")
    ax.set_ylim(0, top * 1.18)
    fig.tight_layout()
    fig.savefig(path)
    plt.close(fig)
    return True


def split_runs(data):
    """The runs that produced a measurement, and the ones that failed."""
    runs = data.get("runs", [])
    return [r for r in runs if not r.get("failed")], [r for r in runs if r.get("failed")]


def http_table(runs, failed):
    rows = "\n".join(
        "<tr><td>{label}</td><td>{rps:,.0f}</td><td>{cpu}</td><td>{rss}</td></tr>".format(
            label=one_line(r["target"]) + f" ({r['target']})",
            cpu=f"{r['cpu_us_per_request']:,.1f}" if r.get("cpu_us_per_request") else "-",
            rss=f"{r['rss_peak_mb']:,.1f}" if r.get("rss_peak_mb") else "-",
            **r
        )
        for r in runs
    )
    rows += "\n" + "\n".join(
        '<tr><td>{}</td><td colspan="3">failed: {}</td></tr>'.format(
            one_line(r["target"]) + f" ({r['target']})",
            r.get("reason", "no measurement"),
        )
        for r in failed
    )
    return f"""<table>
  <caption>Every HTTP target, sorted by response rate. The store runs in the
  web server's own process, so the memory is that one process.</caption>
  <tr><th>target</th><th>response rate, req/s</th><th>CPU, us/req</th><th>memory, peak MiB</th></tr>
  {rows}
</table>"""


def in_process_table(runs, failed):
    rows = "\n".join(
        "<tr><td>{label}</td><td>{qps:,.0f}</td><td>{rss}</td></tr>".format(
            label=one_line(r["target"]) + f" ({r['target']})",
            rss=f"{r['rss_peak_mb']:,.1f}" if r.get("rss_peak_mb") else "-",
            **r
        )
        for r in runs
    )
    rows += "\n" + "\n".join(
        '<tr><td>{}</td><td colspan="2">failed: {}</td></tr>'.format(
            one_line(r["target"]) + f" ({r['target']})",
            r.get("reason", "no measurement"),
        )
        for r in failed
    )
    return f"""<table>
  <caption>Every query-only target, sorted by queries per second. One thread, no
  socket and no HTTP: the store is linked into the caller, asked for a row by
  primary key, and the row is rendered as the same JSON every HTTP target
  returns. The memory is the whole process, which is what the store costs to
  run.</caption>
  <tr><th>target</th><th>queries/s</th><th>memory, peak MiB</th></tr>
  {rows}
</table>"""


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("results", nargs="?", default="results.json")
    ap.add_argument("--embedded", default="embedded.json",
                    help="the query-only run embedded.sh writes; skipped when absent")
    ap.add_argument("--out", default="charts")
    args = ap.parse_args()

    src = pathlib.Path(args.results)
    if not src.exists():
        print(f"{src} not found - run ./run.sh first", file=sys.stderr)
        return 1
    data = json.loads(src.read_text())
    if not data.get("runs"):
        print("no runs in the results", file=sys.stderr)
        return 1
    # A target that died mid-run produced no measurement; it belongs in the
    # table as a failure, not in a bar chart as a slow result.
    runs, failed = split_runs(data)
    runs.sort(key=lambda r: -r["rps"])
    if not runs:
        print("every HTTP target failed; nothing to chart", file=sys.stderr)
        return 1

    ip_src = pathlib.Path(args.embedded)
    ip_data = json.loads(ip_src.read_text()) if ip_src.exists() else {}
    ip_runs, ip_failed = split_runs(ip_data)
    ip_runs.sort(key=lambda r: -r["qps"])

    out = pathlib.Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    made = []
    for key, title, order, ylabel, field, fmt in CHARTS:
        name = f"{key}.png"
        if bar_chart(runs, order, title, ylabel, field, fmt, out / name):
            made.append((f"{title} - {ylabel.split(' (')[0]}", name))
    for key, title, order, ylabel, field, fmt in QUERY_ONLY_CHARTS:
        name = f"{key}.png"
        if ip_runs and bar_chart(ip_runs, order, title, ylabel, field, fmt, out / name):
            made.append((f"{title} - {ylabel.split(' (')[0]}", name))

    cfg = data.get("config", {})
    host = data.get("host", {})
    ip_cfg = ip_data.get("config", {})
    note = (
        "The HTTP charts report the three numbers a deployment is chosen on - "
        "response rate, CPU per request and peak memory. A Go client drives "
        "concurrent GETs at a web server, and the store runs in that server's "
        "own process. The query-only charts "
        "have no HTTP in them at all - one thread calling the store it is linked "
        "against - so they report queries per second and the memory the store "
        "costs to run. Every bar in a chart answered the same request against the "
        "same seeded dataset. A variant - the `gos run` tier, or redb holding "
        "typed columns - is hatched and shares its path's colour."
    )
    ip_note = (
        f"{ip_cfg.get('rows', '?'):,} rows, one thread, "
        f"{ip_cfg.get('seconds', '?')}s measured per target."
    ) if ip_runs else ""
    images = "\n".join(
        f'<figure><img src="{f}" alt="{t}"><figcaption>{t}</figcaption></figure>' for t, f in made
    )
    tables = http_table(runs, failed)
    if ip_runs or ip_failed:
        tables += "\n" + in_process_table(ip_runs, ip_failed)
    (out / "index.html").write_text(f"""<!doctype html>
<meta charset="utf-8"><title>terndb benchmarks</title>
<style>
  body {{ font: 14px/1.5 system-ui, sans-serif; margin: 2rem auto; max-width: 60rem; color: #0b0b0b; }}
  h1 {{ font-size: 1.4rem; }}
  figure {{ margin: 2rem 0; }} img {{ max-width: 100%; }}
  figcaption {{ font-size: .85rem; color: #52514e; margin-top: .4rem; }}
  table {{ border-collapse: collapse; width: 100%; font-variant-numeric: tabular-nums;
           margin: 2rem 0; }}
  th, td {{ padding: .4rem .6rem; border-bottom: 1px solid #e5e5e5; text-align: right; }}
  th:first-child, td:first-child {{ text-align: left; }}
  caption {{ text-align: left; font-size: .85rem; color: #52514e; padding-bottom: .6rem; }}
</style>
<h1>terndb benchmarks</h1>
<p>Over HTTP: {cfg.get('rows', '?'):,} rows, {cfg.get('workers', '?')} concurrent workers,
{cfg.get('seconds', '?')}s measured per target. In process: {ip_note or 'not run'}
On {host.get('cores', '?')} cores ({host.get('kernel', '')}).</p>
<p>{note}</p>
{images}
{tables}
""")
    print(f"wrote {out}/index.html and {len(made)} chart(s)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

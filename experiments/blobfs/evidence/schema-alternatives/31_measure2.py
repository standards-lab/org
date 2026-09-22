#!/usr/bin/env python3
"""Addendum measurements: the tuned index set for design B, and what a cursor
costs an interleaved listing in each design at page two rather than the last
page (the last page flatters design D, because its predicate leaves 21 rows
for the sort to consume)."""
import sys
sys.path.insert(0, __import__("os").path.dirname(__file__))
from importlib import import_module
m = import_module("30_measure")

BIG = m.BIG
ROOT = m.ROOT
PAGE = m.PAGE
psql, explain, shape = m.psql, m.explain, m.shape

uid = lambda v: f"CAST('{v}' AS uuid)"
txt = lambda v: f"CAST('{v}' AS text)"
paging = m.paging
total = ", COUNT(*) OVER () AS total"

# the name of the 20th entry of the biggest directory in the merged order,
# so "page two by cursor" starts right after page one.
c2name = psql("blobfs_opus_a",
              f"SELECT name FROM (SELECT name FROM blobfs_directory WHERE parent_id='{BIG}' "
              f"UNION ALL SELECT name FROM blobfs_file WHERE directory_id='{BIG}') u "
              f"ORDER BY name OFFSET {PAGE - 1} LIMIT 1").strip()
cfile2 = psql("blobfs_opus_a",
              f"SELECT name FROM blobfs_file WHERE directory_id='{BIG}' ORDER BY name OFFSET {PAGE - 1} LIMIT 1").strip()

dcols = ("CAST('directory' AS text) AS kind, d.id, d.parent_id, d.name, "
         "CAST(NULL AS text) AS status, CAST(NULL AS bigint) AS size, d.version, d.created_at, d.updated_at")
fcols = ("CAST('file' AS text) AS kind, f.id, f.directory_id, f.name, "
         "f.status, f.size, f.version, f.created_at, f.updated_at")

cases = [
    ("a", "D12b mixed page 2 by cursor, biggest (UNION ALL, predicate in both branches)",
     f"SELECT {dcols} FROM blobfs_directory d WHERE d.parent_id = {uid(BIG)} AND d.name > {txt(c2name)}\n"
     f"UNION ALL\nSELECT {fcols} FROM blobfs_file f WHERE f.directory_id = {uid(BIG)} AND f.name > {txt(c2name)}\n"
     f"ORDER BY name" + paging(0, PAGE + 1)),
    ("a", "L05b files page 2 by cursor, biggest",
     "SELECT " + m.FILE_COLS_A + f"\nFROM blobfs_file q\nWHERE q.directory_id = {uid(BIG)}"
     f" AND q.name > {txt(cfile2)} ORDER BY q.name" + paging(0, PAGE + 1)),
    ("b", "L12b mixed page 2 by cursor, biggest",
     "SELECT " + m.ENTRY_COLS_B + f"\nFROM blobfs_entry q\nWHERE q.parent_id = {uid(BIG)}"
     f" AND q.name > {txt(c2name)} ORDER BY q.name" + paging(0, PAGE + 1)),
    ("b", "L05b files page 2 by cursor, biggest",
     "SELECT " + m.FILE_COLS_B + f"\nFROM blobfs_entry q\nWHERE q.parent_id = {uid(BIG)} AND q.kind = {txt('file')}"
     f" AND q.name > {txt(cfile2)} ORDER BY q.name" + paging(0, PAGE + 1)),
    ("c", "L12b mixed page 2 by cursor, biggest",
     "SELECT " + m.ENTRY_COLS_C + "\nFROM blobfs_node n LEFT JOIN blobfs_file f ON f.node_id = n.id\n"
     f"WHERE n.parent_id = {uid(BIG)} AND n.name > {txt(c2name)} ORDER BY n.name" + paging(0, PAGE + 1)),
    # the tuned index set for B: the per-kind index is partial over directories
    ("b2", "T03 files p1 name, no total, biggest (tuned indexes)",
     "SELECT " + m.FILE_COLS_B + f"\nFROM blobfs_entry q\nWHERE q.parent_id = {uid(BIG)} AND q.kind = {txt('file')}"
     " ORDER BY q.name" + paging(0, PAGE + 1)),
    ("b2", "T02 files p1 name, exact total, biggest (tuned indexes)",
     "SELECT " + m.FILE_COLS_B + total + f"\nFROM blobfs_entry q\nWHERE q.parent_id = {uid(BIG)} AND q.kind = {txt('file')}"
     " ORDER BY q.name" + paging(0, PAGE + 1)),
    ("b2", "T05 files last page by cursor, biggest (tuned indexes)",
     "SELECT " + m.FILE_COLS_B + f"\nFROM blobfs_entry q\nWHERE q.parent_id = {uid(BIG)} AND q.kind = {txt('file')}"
     f" AND q.name > {txt('f99910.txt')} ORDER BY q.name" + paging(0, PAGE + 1)),
    ("b2", "T09 children p1 name, exact total, biggest (tuned indexes)",
     "SELECT " + m.DIR_COLS_A + total + f"\nFROM blobfs_entry q\nWHERE q.parent_id = {uid(BIG)} AND q.kind = {txt('directory')}"
     " ORDER BY q.name" + paging(0, PAGE + 1)),
    ("b2", "T13 mixed p1 name, no total, biggest (tuned indexes)",
     "SELECT " + m.ENTRY_COLS_B + f"\nFROM blobfs_entry q\nWHERE q.parent_id = {uid(BIG)}"
     " ORDER BY q.name" + paging(0, PAGE + 1)),
    ("b2", "T11 mixed p1 name, exact total, biggest (tuned indexes)",
     "SELECT " + m.ENTRY_COLS_B + total + f"\nFROM blobfs_entry q\nWHERE q.parent_id = {uid(BIG)}"
     " ORDER BY q.name" + paging(0, PAGE + 1)),
    ("b2", "T06 files p1 created_at asc, no total, biggest (tuned indexes)",
     "SELECT " + m.FILE_COLS_B + f"\nFROM blobfs_entry q\nWHERE q.parent_id = {uid(BIG)} AND q.kind = {txt('file')}"
     " ORDER BY q.created_at, q.name" + paging(0, PAGE + 1)),
    ("b2", "T15 directory_child, one path segment (tuned indexes)",
     f"SELECT q.id, q.parent_id, q.name, q.version, q.created_at, q.updated_at\nFROM blobfs_entry q\n"
     f"WHERE q.parent_id = {uid('02d00001-7000-8000-9000-000000000001')} AND q.name = {txt('d3')} AND q.kind = {txt('directory')}"),
]

out, rows, bodies = [], [], []
out.append("# Addendum: page-two cursors on an interleaved listing, and design B's tuned index set.")
out.append(f"# the merged page-one boundary name is {c2name!r}; the file page-one boundary is {cfile2!r}.")
out.append("")
for design, label, sql in cases:
    db = "blobfs_opus_" + design
    ex, pl, buf, plan = explain(db, sql)
    rows.append((design, label, pl, ex, buf, shape(plan)))
    bodies.append(f"\n---- {design}: {label}: median of {m.RUNS} runs: planning {pl:.3f} ms, "
                  f"execution {ex:.3f} ms, buffers {buf} ----\n{sql}\n\n{plan}")
    print(f"{design:3s} {label:70s} {pl:8.3f} {ex:9.3f} {buf:8d}", file=sys.stderr)
out.append("# summary (median run; times in ms; buffers as hit+read pages):")
out.append(f"# {'des':<4}{'query':<72}{'plan ms':>9}{'exec ms':>10}{'buffers':>9}  plan shape")
for d, label, pl, ex, buf, sh in rows:
    out.append(f"# {d:<4}{label:<72}{pl:>9.3f}{ex:>10.3f}{buf:>9}  {sh}")
out.extend(bodies)
print("\n".join(out))

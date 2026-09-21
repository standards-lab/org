#!/usr/bin/env python3
"""Runs the query set of every candidate design and writes one transcript.

Method follows lib/blobfs/data/evidence_integration_test.go: each query is
EXPLAIN (ANALYZE, BUFFERS) once to warm the cache and then five times, and the
run with the median execution time is reported with its plan. Buffers are the
top plan node's shared hit+read, in 8 KB pages. Every value is a literal in
the text, so each run is planned with its own values, as the shipped
measurement's simple-protocol runs are.
"""
import re
import subprocess
import statistics
import sys

BIG = '02d00003-7000-8000-9000-000000000003'   # depth 2, 10,009 files, 6 subdirectories
SMALL = '02d013ca-7000-8000-9000-0000000013ca'  # depth 6, 9 files, 0 subdirectories
ROOT = '00000000-0000-0000-0000-000000000000'
DEEP = '02d013ca-7000-8000-9000-0000000013ca'   # the depth-6 directory the walks start at
TOP = '02d00001-7000-8000-9000-000000000001'    # t1, a depth-1 directory
UNIT = '01b00000-7000-8000-9000-000000000000'   # unit 0, 250 bookmarks
PAGE = 20
RUNS = 5

PLAN_NODES = [
    "Recursive Union", "WorkTable Scan", "CTE Scan", "Subquery Scan", "Append",
    "Merge Append", "Seq Scan", "Sort", "Incremental Sort", "Aggregate",
    "WindowAgg", "Limit", "Nested Loop", "Hash Join", "Merge Join",
    "Materialize", "Memoize", "Heap Fetches: 0",
]

planning_re = re.compile(r"Planning Time: ([0-9.]+) ms")
execution_re = re.compile(r"Execution Time: ([0-9.]+) ms")
buffers_re = re.compile(r"Buffers: shared( hit=(\d+))?( read=(\d+))?")


def psql(db, sql):
    p = subprocess.run(
        ["docker", "exec", "-i", "blobfs-postgres", "psql", "-U", "app", "-d", db,
         "-v", "ON_ERROR_STOP=1", "-tAq", "-c", sql],
        capture_output=True, text=True)
    if p.returncode != 0:
        raise RuntimeError(f"{db}: {p.stderr.strip()}\n{sql}")
    return p.stdout


def explain(db, sql):
    runs = []
    for i in range(RUNS + 1):
        out = psql(db, "EXPLAIN (ANALYZE, BUFFERS) " + sql)
        if i == 0:
            continue
        plan = out.strip()
        pl = float(planning_re.search(plan).group(1))
        ex = float(execution_re.search(plan).group(1))
        m = buffers_re.search(plan)
        buf = 0
        if m:
            buf = int(m.group(2) or 0) + int(m.group(4) or 0)
        runs.append((ex, pl, buf, plan))
    runs.sort(key=lambda r: r[0])
    return runs[len(runs) // 2]


def shape(plan):
    seen = []
    for n in PLAN_NODES:
        if n in plan:
            seen.append(n)
    idx = re.findall(r"(?:Index Only Scan|Index Scan Backward|Index Scan) using (\w+)", plan)
    for i in idx:
        if i not in seen:
            seen.append("idx:" + i)
    return ", ".join(seen)


# --------------------------------------------------------------------------
# The listings, one spelling per design. The file columns the library's
# blobfs.File scans, in the library's order, plus the window count where the
# listing carries its total.

FILE_COLS_A = ("q.id, q.directory_id, q.name, q.status, q.key, q.size, "
               "q.content_type, q.etag, q.version, q.created_at, q.updated_at")
FILE_COLS_B = ("q.id, q.parent_id, q.name, q.status, q.key, q.size, "
               "q.content_type, q.etag, q.version, q.created_at, q.updated_at")
FILE_COLS_C = ("n.id, n.parent_id, n.name, f.status, f.key, f.size, "
               "f.content_type, f.etag, n.version, n.created_at, n.updated_at")
DIR_COLS_A = "q.id, q.parent_id, q.name, q.version, q.created_at, q.updated_at"
ENTRY_COLS_B = ("q.id, q.kind, q.parent_id, q.name, q.status, q.key, q.size, "
                "q.content_type, q.etag, q.version, q.created_at, q.updated_at")
ENTRY_COLS_C = ("n.id, n.kind, n.parent_id, n.name, f.status, f.key, f.size, "
                "f.content_type, f.etag, n.version, n.created_at, n.updated_at")


def paging(offset, fetch):
    return f" OFFSET {offset} ROWS FETCH NEXT {fetch} ROWS ONLY"


def build(design, cursor_file, cursor_mixed, last_offset_files, last_offset_mixed):
    """Returns [(label, sql)] for one design."""
    q = []
    add = lambda label, sql: q.append((label, sql))
    uid = lambda v: f"CAST('{v}' AS uuid)"
    txt = lambda v: f"CAST('{v}' AS text)"
    total = ", COUNT(*) OVER () AS total"

    if design == "a":
        fbase = lambda d, cols: f"SELECT {cols}\nFROM blobfs_file q\nWHERE q.directory_id = {uid(d)}"
        dbase = lambda d, cols: f"SELECT {cols}\nFROM blobfs_directory q\nWHERE q.parent_id = {uid(d)}"
        add("L01 files p1 name, exact total, small",
            fbase(SMALL, FILE_COLS_A + total) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L02 files p1 name, exact total, biggest",
            fbase(BIG, FILE_COLS_A + total) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L03 files p1 name, no total, biggest",
            fbase(BIG, FILE_COLS_A) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L04 files last page by offset, no total, biggest",
            fbase(BIG, FILE_COLS_A) + " ORDER BY q.name" + paging(last_offset_files, PAGE + 1))
        add("L05 files last page by cursor, biggest",
            fbase(BIG, FILE_COLS_A) + f" AND q.name > {txt(cursor_file)} ORDER BY q.name" + paging(0, PAGE + 1))
        add("L06 files p1 created_at asc, no total, biggest",
            fbase(BIG, FILE_COLS_A) + " ORDER BY q.created_at, q.name" + paging(0, PAGE + 1))
        add("L07 files p1 size desc, no total, biggest",
            fbase(BIG, FILE_COLS_A) + " ORDER BY q.size DESC, q.name" + paging(0, PAGE + 1))
        add("L08 files status=pending, exact total, biggest",
            fbase(BIG, FILE_COLS_A + total) + f" AND q.status = {txt('pending')} ORDER BY q.name" + paging(0, PAGE + 1))
        add("L09 children p1 name, exact total, biggest",
            dbase(BIG, DIR_COLS_A + total) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L10 children p1 name, exact total, root",
            dbase(ROOT, DIR_COLS_A + total) + " ORDER BY q.name" + paging(0, PAGE + 1))

    if design == "b":
        fbase = lambda d, cols: (f"SELECT {cols}\nFROM blobfs_entry q\n"
                                 f"WHERE q.parent_id = {uid(d)} AND q.kind = {txt('file')}")
        dbase = lambda d, cols: (f"SELECT {cols}\nFROM blobfs_entry q\n"
                                 f"WHERE q.parent_id = {uid(d)} AND q.kind = {txt('directory')}")
        mbase = lambda d, cols: f"SELECT {cols}\nFROM blobfs_entry q\nWHERE q.parent_id = {uid(d)}"
        add("L01 files p1 name, exact total, small",
            fbase(SMALL, FILE_COLS_B + total) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L02 files p1 name, exact total, biggest",
            fbase(BIG, FILE_COLS_B + total) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L03 files p1 name, no total, biggest",
            fbase(BIG, FILE_COLS_B) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L04 files last page by offset, no total, biggest",
            fbase(BIG, FILE_COLS_B) + " ORDER BY q.name" + paging(last_offset_files, PAGE + 1))
        add("L05 files last page by cursor, biggest",
            fbase(BIG, FILE_COLS_B) + f" AND q.name > {txt(cursor_file)} ORDER BY q.name" + paging(0, PAGE + 1))
        add("L06 files p1 created_at asc, no total, biggest",
            fbase(BIG, FILE_COLS_B) + " ORDER BY q.created_at, q.name" + paging(0, PAGE + 1))
        add("L07 files p1 size desc, no total, biggest",
            fbase(BIG, FILE_COLS_B) + " ORDER BY q.size DESC, q.name" + paging(0, PAGE + 1))
        add("L08 files status=pending, exact total, biggest",
            fbase(BIG, FILE_COLS_B + total) + f" AND q.status = {txt('pending')} ORDER BY q.name" + paging(0, PAGE + 1))
        add("L09 children p1 name, exact total, biggest",
            dbase(BIG, DIR_COLS_A + total) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L10 children p1 name, exact total, root",
            dbase(ROOT, DIR_COLS_A + total) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L11 mixed p1 name, exact total, biggest",
            mbase(BIG, ENTRY_COLS_B + total) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L12 mixed last page by cursor, biggest",
            mbase(BIG, ENTRY_COLS_B) + f" AND q.name > {txt(cursor_mixed)} ORDER BY q.name" + paging(0, PAGE + 1))
        add("L13 mixed p1 name, no total, biggest",
            mbase(BIG, ENTRY_COLS_B) + " ORDER BY q.name" + paging(0, PAGE + 1))

    if design in ("c", "c2"):
        # flat: the node is the correlation name q, the file columns come from f.
        fflat = lambda d, cols: (f"SELECT {cols}\nFROM blobfs_node n JOIN blobfs_file f ON f.node_id = n.id\n"
                                 f"WHERE n.parent_id = {uid(d)} AND n.kind = {txt('file')}")
        # wrapped: the derived table the listing composer needs, because the
        # clause patterns qualify every field as q.<field> and a join has no
        # single correlation name.
        fwrap = lambda d, cols, extra="": (
            "SELECT * FROM (" + fflat(d, cols) + ") q" + extra)
        dbase = lambda d, cols: (f"SELECT {cols}\nFROM blobfs_node q\n"
                                 f"WHERE q.parent_id = {uid(d)} AND q.kind = {txt('directory')}")
        mflat = lambda d, cols: (f"SELECT {cols}\nFROM blobfs_node n LEFT JOIN blobfs_file f ON f.node_id = n.id\n"
                                 f"WHERE n.parent_id = {uid(d)}")
        add("L01 files p1 name, exact total, small",
            fflat(SMALL, FILE_COLS_C + total) + " ORDER BY n.name" + paging(0, PAGE + 1))
        add("L02 files p1 name, exact total, biggest",
            fflat(BIG, FILE_COLS_C + total) + " ORDER BY n.name" + paging(0, PAGE + 1))
        add("L03 files p1 name, no total, biggest",
            fflat(BIG, FILE_COLS_C) + " ORDER BY n.name" + paging(0, PAGE + 1))
        add("L04 files last page by offset, no total, biggest",
            fflat(BIG, FILE_COLS_C) + " ORDER BY n.name" + paging(last_offset_files, PAGE + 1))
        add("L05 files last page by cursor, biggest",
            fflat(BIG, FILE_COLS_C) + f" AND n.name > {txt(cursor_file)} ORDER BY n.name" + paging(0, PAGE + 1))
        add("L06 files p1 created_at asc, no total, biggest",
            fflat(BIG, FILE_COLS_C) + " ORDER BY n.created_at, n.name" + paging(0, PAGE + 1))
        add("L07 files p1 size desc, no total, biggest",
            fwrap(BIG, FILE_COLS_C) + " ORDER BY q.size DESC, q.name" + paging(0, PAGE + 1))
        add("L08 files status=pending, exact total, biggest",
            fwrap(BIG, FILE_COLS_C + total) + f" WHERE q.status = {txt('pending')} ORDER BY q.name" + paging(0, PAGE + 1))
        add("L09 children p1 name, exact total, biggest",
            dbase(BIG, DIR_COLS_A + total) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L10 children p1 name, exact total, root",
            dbase(ROOT, DIR_COLS_A + total) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L11 mixed p1 name, exact total, biggest",
            mflat(BIG, ENTRY_COLS_C + total) + " ORDER BY n.name" + paging(0, PAGE + 1))
        add("L12 mixed last page by cursor, biggest",
            mflat(BIG, ENTRY_COLS_C) + f" AND n.name > {txt(cursor_mixed)} ORDER BY n.name" + paging(0, PAGE + 1))
        add("L13 mixed p1 name, no total, biggest",
            mflat(BIG, ENTRY_COLS_C) + " ORDER BY n.name" + paging(0, PAGE + 1))
        add("L02w files p1 name, exact total, biggest, wrapped for the composer",
            fwrap(BIG, FILE_COLS_C + total) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L03w files p1 name, no total, biggest, wrapped for the composer",
            fwrap(BIG, FILE_COLS_C) + " ORDER BY q.name" + paging(0, PAGE + 1))
        add("L05w files last page by cursor, biggest, wrapped for the composer",
            fwrap(BIG, FILE_COLS_C) + f" WHERE q.name > {txt(cursor_file)} ORDER BY q.name" + paging(0, PAGE + 1))

    # ---- design D: the interleaved listing over design A's two tables, with
    # no schema change. Only meaningful on database a.
    if design == "a":
        dcols = ("CAST('directory' AS text) AS kind, d.id, d.parent_id, d.name, "
                 "CAST(NULL AS text) AS status, CAST(NULL AS text) AS key, CAST(NULL AS bigint) AS size, "
                 "CAST(NULL AS text) AS content_type, CAST(NULL AS text) AS etag, "
                 "d.version, d.created_at, d.updated_at")
        fcols = ("CAST('file' AS text) AS kind, f.id, f.directory_id, f.name, "
                 "f.status, f.key, f.size, f.content_type, f.etag, "
                 "f.version, f.created_at, f.updated_at")
        union = (f"SELECT {dcols} FROM blobfs_directory d WHERE d.parent_id = {uid(BIG)}\n"
                 f"UNION ALL\n"
                 f"SELECT {fcols} FROM blobfs_file f WHERE f.directory_id = {uid(BIG)}")
        add("D11 mixed p1 name, exact total, biggest (UNION ALL wrapped, window count)",
            f"SELECT q.*, COUNT(*) OVER () AS total FROM (\n{union}\n) q ORDER BY q.name" + paging(0, PAGE + 1))
        add("D13 mixed p1 name, no total, biggest (UNION ALL, flat)",
            union + " ORDER BY name" + paging(0, PAGE + 1))
        add("D13w mixed p1 name, no total, biggest (UNION ALL wrapped)",
            f"SELECT * FROM (\n{union}\n) q ORDER BY q.name" + paging(0, PAGE + 1))
        add("D12 mixed last page by cursor, biggest (UNION ALL, flat, predicate in both branches)",
            f"SELECT {dcols} FROM blobfs_directory d WHERE d.parent_id = {uid(BIG)} AND d.name > {txt(cursor_mixed)}\n"
            f"UNION ALL\n"
            f"SELECT {fcols} FROM blobfs_file f WHERE f.directory_id = {uid(BIG)} AND f.name > {txt(cursor_mixed)}\n"
            f" ORDER BY name" + paging(0, PAGE + 1))
        add("D04 mixed last page by offset, no total, biggest (UNION ALL, flat)",
            union + " ORDER BY name" + paging(last_offset_mixed, PAGE + 1))

    # ---- single-row reads and tree walks, every design
    if design == "a":
        add("L15 directory_child, one path segment",
            f"SELECT d.id, d.parent_id, d.name, d.version, d.created_at, d.updated_at\n"
            f"FROM blobfs_directory d WHERE d.parent_id = {uid(TOP)} AND d.name = {txt('d3')}")
        add("L16 file_by_name",
            f"SELECT f.id, f.directory_id, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at\n"
            f"FROM blobfs_file f WHERE f.directory_id = {uid(BIG)} AND f.name = {txt('f50000.txt')}")
        add("L17 file_by_id",
            f"SELECT f.id, f.directory_id, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at\n"
            f"FROM blobfs_file f WHERE f.id = {uid('01f0c350-7000-8000-9000-00000000c350')}")
        add("L13a directory_ancestors, depth 6",
            "WITH RECURSIVE ancestors (id, parent_id, name, depth) AS (\n"
            f"    SELECT d.id, d.parent_id, d.name, CAST(0 AS integer) FROM blobfs_directory d WHERE d.id = {uid(DEEP)}\n"
            "  UNION ALL\n"
            "    SELECT d.id, d.parent_id, d.name, a.depth + 1 FROM blobfs_directory d JOIN ancestors a ON a.parent_id = d.id\n"
            ") SELECT a.parent_id, a.name FROM ancestors a ORDER BY a.depth DESC")
        add("L14a directory_is_within, depth 6 under a depth-1 ancestor",
            "WITH RECURSIVE up (id, parent_id) AS (\n"
            f"    SELECT d.id, d.parent_id FROM blobfs_directory d WHERE d.id = {uid(DEEP)}\n"
            "  UNION ALL\n"
            "    SELECT d.id, d.parent_id FROM blobfs_directory d JOIN up ON up.parent_id = d.id\n"
            f") SELECT COUNT(*) AS matches FROM up WHERE up.id = {uid(TOP)}")
    if design == "b":
        add("L15 directory_child, one path segment",
            f"SELECT q.id, q.parent_id, q.name, q.version, q.created_at, q.updated_at\n"
            f"FROM blobfs_entry q WHERE q.parent_id = {uid(TOP)} AND q.name = {txt('d3')} AND q.kind = {txt('directory')}")
        add("L16 file_by_name",
            f"SELECT {FILE_COLS_B}\nFROM blobfs_entry q WHERE q.parent_id = {uid(BIG)} AND q.name = {txt('f50000.txt')} AND q.kind = {txt('file')}")
        add("L17 file_by_id",
            f"SELECT {FILE_COLS_B}\nFROM blobfs_entry q WHERE q.id = {uid('01f0c350-7000-8000-9000-00000000c350')} AND q.kind = {txt('file')}")
        add("L13a directory_ancestors, depth 6",
            "WITH RECURSIVE ancestors (id, parent_id, name, depth) AS (\n"
            f"    SELECT d.id, d.parent_id, d.name, CAST(0 AS integer) FROM blobfs_entry d WHERE d.id = {uid(DEEP)}\n"
            "  UNION ALL\n"
            "    SELECT d.id, d.parent_id, d.name, a.depth + 1 FROM blobfs_entry d JOIN ancestors a ON a.parent_id = d.id\n"
            ") SELECT a.parent_id, a.name FROM ancestors a ORDER BY a.depth DESC")
        add("L14a directory_is_within, depth 6 under a depth-1 ancestor",
            "WITH RECURSIVE up (id, parent_id) AS (\n"
            f"    SELECT d.id, d.parent_id FROM blobfs_entry d WHERE d.id = {uid(DEEP)}\n"
            "  UNION ALL\n"
            "    SELECT d.id, d.parent_id FROM blobfs_entry d JOIN up ON up.parent_id = d.id\n"
            f") SELECT COUNT(*) AS matches FROM up WHERE up.id = {uid(TOP)}")
    if design in ("c", "c2"):
        add("L15 directory_child, one path segment",
            f"SELECT q.id, q.parent_id, q.name, q.version, q.created_at, q.updated_at\n"
            f"FROM blobfs_node q WHERE q.parent_id = {uid(TOP)} AND q.name = {txt('d3')} AND q.kind = {txt('directory')}")
        add("L16 file_by_name",
            f"SELECT {FILE_COLS_C}\nFROM blobfs_node n JOIN blobfs_file f ON f.node_id = n.id\n"
            f"WHERE n.parent_id = {uid(BIG)} AND n.name = {txt('f50000.txt')}")
        add("L17 file_by_id",
            f"SELECT {FILE_COLS_C}\nFROM blobfs_node n JOIN blobfs_file f ON f.node_id = n.id\n"
            f"WHERE n.id = {uid('01f0c350-7000-8000-9000-00000000c350')}")
        add("L13a directory_ancestors, depth 6",
            "WITH RECURSIVE ancestors (id, parent_id, name, depth) AS (\n"
            f"    SELECT d.id, d.parent_id, d.name, CAST(0 AS integer) FROM blobfs_node d WHERE d.id = {uid(DEEP)}\n"
            "  UNION ALL\n"
            "    SELECT d.id, d.parent_id, d.name, a.depth + 1 FROM blobfs_node d JOIN ancestors a ON a.parent_id = d.id\n"
            ") SELECT a.parent_id, a.name FROM ancestors a ORDER BY a.depth DESC")
        add("L14a directory_is_within, depth 6 under a depth-1 ancestor",
            "WITH RECURSIVE up (id, parent_id) AS (\n"
            f"    SELECT d.id, d.parent_id FROM blobfs_node d WHERE d.id = {uid(DEEP)}\n"
            "  UNION ALL\n"
            "    SELECT d.id, d.parent_id FROM blobfs_node d JOIN up ON up.parent_id = d.id\n"
            f") SELECT COUNT(*) AS matches FROM up WHERE up.id = {uid(TOP)}")

    # ---- the consumer's bookmark read model, the path per row
    if design == "a":
        walk = ("(WITH RECURSIVE up (directory_id, path) AS (\n"
                "      SELECT d.parent_id, COALESCE('/' || d.name, '') FROM blobfs_directory d WHERE d.id = f.directory_id\n"
                "    UNION ALL\n"
                "      SELECT d.parent_id, COALESCE('/' || d.name, '') || up.path FROM up JOIN blobfs_directory d ON d.id = up.directory_id\n"
                "  ) SELECT up.path FROM up WHERE up.directory_id IS NULL) || '/' || f.name AS path")
        add("L18 bookmarks page, one unit, path per row",
            f"SELECT b.unit_id, b.file_id, b.active,\n  {walk},\n  f.name, f.status, f.size, f.content_type, b.created_at, b.updated_at\n"
            f"FROM bookmark b JOIN blobfs_file f ON f.id = b.file_id\n"
            f"WHERE b.unit_id = {uid(UNIT)} ORDER BY path, b.file_id" + paging(0, PAGE))
    if design == "b":
        walk = ("(WITH RECURSIVE up (parent_id, path) AS (\n"
                "      SELECT d.parent_id, COALESCE('/' || d.name, '') FROM blobfs_entry d WHERE d.id = f.parent_id\n"
                "    UNION ALL\n"
                "      SELECT d.parent_id, COALESCE('/' || d.name, '') || up.path FROM up JOIN blobfs_entry d ON d.id = up.parent_id\n"
                "  ) SELECT up.path FROM up WHERE up.parent_id IS NULL) || '/' || f.name AS path")
        add("L18 bookmarks page, one unit, path per row",
            f"SELECT b.unit_id, b.file_id, b.active,\n  {walk},\n  f.name, f.status, f.size, f.content_type, b.created_at, b.updated_at\n"
            f"FROM bookmark b JOIN blobfs_entry f ON f.id = b.file_id\n"
            f"WHERE b.unit_id = {uid(UNIT)} ORDER BY path, b.file_id" + paging(0, PAGE))
    if design in ("c", "c2"):
        walk = ("(WITH RECURSIVE up (parent_id, path) AS (\n"
                "      SELECT d.parent_id, COALESCE('/' || d.name, '') FROM blobfs_node d WHERE d.id = n.parent_id\n"
                "    UNION ALL\n"
                "      SELECT d.parent_id, COALESCE('/' || d.name, '') || up.path FROM up JOIN blobfs_node d ON d.id = up.parent_id\n"
                "  ) SELECT up.path FROM up WHERE up.parent_id IS NULL) || '/' || n.name AS path")
        add("L18 bookmarks page, one unit, path per row",
            f"SELECT b.unit_id, b.file_id, b.active,\n  {walk},\n  n.name, f.status, f.size, f.content_type, b.created_at, b.updated_at\n"
            f"FROM bookmark b JOIN blobfs_file f ON f.node_id = b.file_id JOIN blobfs_node n ON n.id = b.file_id\n"
            f"WHERE b.unit_id = {uid(UNIT)} ORDER BY path, b.file_id" + paging(0, PAGE))
    return q


def main():
    designs = {"a": "blobfs_opus_a", "b": "blobfs_opus_b",
               "c": "blobfs_opus_c", "c2": "blobfs_opus_c2"}
    # the cursor values and the last-page offsets, read once from design A
    nfiles = int(psql("blobfs_opus_a", f"SELECT count(*) FROM blobfs_file WHERE directory_id='{BIG}'").strip())
    nmixed = nfiles + int(psql("blobfs_opus_a", f"SELECT count(*) FROM blobfs_directory WHERE parent_id='{BIG}'").strip())
    last_files = ((nfiles + PAGE - 1) // PAGE - 1) * PAGE
    last_mixed = ((nmixed + PAGE - 1) // PAGE - 1) * PAGE
    cursor_file = psql("blobfs_opus_a",
                       f"SELECT name FROM blobfs_file WHERE directory_id='{BIG}' ORDER BY name OFFSET {last_files - 1} LIMIT 1").strip()
    cursor_mixed = psql("blobfs_opus_a",
                        f"SELECT name FROM (SELECT name FROM blobfs_directory WHERE parent_id='{BIG}' "
                        f"UNION ALL SELECT name FROM blobfs_file WHERE directory_id='{BIG}') u "
                        f"ORDER BY name OFFSET {last_mixed - 1} LIMIT 1").strip()

    out = []
    results = []
    out.append("# blobfs core-schema evaluation: the listing and read cost of each candidate")
    out.append("# engine: " + psql("blobfs_opus_a", "SELECT version()").strip())
    out.append(f"# fixture: 10,003 directories in 3 trees to depth 6, 100,000 files; the biggest directory holds")
    out.append(f"#   {nfiles} files and 6 subdirectories at depth 2; the small one 9 files at depth 6; 1,000 files are")
    out.append("#   pending and 500 deleting. Every design holds byte-identical rows (md5 of the file set matches).")
    out.append(f"# method: each query is EXPLAIN (ANALYZE, BUFFERS) once to warm the cache, then {RUNS} times; the run")
    out.append("#   with the median execution time is reported with its plan. Buffers are the top plan node's")
    out.append("#   shared hit+read, in 8 KB pages. Every table was VACUUM ANALYZEd after loading.")
    out.append(f"# page size {PAGE} (the listing fetches one row beyond it); last file page offset {last_files},")
    out.append(f"#   last mixed page offset {last_mixed}; file cursor name {cursor_file!r}, mixed cursor {cursor_mixed!r}.")
    out.append("")

    bodies = []
    for design, db in designs.items():
        for label, sql in build(design, cursor_file, cursor_mixed, last_files, last_mixed):
            ex, pl, buf, plan = explain(db, sql)
            results.append((design, label, pl, ex, buf, shape(plan)))
            bodies.append(f"\n---- {design}: {label}: median of {RUNS} runs: "
                          f"planning {pl:.3f} ms, execution {ex:.3f} ms, buffers {buf} ----\n{sql}\n\n{plan}")
            print(f"{design:3s} {label:64s} {pl:8.3f} {ex:9.3f} {buf:8d}", file=sys.stderr)

    out.append("# summary (median run; times in ms; buffers as hit+read pages):")
    out.append(f"# {'des':<4}{'query':<66}{'plan ms':>9}{'exec ms':>10}{'buffers':>9}  plan shape")
    for d, label, pl, ex, buf, sh in results:
        out.append(f"# {d:<4}{label:<66}{pl:>9.3f}{ex:>10.3f}{buf:>9}  {sh}")
    out.extend(bodies)
    print("\n".join(out))


if __name__ == "__main__":
    main()

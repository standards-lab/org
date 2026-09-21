#!/usr/bin/env python3
"""Interleaves two real sessions over each candidate schema and records what
each design permits.

Every case follows the same shape: session 1 opens a transaction and runs the
first step, session 2 then runs the second step and the harness reports
whether session 2 blocked on a lock or ran straight through, session 1
commits, and the final row state is read. Blocking is detected by polling
pg_stat_activity for a lock wait on session 2's backend.
"""
import subprocess
import sys
import threading
import time
import uuid

DESIGNS = ["a", "b", "c", "c2"]
PARENT = '02d013ca-7000-8000-9000-0000000013ca'   # the small directory, depth 6
OTHER = '02d013cb-7000-8000-9000-0000000013cb'    # another depth-6 directory


class Session:
    """One psql process kept open, driven line by line."""

    def __init__(self, db, name):
        self.name = name
        self.p = subprocess.Popen(
            ["docker", "exec", "-i", "blobfs-postgres", "psql", "-U", "app", "-d", db,
             "-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=0"],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
            text=True, bufsize=1)
        self.pid = int(self.run("SELECT pg_backend_pid()")[0])

    def run(self, sql):
        self.p.stdin.write(sql.rstrip().rstrip(";") + ";\n")
        self.p.stdin.write("\\echo <<<END>>>\n")
        self.p.stdin.flush()
        lines = []
        for line in self.p.stdout:
            if line.strip() == "<<<END>>>":
                break
            if line.strip():
                lines.append(line.rstrip("\n"))
        return lines

    def run_async(self, sql):
        result = {}
        t = threading.Thread(target=lambda: result.setdefault("out", self.run(sql)))
        t.start()
        return t, result

    def close(self):
        try:
            self.p.stdin.write("\\q\n")
            self.p.stdin.flush()
            self.p.wait(timeout=5)
        except Exception:
            self.p.kill()


def waiting(watcher, pid, seconds=1.5):
    """True when pid is waiting on a lock within seconds."""
    deadline = time.time() + seconds
    while time.time() < deadline:
        rows = watcher.run(
            f"SELECT coalesce(wait_event_type,'') || '/' || coalesce(wait_event,'') || '/' || state "
            f"FROM pg_stat_activity WHERE pid = {pid}")
        if rows and rows[0].startswith("Lock/"):
            return True
        time.sleep(0.05)
    return False


# --------------------------------------------------------------------- cases
def newid():
    return str(uuid.uuid4())


def seed_file(s, design, fid, name, status="available", parent=PARENT):
    if design == "a":
        s.run(f"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type) "
              f"VALUES ('{fid}','{parent}','{name}','{status}','{fid}/{name}','text/plain')")
    elif design == "b":
        s.run(f"INSERT INTO blobfs_entry (id, kind, parent_id, name, status, key, content_type) "
              f"VALUES ('{fid}','file','{parent}','{name}','{status}','{fid}/{name}','text/plain')")
    else:
        s.run(f"INSERT INTO blobfs_node (id, kind, parent_id, name) VALUES ('{fid}','file','{parent}','{name}')")
        s.run(f"INSERT INTO blobfs_file (node_id, status, key, content_type) "
              f"VALUES ('{fid}','{status}','{fid}/{name}','text/plain')")


def drop_file(s, design, fid):
    if design == "a":
        s.run(f"DELETE FROM blobfs_file WHERE id='{fid}'")
    elif design == "b":
        s.run(f"DELETE FROM blobfs_entry WHERE id='{fid}'")
    else:
        s.run(f"DELETE FROM blobfs_file WHERE node_id='{fid}'")
        s.run(f"DELETE FROM blobfs_node WHERE id='{fid}'")


def state(s, design, fid):
    if design == "a":
        r = s.run(f"SELECT name || ' | ' || directory_id || ' | ' || status || ' | v' || version "
                  f"FROM blobfs_file WHERE id='{fid}'")
    elif design == "b":
        r = s.run(f"SELECT name || ' | ' || parent_id || ' | ' || status || ' | v' || version "
                  f"FROM blobfs_entry WHERE id='{fid}'")
    else:
        r = s.run(f"SELECT n.name || ' | ' || n.parent_id || ' | ' || f.status || ' | v' || n.version "
                  f"FROM blobfs_node n JOIN blobfs_file f ON f.node_id=n.id WHERE n.id='{fid}'")
    return r[0] if r else "(gone)"


# the statements each design runs for the two steps that race
def move_file(design, fid, version, to, name):
    if design == "a":
        return (f"UPDATE blobfs_file SET directory_id='{to}', name='{name}', "
                f"updated_at=CURRENT_TIMESTAMP, version=version+1 "
                f"WHERE id='{fid}' AND version={version} AND status <> 'deleting'")
    if design == "b":
        return (f"UPDATE blobfs_entry SET parent_id='{to}', name='{name}', "
                f"updated_at=CURRENT_TIMESTAMP, version=version+1 "
                f"WHERE id='{fid}' AND version={version} AND kind='file' AND status <> 'deleting'")
    return (f"UPDATE blobfs_node SET parent_id='{to}', name='{name}', "
            f"updated_at=CURRENT_TIMESTAMP, version=version+1 "
            f"WHERE id='{fid}' AND version={version} "
            f"AND EXISTS (SELECT 1 FROM blobfs_file f WHERE f.node_id=blobfs_node.id AND f.status <> 'deleting')")


def begin_delete_disciplined(design, fid):
    """The begin step as a careful implementation writes it: the status change
    and the version bump together."""
    if design == "a":
        return [f"UPDATE blobfs_file SET status='deleting', version=version+1, updated_at=CURRENT_TIMESTAMP "
                f"WHERE id='{fid}' AND status <> 'deleting'"]
    if design == "b":
        return [f"UPDATE blobfs_entry SET status='deleting', version=version+1, updated_at=CURRENT_TIMESTAMP "
                f"WHERE id='{fid}' AND kind='file' AND status <> 'deleting'"]
    return [f"UPDATE blobfs_node SET version=version+1, updated_at=CURRENT_TIMESTAMP WHERE id='{fid}'",
            f"UPDATE blobfs_file SET status='deleting' WHERE node_id='{fid}' AND status <> 'deleting'"]


def begin_delete_natural(design, fid):
    """The begin step as the column layout invites: the status lives on the
    file, so the statement touches the file. In A and B that is the same row
    the move locks; in C and C2 it is a different row in a different table."""
    if design == "a":
        return [f"UPDATE blobfs_file SET status='deleting' WHERE id='{fid}' AND status <> 'deleting'"]
    if design == "b":
        return [f"UPDATE blobfs_entry SET status='deleting' WHERE id='{fid}' AND kind='file' AND status <> 'deleting'"]
    return [f"UPDATE blobfs_file SET status='deleting' WHERE node_id='{fid}' AND status <> 'deleting'"]


def complete_write(design, fid, version):
    if design == "a":
        return [f"UPDATE blobfs_file SET status='available', size=7, etag='e', "
                f"updated_at=CURRENT_TIMESTAMP, version=version+1 "
                f"WHERE id='{fid}' AND version={version} AND status='pending'"]
    if design == "b":
        return [f"UPDATE blobfs_entry SET status='available', size=7, etag='e', "
                f"updated_at=CURRENT_TIMESTAMP, version=version+1 "
                f"WHERE id='{fid}' AND version={version} AND kind='file' AND status='pending'"]
    return [f"UPDATE blobfs_node SET updated_at=CURRENT_TIMESTAMP, version=version+1 "
            f"WHERE id='{fid}' AND version={version}",
            f"UPDATE blobfs_file SET status='available', size=7, etag='e' "
            f"WHERE node_id='{fid}' AND status='pending'"]


def case(design, title, seed_status, first, second, out):
    """first and second are (label, builder) where builder(design, fid)
    returns the statements session 1 and session 2 run, in that order, each
    inside its own transaction."""
    db = "blobfs_opus_race_" + design
    s1, s2, w = Session(db, "s1"), Session(db, "s2"), Session(db, "watch")
    fid, name = newid(), "race-" + newid()[:8]
    try:
        seed_file(s1, design, fid, name, seed_status)
        before = state(s1, design, fid)
        s1.run("BEGIN")
        r1 = []
        for stmt in first[1](design, fid):
            r1 += s1.run(stmt)
        s2.run("BEGIN")
        t, res = s2.run_async("; ".join(second[1](design, fid)))
        blocked = waiting(w, s2.pid)
        s1.run("COMMIT")
        t.join(timeout=10)
        r2 = res.get("out", ["(no answer)"])
        s2.run("COMMIT")
        after = state(s2, design, fid)
        out.append(f"  {design:3s} seeded {before}")
        out.append(f"      s1 {first[0]}: {' '.join(r1) or '(no output)'}")
        out.append(f"      s2 {second[0]}: {'BLOCKED on a row lock until s1 committed, then ' if blocked else 'ran straight through, '}"
                   f"{' '.join(r2) or '(no output)'}")
        out.append(f"      final {after}")
    finally:
        drop_file(s1, design, fid)
        for s in (s1, s2, w):
            s.close()


def main():
    out = []
    out.append("# Contention: two sessions over each schema, the same interleaving each time.")
    out.append("# s1 opens a transaction and runs the first step; s2 then runs the second step")
    out.append("# in its own transaction; the harness reports whether s2 waited on a row lock;")
    out.append("# s1 commits; the row is read afterwards. UPDATE n is the rows the update")
    out.append("# affected. The file starts in the small directory and the move sends it to")
    out.append("# another depth-six directory.")

    out.append("")
    out.append("## Case 1. A file's delete-begin, written as the column layout invites, against a move")
    out.append("##   s1 runs the delete's begin step; s2 runs the guarded move at the version it read.")
    for d in DESIGNS:
        case(d, "", "available",
             ("begin delete (status only)", begin_delete_natural),
             ("move file, guard version=1", lambda de, f: [move_file(de, f, 1, OTHER, "moved.txt")]), out)

    out.append("")
    out.append("## Case 2. The same, with the delete's begin step bumping the version too")
    for d in DESIGNS:
        case(d, "", "available",
             ("begin delete (status and version)", begin_delete_disciplined),
             ("move file, guard version=1", lambda de, f: [move_file(de, f, 1, OTHER, "moved.txt")]), out)

    out.append("")
    out.append("## Case 3. A move against the write's complete step on a pending row")
    for d in DESIGNS:
        case(d, "", "pending",
             ("move file, guard version=1", lambda de, f: [move_file(de, f, 1, OTHER, "moved.txt")]),
             ("complete write, guard version=1", lambda de, f: complete_write(de, f, 1)), out)

    print("\n".join(out))


if __name__ == "__main__":
    main()

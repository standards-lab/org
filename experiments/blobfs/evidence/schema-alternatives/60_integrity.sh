#!/usr/bin/env bash
# What each schema enforces declaratively, and what it lets through. Every
# case is run against all four databases and the engine's answer is recorded
# verbatim, with the constraint name the consumer would map to a sentinel.
set -uo pipefail
S="$(cd "$(dirname "$0")" && pwd)"
PARENT='02d013ca-7000-8000-9000-0000000013ca'   # a depth-six directory, 9 files
BIGDIR='02d00003-7000-8000-9000-000000000003'   # depth two
UNIT='01b00000-7000-8000-9000-000000000000'

q() { # $1 db  $2 sql
  docker exec blobfs-postgres psql -U app -d "blobfs_opus_race_$1" -X -q -t -A -c "$2" 2>&1 \
    | tr '\n' ' ' | sed 's/  */ /g;s/ $//'
}

say() { printf '%s\n' "$*"; }
probe() { # $1 title  $2..: per-design sql, keyed
  say "--- $1"
}

run4() { # $1 label  then four sql strings for a b c c2
  local label=$1; shift
  local i=0
  for d in a b c c2; do
    i=$((i+1))
    local sql
    sql=$(printf '%s\n' "$@" | sed -n "${i}p")
    printf '  %-3s %s\n' "$d" "$(q "$d" "$sql")"
  done
}

{
say "# What each schema enforces declaratively. Each line is the engine's answer,"
say "# so a refusal shows the constraint name a consumer maps to a sentinel."
say ""

say "## 1. One name space: a file taking the name a sibling directory already holds"
say "   (the directory d5060 already exists under $PARENT's parent; here a new"
say "    directory and a new file are given the same name in the same parent)."
for d in a b c c2; do
  case $d in
    a) setup="INSERT INTO blobfs_directory (id,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000001','$PARENT','clash')";
       act="INSERT INTO blobfs_file (id,directory_id,name,status,key,content_type) VALUES ('11111111-0000-0000-0000-000000000002','$PARENT','clash','available','k','text/plain')";
       clean="DELETE FROM blobfs_file WHERE id='11111111-0000-0000-0000-000000000002'; DELETE FROM blobfs_directory WHERE id='11111111-0000-0000-0000-000000000001'";;
    b) setup="INSERT INTO blobfs_entry (id,kind,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000001','directory','$PARENT','clash')";
       act="INSERT INTO blobfs_entry (id,kind,parent_id,name,status,key,content_type) VALUES ('11111111-0000-0000-0000-000000000002','file','$PARENT','clash','available','k','text/plain')";
       clean="DELETE FROM blobfs_entry WHERE id IN ('11111111-0000-0000-0000-000000000001','11111111-0000-0000-0000-000000000002')";;
    *) setup="INSERT INTO blobfs_node (id,kind,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000001','directory','$PARENT','clash')";
       act="INSERT INTO blobfs_node (id,kind,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000002','file','$PARENT','clash')";
       clean="DELETE FROM blobfs_node WHERE id IN ('11111111-0000-0000-0000-000000000001','11111111-0000-0000-0000-000000000002')";;
  esac
  q "$d" "$setup" >/dev/null
  printf '  %-3s %s\n' "$d" "$(q "$d" "$act")"
  q "$d" "$clean" >/dev/null
done

say ""
say "## 2. A path naming two entries at once: after case 1, how many rows does"
say "##    the path <parent>/clash reach? (design A only, since the others refused)"
q a "INSERT INTO blobfs_directory (id,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000001','$PARENT','clash')" >/dev/null
q a "INSERT INTO blobfs_file (id,directory_id,name,status,key,content_type) VALUES ('11111111-0000-0000-0000-000000000002','$PARENT','clash','available','k','text/plain')" >/dev/null
printf '  %-3s entries named clash under the parent: %s\n' a \
  "$(q a "SELECT (SELECT count(*) FROM blobfs_directory WHERE parent_id='$PARENT' AND name='clash') || ' directory + ' || (SELECT count(*) FROM blobfs_file WHERE directory_id='$PARENT' AND name='clash') || ' file'")"
q a "DELETE FROM blobfs_file WHERE id='11111111-0000-0000-0000-000000000002'" >/dev/null
q a "DELETE FROM blobfs_directory WHERE id='11111111-0000-0000-0000-000000000001'" >/dev/null

say ""
say "## 3. A directory parented to a file (a caller passing a file's id as the parent)"
run4 x \
"INSERT INTO blobfs_directory (id,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000003','01f00000-7000-8000-9000-000000000000','under-a-file')" \
"INSERT INTO blobfs_entry (id,kind,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000003','directory','01f00000-7000-8000-9000-000000000000','under-a-file')" \
"INSERT INTO blobfs_node (id,kind,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000003','directory','01f00000-7000-8000-9000-000000000000','under-a-file')" \
"INSERT INTO blobfs_node (id,kind,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000003','directory','01f00000-7000-8000-9000-000000000000','under-a-file')"
for d in a b c c2; do
  case $d in a) q a "DELETE FROM blobfs_directory WHERE id='11111111-0000-0000-0000-000000000003'" >/dev/null;;
             b) q b "DELETE FROM blobfs_entry WHERE id='11111111-0000-0000-0000-000000000003'" >/dev/null;;
             *) q "$d" "DELETE FROM blobfs_node WHERE id='11111111-0000-0000-0000-000000000003'" >/dev/null;; esac
done

say ""
say "## 4. A file row with no status (an incomplete insert reaching the table)"
run4 x \
"INSERT INTO blobfs_file (id,directory_id,name,key,content_type) VALUES ('11111111-0000-0000-0000-000000000004','$PARENT','nostatus','k','text/plain')" \
"INSERT INTO blobfs_entry (id,kind,parent_id,name,key,content_type) VALUES ('11111111-0000-0000-0000-000000000004','file','$PARENT','nostatus','k','text/plain')" \
"INSERT INTO blobfs_node (id,kind,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000004','file','$PARENT','nostatus')" \
"INSERT INTO blobfs_node (id,kind,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000004','file','$PARENT','nostatus')"

say ""
say "## 5. After case 4: is the half-written file visible to the file listing,"
say "##    and does it hold its name slot and block its parent's removal?"
for d in c c2; do
  printf '  %-3s listed by the file listing: %s\n' "$d" \
    "$(q "$d" "SELECT count(*) FROM blobfs_node n JOIN blobfs_file f ON f.node_id=n.id WHERE n.parent_id='$PARENT' AND n.name='nostatus'")"
  printf '  %-3s holds the name slot: %s\n' "$d" \
    "$(q "$d" "INSERT INTO blobfs_node (id,kind,parent_id,name) VALUES ('11111111-0000-0000-0000-000000000005','file','$PARENT','nostatus')")"
  printf '  %-3s refuses the parent directory removal: %s\n' "$d" \
    "$(q "$d" "DELETE FROM blobfs_node WHERE id='$PARENT'")"
  q "$d" "DELETE FROM blobfs_node WHERE id='11111111-0000-0000-0000-000000000004'" >/dev/null
done
q a "DELETE FROM blobfs_file WHERE id='11111111-0000-0000-0000-000000000004'" >/dev/null
q b "DELETE FROM blobfs_entry WHERE id='11111111-0000-0000-0000-000000000004'" >/dev/null

say ""
say "## 6. A directory row carrying a file's columns"
run4 x \
"SELECT 'not expressible: blobfs_directory has no status column'" \
"INSERT INTO blobfs_entry (id,kind,parent_id,name,status,key) VALUES ('11111111-0000-0000-0000-000000000006','directory','$PARENT','odd','pending','k')" \
"SELECT 'not expressible: blobfs_node has no status column'" \
"SELECT 'not expressible: blobfs_node has no status column'"

say ""
say "## 7. A consumer bookmarking a directory instead of a file"
run4 x \
"INSERT INTO bookmark (unit_id,file_id,active) VALUES ('$UNIT','$BIGDIR',false)" \
"INSERT INTO bookmark (unit_id,file_id,active) VALUES ('$UNIT','$BIGDIR',false)" \
"INSERT INTO bookmark (unit_id,file_id,active) VALUES ('$UNIT','$BIGDIR',false)" \
"INSERT INTO bookmark (unit_id,file_id,active) VALUES ('$UNIT','$BIGDIR',false)"

say ""
say "## 8. A consumer's directory_owner row naming a file instead of a directory"
run4 x \
"INSERT INTO directory_owner (directory_id,unit_id) VALUES ('01f00000-7000-8000-9000-000000000000','$UNIT')" \
"INSERT INTO directory_owner (directory_id,unit_id) VALUES ('01f00000-7000-8000-9000-000000000000','$UNIT')" \
"INSERT INTO directory_owner (directory_id,unit_id) VALUES ('01f00000-7000-8000-9000-000000000000','$UNIT')" \
"INSERT INTO directory_owner (directory_id,unit_id) VALUES ('01f00000-7000-8000-9000-000000000000','$UNIT')"

say ""
say "## 9. Removing a file row a consumer's bookmark still references"
say "##    (the file 01f00002... is bookmarked by unit 2)"
run4 x \
"DELETE FROM blobfs_file WHERE id='01f00002-7000-8000-9000-000000000002' AND status='available'" \
"DELETE FROM blobfs_entry WHERE id='01f00002-7000-8000-9000-000000000002' AND kind='file'" \
"DELETE FROM blobfs_file WHERE node_id='01f00002-7000-8000-9000-000000000002'" \
"DELETE FROM blobfs_file WHERE node_id='01f00002-7000-8000-9000-000000000002'"

say ""
say "## 10. Removing a non-empty directory"
run4 x \
"DELETE FROM blobfs_directory WHERE id='$BIGDIR' AND parent_id IS NOT NULL" \
"DELETE FROM blobfs_entry WHERE id='$BIGDIR' AND kind='directory' AND parent_id IS NOT NULL" \
"DELETE FROM blobfs_directory WHERE node_id='$BIGDIR'" \
"DELETE FROM blobfs_node WHERE id='$BIGDIR' AND kind='directory' AND parent_id IS NOT NULL"

say ""
say "## 11. A second root"
run4 x \
"INSERT INTO blobfs_directory (id) VALUES ('11111111-0000-0000-0000-000000000009')" \
"INSERT INTO blobfs_entry (id,kind) VALUES ('11111111-0000-0000-0000-000000000009','directory')" \
"INSERT INTO blobfs_node (id,kind) VALUES ('11111111-0000-0000-0000-000000000009','directory')" \
"INSERT INTO blobfs_node (id,kind) VALUES ('11111111-0000-0000-0000-000000000009','directory')"

say ""
say "## 12. A file at the root (no parent)"
run4 x \
"INSERT INTO blobfs_file (id,directory_id,name,status,key,content_type) VALUES ('1111111a-0000-0000-0000-000000000009','00000000-0000-0000-0000-000000000000','r.txt','available','k','text/plain')" \
"INSERT INTO blobfs_entry (id,kind,parent_id,name,status,key,content_type) VALUES ('1111111a-0000-0000-0000-000000000009','file','00000000-0000-0000-0000-000000000000','r.txt','available','k','text/plain')" \
"INSERT INTO blobfs_node (id,kind,parent_id,name) VALUES ('1111111a-0000-0000-0000-000000000009','file','00000000-0000-0000-0000-000000000000','r.txt')" \
"INSERT INTO blobfs_node (id,kind,parent_id,name) VALUES ('1111111a-0000-0000-0000-000000000009','file','00000000-0000-0000-0000-000000000000','r.txt')"
q a "DELETE FROM blobfs_file WHERE id='1111111a-0000-0000-0000-000000000009'" >/dev/null
q b "DELETE FROM blobfs_entry WHERE id='1111111a-0000-0000-0000-000000000009'" >/dev/null
q c "DELETE FROM blobfs_node WHERE id='1111111a-0000-0000-0000-000000000009'" >/dev/null
q c2 "DELETE FROM blobfs_node WHERE id='1111111a-0000-0000-0000-000000000009'" >/dev/null

say ""
say "## 13. A node given both subtype rows (design C only)"
printf '  %-3s %s\n' c "$(q c "INSERT INTO blobfs_directory (node_id) VALUES ('01f00000-7000-8000-9000-000000000000')")"
} | tee "$S/integrity.txt"

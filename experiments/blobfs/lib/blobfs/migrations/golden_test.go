package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"testing"
)

// released pins the sha256 of every released migration file by name. A
// released file never changes in text or name: a new migration adds a row
// here, and a changed row is a breaking change to the schema's version line.
// Versions 1 and 2 were pinned at stage 6; stage 14 added version 3, the
// upgrade rehearsal, as new rows and moved neither earlier hash.
var released = map[string]string{
	"0001_directory.up.sql":            "52373927abbd09f0509b9796a1be18597efcf65e0011eb9205819c65ccb92131",
	"0001_directory.down.sql":          "7193988c04d57b6b7729c0daa0523ee70e3c920e12baec2897ce720d5e274ba9",
	"0002_file.up.sql":                 "bafff9b9d797e61791ecbcdfd5df7db414fb2ddb2259e547b87eaa0eed454712",
	"0002_file.down.sql":               "adbf56e6f4f0ab977bcb8b3e7874bae41fe3eebf7e7a610e9cda98c162f3b14c",
	"0003_file_created_index.up.sql":   "8182a2259fe11ee1895aa7cd6bdaa386767b0f7c68c8cead567be7ef869d59ab",
	"0003_file_created_index.down.sql": "a4136970164d1e18697484889afcf796ffb144e310489c30c73ece81fe658b95",
}

// installed pins the hashes of the migrations an installed database holds
// before the stage 14 upgrade, versions 1 and 2, as stage 6 released them.
// An upgrade adds a version and never rewrites an installed one, so these
// must equal the released table's rows for the same files.
var installed = map[string]string{
	"0001_directory.up.sql":   "52373927abbd09f0509b9796a1be18597efcf65e0011eb9205819c65ccb92131",
	"0001_directory.down.sql": "7193988c04d57b6b7729c0daa0523ee70e3c920e12baec2897ce720d5e274ba9",
	"0002_file.up.sql":        "bafff9b9d797e61791ecbcdfd5df7db414fb2ddb2259e547b87eaa0eed454712",
	"0002_file.down.sql":      "adbf56e6f4f0ab977bcb8b3e7874bae41fe3eebf7e7a610e9cda98c162f3b14c",
}

// TestUpgradeKeepsInstalledHashes is the upgrade rehearsal's hash check:
// the files an installed database was migrated with carry the hashes they
// had before the rehearsal migration was added.
func TestUpgradeKeepsInstalledHashes(t *testing.T) {
	for name, want := range installed {
		if got := released[name]; got != want {
			t.Errorf("%s: released hash %s differs from the installed %s; an upgrade may not rewrite an installed migration", name, got, want)
		}
	}
}

// TestGoldenHashes checks every embedded Postgres file against the pinned
// hashes: a file whose text changed, a file the table does not list, and a
// listed file missing from the directory are each a failure.
func TestGoldenHashes(t *testing.T) {
	entries, err := fs.ReadDir(files, "postgres")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		text, err := fs.ReadFile(files, "postgres/"+e.Name())
		if err != nil {
			t.Fatalf("ReadFile %s: %v", e.Name(), err)
		}
		sum := sha256.Sum256(text)
		got := hex.EncodeToString(sum[:])
		want, ok := released[e.Name()]
		switch {
		case !ok:
			t.Errorf("%s is not in the released table; add it with hash %s", e.Name(), got)
		case got != want:
			t.Errorf("%s changed: hash %s, released %s", e.Name(), got, want)
		}
		seen[e.Name()] = true
	}
	for name := range released {
		if !seen[name] {
			t.Errorf("released file %s is missing from the postgres directory", name)
		}
	}
}

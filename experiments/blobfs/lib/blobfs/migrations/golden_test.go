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
var released = map[string]string{
	"0001_volume.up.sql":      "675067dbe9d636d8cc8a86ee7cd16459cdd8a5cedcca6a7766f4429c06afc5ef",
	"0001_volume.down.sql":    "ce82992b2d32f1045032fc9d33a0233a21d77b11488838a25dfedbdc728e6e9d",
	"0002_directory.up.sql":   "4067dc82adcd52c095a25da4de230158a86fbd043052731a5cf4402ba25d0127",
	"0002_directory.down.sql": "7193988c04d57b6b7729c0daa0523ee70e3c920e12baec2897ce720d5e274ba9",
	"0003_file.up.sql":        "bafff9b9d797e61791ecbcdfd5df7db414fb2ddb2259e547b87eaa0eed454712",
	"0003_file.down.sql":      "adbf56e6f4f0ab977bcb8b3e7874bae41fe3eebf7e7a610e9cda98c162f3b14c",
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

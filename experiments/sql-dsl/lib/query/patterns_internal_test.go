package query

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestPatterns_EveryCatalogEntryDeclaresItsTierAndSlots(t *testing.T) {
	for name, f := range patterns {
		if strings.Contains(f.text, "{{>") {
			t.Errorf("pattern %s includes another pattern; patterns hold slots only", name)
		}
	}
	if f := patterns["collection"]; len(f.slots) != 4 || f.slots[0] != "base" {
		t.Errorf("collection slots = %v", f.slots)
	}
}

func TestRender_PanicsOnAnUnfilledSlot(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("render did not panic")
		}
	}()
	render("collection", map[string]string{"base": "SELECT 1"})
}

func TestLoadPatterns_RejectsAPatternWithoutATier(t *testing.T) {
	_, err := loadPatterns(fstest.MapFS{"f/x.sql": {Data: []byte("SELECT {{a}}")}}, "f")
	if err == nil || !strings.Contains(err.Error(), "no tier") {
		t.Errorf("err = %v", err)
	}
}

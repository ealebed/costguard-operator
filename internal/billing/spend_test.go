package billing

import (
	"testing"
)

func TestValidateTableRef(t *testing.T) {
	t.Parallel()
	_, _, _, err := validateTableRef("myproj.my_dataset.my-table")
	if err != nil {
		t.Fatalf("expected valid ref: %v", err)
	}
	_, _, _, err = validateTableRef("bad")
	if err == nil {
		t.Fatal("expected error for short ref")
	}
	_, _, _, err = validateTableRef("a.b.c;d")
	if err == nil {
		t.Fatal("expected error for invalid segment")
	}
}

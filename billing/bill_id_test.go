package billing

import "testing"

func TestValidateBillID(t *testing.T) {
	if err := validateBillID("509d6f01-8329-49ec-aaec-1c8187216f38"); err != nil {
		t.Fatalf("valid uuid rejected: %v", err)
	}
	if err := validateBillID("null"); err == nil {
		t.Fatal("expected error for null id")
	}
	if err := validateBillID("not-a-uuid"); err == nil {
		t.Fatal("expected error for invalid uuid")
	}
}

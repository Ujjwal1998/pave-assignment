package billing

import (
	"testing"
	"time"

	"pave-bank/domain"
)

func TestInitialBillStatus(t *testing.T) {
	today := time.Now().UTC()
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	if got := initialBillStatus(todayDate); got != domain.BillStatusOpen {
		t.Fatalf("today period start = %q, want open", got)
	}

	future := todayDate.AddDate(0, 0, 30)
	if got := initialBillStatus(future); got != domain.BillStatusScheduled {
		t.Fatalf("future period start = %q, want scheduled", got)
	}

	past := todayDate.AddDate(0, 0, -30)
	if got := initialBillStatus(past); got != domain.BillStatusOpen {
		t.Fatalf("past period start = %q, want open", got)
	}
}

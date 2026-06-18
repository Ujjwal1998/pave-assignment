//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func baseURL() string {
	if u := os.Getenv("BASE_URL"); u != "" {
		return u
	}
	return "http://localhost:4000"
}

type httpResult struct {
	status int
	body   []byte
}

func apiJSON(t *testing.T, method, path string, payload any) httpResult {
	t.Helper()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL()+path, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return httpResult{status: resp.StatusCode, body: respBody}
}

func createBill(t *testing.T, customerID string) string {
	t.Helper()
	res := apiJSON(t, http.MethodPost, "/bills", map[string]string{
		"customer_id":  customerID,
		"period_start": "2025-06-01",
		"period_end":   "2025-06-30",
		"currency":     "USD",
	})
	if res.status != http.StatusOK {
		t.Fatalf("create bill: status=%d body=%s", res.status, res.body)
	}
	var bill struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.body, &bill); err != nil {
		t.Fatalf("decode bill: %v", err)
	}
	t.Cleanup(func() { closeBillIfOpen(t, bill.ID) })
	return bill.ID
}

func closeBillIfOpen(t *testing.T, billID string) {
	t.Helper()
	bill := getBill(t, billID)
	if bill["status"] != "open" {
		return
	}
	res := apiJSON(t, http.MethodPost, "/bills/"+billID+"/close?wait=true", nil)
	if res.status != http.StatusOK {
		t.Logf("cleanup close bill %s: status=%d body=%s", billID, res.status, res.body)
	}
}

func addLineItem(t *testing.T, billID, ref, unitPrice string) httpResult {
	t.Helper()
	return apiJSON(t, http.MethodPost, "/bills/"+billID+"/line-items", map[string]string{
		"fee_type":              "usage",
		"description":           "integration race",
		"quantity":              "1",
		"unit_price":            unitPrice,
		"effective_date":        "2025-06-10",
		"external_reference_id": ref,
	})
}

func getBill(t *testing.T, billID string) map[string]any {
	t.Helper()
	res := apiJSON(t, http.MethodGet, "/bills/"+billID, nil)
	if res.status != http.StatusOK {
		t.Fatalf("get bill: status=%d body=%s", res.status, res.body)
	}
	var bill map[string]any
	if err := json.Unmarshal(res.body, &bill); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return bill
}

func lineItemCount(bill map[string]any) int {
	items, _ := bill["line_items"].([]any)
	return len(items)
}

func TestRaceDuplicateExternalReference(t *testing.T) {
	billID := createBill(t, fmt.Sprintf("race-dup-ref-%d", time.Now().UnixNano()))
	const n = 80
	ref := fmt.Sprintf("dup-%d", time.Now().UnixNano())

	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	ids := make(chan string, n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-start
			res := addLineItem(t, billID, ref, "7.00")
			if res.status < 200 || res.status >= 300 {
				t.Errorf("add line item: status=%d body=%s", res.status, res.body)
				return
			}
			var item struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(res.body, &item)
			ids <- item.ID
		}()
	}
	close(start)
	wg.Wait()
	close(ids)

	seen := map[string]struct{}{}
	for id := range ids {
		seen[id] = struct{}{}
	}
	if len(seen) != 1 {
		t.Fatalf("expected 1 unique line item id, got %d", len(seen))
	}
	if lineItemCount(getBill(t, billID)) != 1 {
		t.Fatal("expected 1 line item in DB")
	}
}

func TestRaceDuplicateClose(t *testing.T) {
	billID := createBill(t, fmt.Sprintf("race-dup-close-%d", time.Now().UnixNano()))
	addLineItem(t, billID, "item-1", "10.00")
	addLineItem(t, billID, "item-2", "5.00")

	const n = 40
	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	var okCount atomic.Int32

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-start
			res := apiJSON(t, http.MethodPost, "/bills/"+billID+"/close?wait=true", nil)
			if res.status == http.StatusOK {
				okCount.Add(1)
			} else if res.status != 422 && res.status != 400 {
				t.Errorf("close: unexpected status=%d body=%s", res.status, res.body)
			}
		}()
	}
	close(start)
	wg.Wait()

	if okCount.Load() < 1 {
		t.Fatal("expected at least one successful close")
	}
	bill := getBill(t, billID)
	if bill["status"] != "closed" {
		t.Fatalf("bill not closed: %v", bill["status"])
	}
	if lineItemCount(bill) != 2 {
		t.Fatalf("expected 2 line items, got %d", lineItemCount(bill))
	}
}

func TestRaceCloseVsAdds(t *testing.T) {
	billID := createBill(t, fmt.Sprintf("race-close-add-%d", time.Now().UnixNano()))
	const adds = 30
	runID := time.Now().UnixNano()

	var wg sync.WaitGroup
	wg.Add(adds + 1)
	start := make(chan struct{})
	var okAdds atomic.Int32

	for i := 0; i < adds; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			ref := fmt.Sprintf("race-%d-%d", runID, i)
			res := addLineItem(t, billID, ref, "3.00")
			if res.status >= 200 && res.status < 300 {
				okAdds.Add(1)
			}
		}()
	}
	go func() {
		defer wg.Done()
		<-start
		res := apiJSON(t, http.MethodPost, "/bills/"+billID+"/close?wait=true", nil)
		if res.status != http.StatusOK {
			t.Errorf("close: status=%d body=%s", res.status, res.body)
		}
	}()
	close(start)
	wg.Wait()

	bill := getBill(t, billID)
	if bill["status"] != "closed" {
		t.Fatalf("bill not closed")
	}
	if lineItemCount(bill) != int(okAdds.Load()) {
		t.Fatalf("line item count=%d ok_adds=%d", lineItemCount(bill), okAdds.Load())
	}
}

func TestRaceAddAfterCloseRejected(t *testing.T) {
	billID := createBill(t, fmt.Sprintf("race-after-close-%d", time.Now().UnixNano()))
	addLineItem(t, billID, "only", "12.00")
	closeRes := apiJSON(t, http.MethodPost, "/bills/"+billID+"/close", nil)
	if closeRes.status != http.StatusOK {
		t.Fatalf("close: %d %s", closeRes.status, closeRes.body)
	}

	const n = 30
	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			res := addLineItem(t, billID, fmt.Sprintf("late-%d", i), "1.00")
			if res.status != 422 && res.status != 400 {
				t.Errorf("expected 422, got %d body=%s", res.status, res.body)
			}
		}()
	}
	close(start)
	wg.Wait()

	if lineItemCount(getBill(t, billID)) != 1 {
		t.Fatal("line items added after close")
	}
}

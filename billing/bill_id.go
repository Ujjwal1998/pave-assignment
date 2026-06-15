package billing

import (
	"fmt"

	"github.com/google/uuid"
)

func validateBillID(id string) error {
	if id == "" || id == "null" {
		return fmt.Errorf("bill id is required")
	}
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("bill id must be a valid UUID")
	}
	return nil
}

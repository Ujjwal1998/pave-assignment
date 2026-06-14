package domain

import "testing"

func TestFeeTypeValid(t *testing.T) {
	tests := []struct {
		feeType FeeType
		want    bool
	}{
		{FeeTypeSubscription, true},
		{FeeTypeUsage, true},
		{FeeTypeTax, true},
		{FeeTypePenalty, true},
		{FeeTypeDiscount, true},
		{FeeType("rebate"), false},
		{FeeType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.feeType), func(t *testing.T) {
			if got := tt.feeType.Valid(); got != tt.want {
				t.Fatalf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

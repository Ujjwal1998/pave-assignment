package money

// Static FX rates for converting line item amounts to the bill (invoice) currency.
// amount_in_to = amount_in_from * rate
//
// Rates are compile-time constants for deterministic Temporal replay.
var rates = map[string]map[string]string{
	"USD": {
		"USD": "1",
		"GEL": "2.70",
	},
	"GEL": {
		"GEL": "1",
		"USD": "0.37",
	},
}

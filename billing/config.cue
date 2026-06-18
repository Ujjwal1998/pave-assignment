package billing

// Local dev uses Temporalite or `temporal server start-dev` on the default port.
TemporalServer: [
	if #Meta.Environment.Cloud == "local" { "127.0.0.1:7233" },
	// TODO: set your cloud Temporal cluster address for deployed environments.
	"127.0.0.1:7233",
][0]

// Static FX rates for multi-currency line items (see money/rates.go for authoritative values).
// USD→GEL: 2.70, GEL→USD: 0.37
FXRates: {
	USD: {USD: "1", GEL: "2.70"}
	GEL: {GEL: "1", USD: "0.37"}
}

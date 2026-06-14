package billing

// Local dev uses Temporalite or `temporal server start-dev` on the default port.
TemporalServer: [
	if #Meta.Environment.Cloud == "local" { "127.0.0.1:7233" },
	// TODO: set your cloud Temporal cluster address for deployed environments.
	"127.0.0.1:7233",
][0]

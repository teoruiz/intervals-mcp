package runtime

import "github.com/teoruiz/intervals-mcp/internal/platform/config"

type Config = config.Config
type DiscoveryOptions = config.DiscoveryOptions

func LoadDiscovered(opts DiscoveryOptions) (Config, config.Source, error) {
	return config.LoadDiscovered(opts)
}

func LoadIntervalsDiscovered(opts DiscoveryOptions) (Config, config.Source, error) {
	return config.LoadIntervalsDiscovered(opts)
}

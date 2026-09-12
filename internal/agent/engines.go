package agent

import "github.com/honeydipper/honeydipper/v4/pkg/dipper"

// EngineEntry pairs a driver name with an engine (model) name available on that
// driver. It is the shared representation returned by CollectAgentDriverEngines
// and used by both /model and the engine-listing API.
type EngineEntry struct {
	Driver string `json:"driver"`
	Engine string `json:"engine"`
}

// isAgentDriver checks whether a driver config (from drivers.daemon.drivers.<name>)
// has "agent_driver" in its meta.labels.
func isAgentDriver(driverConfig interface{}) bool {
	labels, ok := dipper.GetMapData(driverConfig, "meta.labels")
	if !ok {
		return false
	}

	labelList, ok := labels.([]interface{})
	if !ok {
		return false
	}

	for _, l := range labelList {
		if s, ok := l.(string); ok && s == "agent_driver" {
			return true
		}
	}

	return false
}

// collectEngineEntriesFromDriver extracts unique {driver,engine} pairs from the
// engines block of a single driver config. Duplicates are filtered via the seen
// set so the same pair reported by multiple daemon entries appears only once.
func collectEngineEntriesFromDriver(driverName string, driverConfig interface{}, seen map[string]bool, entries *[]EngineEntry) {
	engines, ok := dipper.GetMapData(driverConfig, "engines")
	if !ok {
		return
	}

	engineMap, ok := engines.(map[string]interface{})
	if !ok {
		return
	}

	for engineName := range engineMap {
		key := driverName + ":" + engineName
		if seen[key] {
			continue
		}
		seen[key] = true
		*entries = append(*entries, EngineEntry{Driver: driverName, Engine: engineName})
	}
}

// collectDriverEngines iterates over drivers.daemon.drivers, filtering for
// agent-driver labelled entries, and populates entries with {driver,engine}
// pairs. It reads the engines from the top-level driver config at
// drivers.<name>.engines.
func collectDriverEngines(drivers map[string]interface{}, seen map[string]bool, entries *[]EngineEntry) {
	raw, ok := dipper.GetMapData(drivers, "daemon.drivers")
	if !ok {
		return
	}

	driverMap, ok := raw.(map[string]interface{})
	if !ok {
		return
	}

	for driverName, driverConfig := range driverMap {
		if !isAgentDriver(driverConfig) {
			continue
		}

		// Look up the actual driver config (e.g. drivers.openai) for engines,
		// not the daemon metadata (drivers.daemon.drivers.openai).
		actualDriverConfig, ok := drivers[driverName]
		if !ok {
			continue
		}

		collectEngineEntriesFromDriver(driverName, actualDriverConfig, seen, entries)
	}
}

// CollectAgentDriverEngines returns the unique {driver,engine} pairs available
// on all agent drivers in the config. It is the single source of truth shared
// by the /model slash command and the engine-listing API so both always agree
// on the set of known engines.
func CollectAgentDriverEngines(drivers map[string]interface{}) []EngineEntry {
	seen := map[string]bool{}
	entries := []EngineEntry{}
	collectDriverEngines(drivers, seen, &entries)

	return entries
}

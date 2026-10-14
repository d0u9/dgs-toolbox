package config

// Cache is where a command keeps its discardable local cache. It is outside
// what the cache describes on purpose: a cache belongs to one machine, and a
// database file on a network filesystem is a way to lose data.
type Cache struct {
	// Dir is the cache directory. Empty means <user cache dir>/dgs/<command>.
	Dir string `json:"dir"`
}

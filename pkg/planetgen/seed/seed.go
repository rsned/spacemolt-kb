package seed

import "hash/fnv"

// Hash converts a string (typically a planet name) to a deterministic
// int64 seed via fnv-64a.
func Hash(name string) int64 {
	h := fnv.New64a()
	h.Write([]byte(name))
	return int64(h.Sum64())
}

package main

import (
	"fmt"
	"hash/fnv"
)

// seedHash is a stable 64-bit FNV-1a hash of s, used to derive deterministic
// visual variation from an entity ID.
func seedHash(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// derivePalette returns (field, accent) CSS colors derived deterministically
// from seed. Used when a profile has no color data of its own. Both are
// hsl() strings, valid as SVG fills.
func derivePalette(seed string) (field, accent string) {
	h := seedHash(seed)
	hue := h % 360
	field = fmt.Sprintf("hsl(%d 45%% 38%%)", hue)
	accent = fmt.Sprintf("hsl(%d 60%% 58%%)", (hue+150)%360)
	return field, accent
}

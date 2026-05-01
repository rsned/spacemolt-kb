package render

import (
	"container/list"
	"encoding/json"
	"hash/fnv"
	"sync"
	"sync/atomic"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// flatCacheEntry holds the post-Normalize heightmap and Temperature/Humidity
// climate fields that the colorize stage needs.
type flatCacheEntry struct {
	Heightmap []float64
	TempField []float64
	HumField  []float64
}

// flatCacheCapacity is the maximum number of entries in the flat LRU cache.
const flatCacheCapacity = 8

type flatLRU struct {
	mu    sync.Mutex
	cap   int
	ll    *list.List
	items map[string]*list.Element
}

type flatCacheItem struct {
	key   string
	value flatCacheEntry
}

func newFlatLRU(capacity int) *flatLRU {
	return &flatLRU{cap: capacity, ll: list.New(), items: map[string]*list.Element{}}
}

func (l *flatLRU) get(key string) (flatCacheEntry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	el, ok := l.items[key]
	if !ok {
		return flatCacheEntry{}, false
	}
	l.ll.MoveToFront(el)
	return el.Value.(*flatCacheItem).value, true //nolint:forcetypeassert
}

func (l *flatLRU) put(key string, value flatCacheEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.items[key]; ok {
		el.Value.(*flatCacheItem).value = value //nolint:forcetypeassert
		l.ll.MoveToFront(el)
		return
	}
	el := l.ll.PushFront(&flatCacheItem{key: key, value: value})
	l.items[key] = el
	if l.ll.Len() > l.cap {
		oldest := l.ll.Back()
		if oldest != nil {
			l.ll.Remove(oldest)
			delete(l.items, oldest.Value.(*flatCacheItem).key) //nolint:forcetypeassert
		}
	}
}

// flatUpstreamCache stores post-Normalize state keyed by upstream-affecting params.
var flatUpstreamCache = newFlatLRU(flatCacheCapacity)

// flatCacheHits and flatCacheMisses count LRU outcomes since process start.
var (
	flatCacheHits   uint64
	flatCacheMisses uint64
)

// FlatCacheStats returns (hits, misses) since process start. Exported for wasm.
func FlatCacheStats() (uint64, uint64) {
	return atomic.LoadUint64(&flatCacheHits), atomic.LoadUint64(&flatCacheMisses)
}

// flatUpstreamKey hashes the inputs that affect stages 1-4 of RenderFlat.
// Any new pre-Normalize stage or ControlField must be added here.
func flatUpstreamKey(prof *types.PlanetProfile, masterSeed int64, size int) string {
	h := fnv.New64a()
	type subset struct {
		Seed               int64
		Size               int
		HeightSmoothRadius int
		Continentalness    types.ControlField
		Detail             types.ControlField
		PeaksValleys       types.ControlField
		Temperature        types.ControlField
		Humidity           types.ControlField
	}
	s := subset{
		Seed:               masterSeed,
		Size:               size,
		HeightSmoothRadius: prof.HeightSmoothRadius,
		Continentalness:    prof.ControlConfig.Continentalness,
		Detail:             prof.ControlConfig.Detail,
		PeaksValleys:       prof.ControlConfig.PeaksValleys,
		Temperature:        prof.ControlConfig.Temperature,
		Humidity:           prof.ControlConfig.Humidity,
	}
	if b, err := json.Marshal(s); err == nil {
		_, _ = h.Write(b)
	}
	return string(h.Sum(nil))
}

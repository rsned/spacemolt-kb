package main

import (
	"strings"
	"testing"
	"time"
)

func TestLinkWormholesPairsBothIDSchemes(t *testing.T) {
	sac := &System{ID: "saclateni", Name: "Saclateni", POIs: []SystemPOI{
		{ID: "wh_entrance_72e87d7c", Type: "wormhole_entrance", LastUpdatedTick: 1000 - 360*3},
	}}
	okab := &System{ID: "okab", Name: "Okab", POIs: []SystemPOI{
		{ID: "wh_exit_72e87d7c", Type: "wormhole_exit", LastUpdatedTick: 1000},
	}}
	alz := &System{ID: "alzirr", Name: "Alzirr", POIs: []SystemPOI{
		{ID: "leap_of_faith", Type: "wormhole_entrance"},
		{ID: "wh_entrance_dead", Type: "wormhole_collapsed"},
	}}
	pra := &System{ID: "praecipua", Name: "Praecipua", POIs: []SystemPOI{
		{ID: "leap_of_faith_exit", Type: "wormhole_exit"},
		{ID: "wh_exit_dead", Type: "wormhole_collapsed"},
	}}
	lone := &System{ID: "deneb", Name: "Deneb", POIs: []SystemPOI{
		{ID: "wh_entrance_49e52112", Type: "wormhole_entrance"},
	}}
	linkWormholes([]*System{sac, okab, alz, pra, lone})

	if got := sac.POIs[0].WormholeTo; got == nil || got.ID != "okab" || got.Name != "Okab" || got.SeenAgo != "3h" {
		t.Errorf("saclateni entrance -> %+v (age measured from the newest POI tick, 10 s/tick)", got)
	}
	if got := okab.POIs[0].WormholeFrom; got == nil || got.SeenAgo != "just now" {
		t.Errorf("okab exit seen = %+v", got)
	}
	if got := okab.POIs[0].WormholeFrom; got == nil || got.ID != "saclateni" {
		t.Errorf("okab exit <- %+v", got)
	}
	if got := alz.POIs[0].WormholeTo; got == nil || got.ID != "praecipua" {
		t.Errorf("named entrance -> %+v", got)
	}
	if pra.POIs[0].WormholeFrom == nil || pra.POIs[0].WormholeFrom.ID != "alzirr" {
		t.Errorf("named exit <- %+v", pra.POIs[0].WormholeFrom)
	}
	if alz.POIs[1].WormholeTo != nil || pra.POIs[1].WormholeFrom != nil {
		t.Error("collapsed wormholes must not be linked")
	}
	if lone.POIs[0].WormholeTo != nil {
		t.Error("an entrance with no recorded exit must stay unlinked")
	}
}

func TestHumanAge(t *testing.T) {
	for ticks, want := range map[int]string{0: "just now", 5: "just now", 6: "1m", 359: "59m", 360: "1h", 360 * 47: "47h", 360 * 48: "2d", 360 * 24 * 9: "9d"} {
		if got := humanAge(ticks); got != want {
			t.Errorf("humanAge(%d) = %q, want %q", ticks, got, want)
		}
	}
}

func TestDropExpiredWormholes(t *testing.T) {
	now := time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)
	const nowTick = 1_000_000
	sys := &System{ID: "s", POIs: []SystemPOI{
		{ID: "star", Type: "sun"},
		{ID: "old_collapsed", Type: "wormhole_collapsed", ExpiresAt: "2026-11-08T14:58:32Z"},       // expired 4+ days ago
		{ID: "recent_collapsed", Type: "wormhole_collapsed", ExpiresAt: "2026-11-11T20:59:48Z"},    // expired yesterday
		{ID: "unknown_old", Type: "wormhole_collapsed", LastUpdatedTick: nowTick - 3*86400/10 - 1}, // seen just over 3 days ago
		{ID: "unknown_recent", Type: "wormhole_collapsed", LastUpdatedTick: nowTick - 100},
		{ID: "live_entrance", Type: "wormhole_entrance", LastUpdatedTick: nowTick - 3*86400}, // no expiry known: kept
		{ID: "expired_entrance", Type: "wormhole_entrance", ExpiresAt: "2026-11-01T00:00:00Z"},
	}}
	dropExpiredWormholes([]*System{sys}, now, nowTick)
	var kept []string
	for _, p := range sys.POIs {
		kept = append(kept, p.ID)
	}
	if got, want := strings.Join(kept, ","), "star,recent_collapsed,unknown_recent,live_entrance"; got != want {
		t.Errorf("kept = %s, want %s", got, want)
	}
}

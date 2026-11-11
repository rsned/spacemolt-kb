package main

import "testing"

func TestLinkWormholesPairsBothIDSchemes(t *testing.T) {
	sac := &System{ID: "saclateni", Name: "Saclateni", POIs: []SystemPOI{
		{ID: "wh_entrance_72e87d7c", Type: "wormhole_entrance"},
	}}
	okab := &System{ID: "okab", Name: "Okab", POIs: []SystemPOI{
		{ID: "wh_exit_72e87d7c", Type: "wormhole_exit"},
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

	if got := sac.POIs[0].WormholeTo; got == nil || got.ID != "okab" || got.Name != "Okab" {
		t.Errorf("saclateni entrance -> %+v", got)
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

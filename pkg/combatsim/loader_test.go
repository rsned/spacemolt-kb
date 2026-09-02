package combatsim

import (
	"path/filepath"
	"testing"
)

func catalogDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "data", "combat-sim", "catalog")
}

func TestLoadCatalog(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	pl := cat.Items["pulse_laser_iii"]
	if pl == nil || pl.Damage != 28 || pl.DamageType != "energy" || pl.Cooldown != 1 {
		t.Errorf("pulse_laser_iii = %+v, want damage 28 energy cooldown 1", pl)
	}
	ac := cat.Items["autocannon_ii"]
	if ac == nil || ac.Damage != 14 || ac.MagazineSize != 650 {
		t.Errorf("autocannon_ii = %+v, want damage 14 magazine 650", ac)
	}
	as := cat.Items["adaptive_shield_iii"]
	if as == nil || as.DamageReduction != 35 || as.ShieldBonus != 200 {
		t.Errorf("adaptive_shield_iii = %+v, want damage_reduction 35 shield_bonus 200", as)
	}
	bx := cat.Ships["broadaxe"]
	if bx == nil || bx.BaseHull != 282 || bx.BaseShield != 28 || bx.BaseArmor != 18 || bx.BaseShieldRecharge != 2 {
		t.Errorf("broadaxe = %+v, want hull 282 shield 28 armor 18 recharge 2", bx)
	}
	if sv := cat.Ships["survey_vessel"]; sv == nil || sv.BaseArmor != 14 {
		t.Errorf("survey_vessel = %+v, want base_armor 14", sv)
	}
}

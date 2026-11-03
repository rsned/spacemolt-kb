package prodcost

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

// HullFlag marks a caveat on a hull's make cost.
type HullFlag string

// Hull flags.
const (
	FlagCrystal      HullFlag = "uses energy crystal"
	FlagUnbuilt      HullFlag = "facility not built"
	FlagSalvage      HullFlag = "needs salvage"
	FlagUnpriced     HullFlag = "unpriced input"
	FlagCycle        HullFlag = "cycle"
	FlagLaborUnknown HullFlag = "labor unknown"
)

// royaltyRate is the share of material cost charged as royalty.
const royaltyRate = 0.10

// Quote is one recorded shipyard commission quote.
type Quote struct {
	ShipClass   string
	Tier, Labor int
	Royalty     *int
	StationID   string
	Mode        string
	Tick        int64
}

// ListingMin is the cheapest player listing of a hull class in the window.
type ListingMin struct {
	Price, Count int
	StationID    string
}

// HullData is the shipyard knowledge CostHulls draws on.
type HullData struct {
	Quotes []Quote
	// Licensed maps ship class to whether CRFT owns its license.
	Licensed map[string]bool
	// Listings maps ship class to its cheapest listing in the window.
	Listings map[string]ListingMin
}

// HullMaterial is one build material of a hull, costed.
type HullMaterial struct {
	ItemID string
	Qty    float64
	// Route is the chosen route label, "raw" or "unpriced".
	Route          string
	CreditsPerUnit float64
	Credits        float64
	// Flags found anywhere in this material's tree.
	Flags []HullFlag
	// AskPrice, AskShort and AskStation are BuyNow for Qty.
	AskPrice, AskShort float64
	AskStation         string
}

// LicenseState says whether CRFT can build a hull.
type LicenseState string

// License states.
const (
	LicenseOwned         LicenseState = "owned"
	LicenseNotOwned      LicenseState = "not owned"
	LicenseNotLicensable LicenseState = "not licensable"
	LicenseNotBuildable  LicenseState = "not buildable"
)

// HullCost is the make-versus-market picture of one hull.
type HullCost struct {
	Ship      catalog.Ship
	Materials []HullMaterial
	// MaterialCredits sums Credits; a floor when Partial.
	MaterialCredits float64
	// Partial is set when a salvage, unpriced or cycle step contributed nothing.
	Partial bool
	// Labor is nil when unknown; LaborNote is "quoted", "tier quote",
	// "tier range a–b" or "".
	Labor     *float64
	LaborNote string
	Royalty   float64
	// QuotedRoyalty is the latest recorded royalty for this class.
	QuotedRoyalty *int
	MakeTotal     float64
	License       LicenseState
	LicenseCost   int
	// MarketCredits sums AskPrice*Qty; MarketShort is set when any ask is short.
	MarketCredits float64
	MarketShort   bool
	Listing       *ListingMin
	Flags         []HullFlag
	// Best is set when making beats the market ask and any listing; never
	// when Labor is nil.
	Best bool
}

// LicenseCost is the license price for a hull tier, 0 for an unknown tier.
func LicenseCost(tier int) int {
	switch tier {
	case 1:
		return 1_000_000
	case 2:
		return 5_000_000
	case 3:
		return 10_000_000
	case 4:
		return 20_000_000
	case 5:
		return 50_000_000
	}
	return 0
}

// CostHulls costs every ship's build materials from costs (Compute with
// UnbuiltL1 and no SkipInputs), keeping the ships' order.
func CostHulls(ships []catalog.Ship, w World, costs map[string]*ItemCost, m Market, d HullData) []HullCost {
	recipes := make(map[string]catalog.Recipe, len(w.Recipes))
	for _, r := range w.Recipes {
		recipes[r.ID] = r
	}
	out := make([]HullCost, 0, len(ships))
	for _, s := range ships {
		out = append(out, costHull(s, w, recipes, costs, m, d))
	}
	return out
}

func costHull(s catalog.Ship, w World, recipes map[string]catalog.Recipe, costs map[string]*ItemCost, m Market, d HullData) HullCost {
	hc := HullCost{Ship: s}
	for _, bm := range s.BuildMaterials {
		hm := HullMaterial{ItemID: bm.ItemID, Qty: bm.Quantity}
		ic := costs[bm.ItemID]
		switch {
		case isRaw(w, bm.ItemID):
			hm.Route = "raw"
		case ic == nil || ic.NoUsableRoute || ic.Cheapest < 0:
			hm.Route = "unpriced"
		default:
			rc := ic.Routes[ic.Cheapest]
			hm.Route = rc.Label()
			hm.CreditsPerUnit = rc.CreditsPerUnit
			hm.Credits = bm.Quantity * rc.CreditsPerUnit
			if rc.Partial {
				hc.Partial = true
			}
		}
		walkFlags(bm.ItemID, w, recipes, costs, map[string]bool{}, &hm.Flags)
		if slices.ContainsFunc(hm.Flags, func(f HullFlag) bool {
			return f == FlagSalvage || f == FlagUnpriced || f == FlagCycle
		}) {
			hc.Partial = true
		}
		for _, f := range hm.Flags {
			if !slices.Contains(hc.Flags, f) {
				hc.Flags = append(hc.Flags, f)
			}
		}
		hm.AskPrice, hm.AskShort, hm.AskStation = BuyNow(m, bm.ItemID, bm.Quantity)
		hc.MaterialCredits += hm.Credits
		hc.MarketCredits += hm.AskPrice * bm.Quantity
		hc.MarketShort = hc.MarketShort || hm.AskShort > 0
		hc.Materials = append(hc.Materials, hm)
	}
	hc.Royalty = royaltyRate * hc.MaterialCredits
	hc.License, hc.LicenseCost = licenseState(s, d)
	hc.Labor, hc.LaborNote = hullLabor(s, d.Quotes)
	if hc.Labor == nil && s.CapturePolicy != "reclaim_only" {
		hc.Flags = append(hc.Flags, FlagLaborUnknown)
	}
	hc.QuotedRoyalty = quotedRoyalty(d.Quotes, s.Class)
	hc.MakeTotal = hc.MaterialCredits + hc.Royalty
	if hc.Labor != nil {
		hc.MakeTotal += *hc.Labor
	}
	if l, ok := d.Listings[s.Class]; ok {
		hc.Listing = &l
	}
	hc.Best = hc.Labor != nil && hc.MakeTotal < hc.MarketCredits && (hc.Listing == nil || hc.MakeTotal < float64(hc.Listing.Price))
	return hc
}

// quotedRoyalty is the royalty of the latest class quote that has one.
func quotedRoyalty(qs []Quote, class string) *int {
	var out *int
	var tick int64
	for _, q := range qs {
		if q.ShipClass == class && q.Royalty != nil && (out == nil || q.Tick >= tick) {
			out, tick = q.Royalty, q.Tick
		}
	}
	return out
}

func isRaw(w World, id string) bool {
	it, ok := w.Items[id]
	return ok && len(it.ExtractedBy) > 0
}

// walkFlags collects the flags in item's tree along each cheapest route.
func walkFlags(item string, w World, recipes map[string]catalog.Recipe, costs map[string]*ItemCost, seen map[string]bool, flags *[]HullFlag) {
	add := func(f HullFlag) {
		if !slices.Contains(*flags, f) {
			*flags = append(*flags, f)
		}
	}
	if seen[item] || isRaw(w, item) {
		return
	}
	seen[item] = true
	ic := costs[item]
	if ic == nil || ic.NoUsableRoute || ic.Cheapest < 0 {
		switch item {
		case "salvage_components", "salvage_metal", "rare_salvage":
			add(FlagSalvage)
		default:
			add(FlagUnpriced)
		}
		return
	}
	rc := ic.Routes[ic.Cheapest]
	if rec, ok := recipes[rc.RecipeID]; ok && slices.ContainsFunc(rec.Inputs, func(in catalog.Material) bool { return in.ItemID == "energy_crystal" }) {
		add(FlagCrystal)
	}
	if rc.Kind == KindOwn && rc.Usable {
		add(FlagUnbuilt)
	}
	if rc.Cycle {
		add(FlagCycle)
	}
	for _, in := range rc.Inputs {
		walkFlags(in.ItemID, w, recipes, costs, seen, flags)
	}
}

func licenseState(s catalog.Ship, d HullData) (LicenseState, int) {
	switch {
	case s.CapturePolicy == "reclaim_only":
		return LicenseNotBuildable, 0
	case s.PrestigeLock != "" || s.StarterShip || s.NPCRole == "boss_flagship":
		return LicenseNotLicensable, 0
	case s.Faction != "":
		if d.Licensed[s.Class] {
			return LicenseOwned, LicenseCost(s.Tier)
		}
		return LicenseNotOwned, LicenseCost(s.Tier)
	}
	return LicenseNotLicensable, 0
}

// hullLabor picks the labor for s: its own latest quote, else the quotes at
// its tier, else unknown.
func hullLabor(s catalog.Ship, quotes []Quote) (*float64, string) {
	if s.CapturePolicy == "reclaim_only" {
		return nil, ""
	}
	var latest *Quote
	for i, q := range quotes {
		if q.ShipClass == s.Class && q.Labor > 0 && (latest == nil || q.Tick >= latest.Tick) {
			latest = &quotes[i]
		}
	}
	if latest != nil {
		v := float64(latest.Labor)
		return &v, "quoted"
	}
	lo, hi, found := 0, 0, false
	for _, q := range quotes {
		if q.Tier != s.Tier || q.Labor <= 0 {
			continue
		}
		if !found {
			lo, hi, found = q.Labor, q.Labor, true
		}
		lo, hi = min(lo, q.Labor), max(hi, q.Labor)
	}
	if !found {
		return nil, ""
	}
	v := float64(lo)
	if lo == hi {
		return &v, "tier quote"
	}
	return &v, fmt.Sprintf("tier range %s–%s", commas(lo), commas(hi))
}

// commas formats n with thousands separators.
func commas(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

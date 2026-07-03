package jumpmap

// blockedCap is the most a coverage figure may show when escape gaps exist, so a
// system with a (possibly sub-0.1%) gap never reads as "100.0% blocked".
const blockedCap = 99.9

// DisplayBlockedPct returns the blocked-coverage percentage to display. When any
// escape gap exists it is capped at 99.9% so it never rounds up to a misleading
// 100.0%; a truly sealed system (no gaps) still shows its real value.
func DisplayBlockedPct(blockedPct float64, gapCount int) float64 {
	if gapCount > 0 && blockedPct > blockedCap {
		return blockedCap
	}
	return blockedPct
}

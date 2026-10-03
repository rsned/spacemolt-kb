package tectonics

import "fmt"

// PlateRow is one plate's state as emitted in a Frame.
type PlateRow struct {
	ID        int32      `json:"id"`
	Centroid  [3]float64 `json:"centroid"`
	Pole      [3]float64 `json:"pole"`
	SpeedCmYr float64    `json:"speed_cm_yr"`
	Area      float64    `json:"area"`
	Major     bool       `json:"major"`
	Retired   bool       `json:"retired"`
}

// Frame is one emitted snapshot of the simulation: the step number, the
// elapsed geologic time, and copies of every per-pixel grid plus the plate
// table at that point.
type Frame struct {
	Step    int
	Myr     float64
	Th      *Grid[float64]
	Labels  *Grid[int32]
	Feature *Grid[uint8]
	Plates  []PlateRow
}

// Validate reports whether p's knobs are internally consistent enough to
// run: a face size in 8..1024, a positive step count, and sane plate-count
// and speed ranges.
func Validate(p Params) error {
	switch {
	case p.Face < 8:
		return fmt.Errorf("face %d < 8", p.Face)
	case p.Face > 1024:
		return fmt.Errorf("face %d > 1024", p.Face)
	case p.Steps < 1:
		return fmt.Errorf("steps %d < 1", p.Steps)
	case p.KeyframeEvery < 1:
		return fmt.Errorf("KeyframeEvery %d < 1", p.KeyframeEvery)
	case p.MajorMin > p.MajorMax || p.MinorMin > p.MinorMax || p.MajorMin < 1:
		return fmt.Errorf("plate count ranges %d-%d / %d-%d invalid", p.MajorMin, p.MajorMax, p.MinorMin, p.MinorMax)
	case p.SpeedMinCmYr < 0 || p.SpeedMaxCmYr < p.SpeedMinCmYr:
		return fmt.Errorf("speed range %g-%g invalid", p.SpeedMinCmYr, p.SpeedMaxCmYr)
	case p.RepoleEvery < 1:
		return fmt.Errorf("RepoleEvery %d < 1", p.RepoleEvery)
	}
	return nil
}

// Snapshot copies the current grids and plate table into a Frame at the
// state's current step, so callers may retain it across further Steps.
func (s *State) Snapshot(myrPerStep float64) Frame {
	feat := NewGrid[uint8](s.Th.S)
	for i := range feat.Cells {
		feat.Cells[i] = s.FeatureByte(i)
	}
	rows := make([]PlateRow, len(s.Plates))
	for i, pl := range s.Plates {
		rows[i] = PlateRow{ID: pl.ID, Centroid: pl.Centroid, Pole: s.Motions[i].Pole,
			SpeedCmYr: s.Motions[i].SpeedCmYr, Area: pl.Area, Major: pl.Major, Retired: pl.Retired}
	}
	return Frame{Step: s.StepNo, Myr: float64(s.StepNo) * myrPerStep,
		Th: s.Th.Clone(), Labels: s.Labels.Clone(), Feature: feat, Plates: rows}
}

// Run builds the initial plates for (p, master) and drives them for
// p.Steps steps, calling emit for frame 0, every KeyframeEvery-th step and
// the last.
func Run(p Params, master int64, emit func(Frame) error) error {
	if err := Validate(p); err != nil {
		return err
	}
	th := GenerateThickness(p, master)
	labels, plates := BuildPlates(th, p, master)
	motions := InitMotion(th, labels, plates, p, master)
	s := NewState(p, th, labels, plates, motions)
	stepRNG := newRNG(master, "sim.step")
	reaimRNG := newRNG(master, "sim.reaim")
	if err := emit(s.Snapshot(p.MyrPerStep())); err != nil {
		return err
	}
	for step := 1; step <= p.Steps; step++ {
		s.Step(stepRNG)
		if step%p.RepoleEvery == 0 {
			Reaim(s.Th, s.Labels, s.Plates, s.Motions, p, reaimRNG)
		}
		if step%p.KeyframeEvery == 0 || step == p.Steps {
			if err := emit(s.Snapshot(p.MyrPerStep())); err != nil {
				return err
			}
		}
	}
	return nil
}

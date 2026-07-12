package render

// ProgressFunc receives pipeline stage announcements. stage is a
// canonical key ("render:heightmap", "Ridged"); i/n give the stage's
// 1-based position and the stage count when known. Conditional stages
// are skipped when disabled, so i may jump — treat i/n as position,
// not a completion guarantee.
type ProgressFunc func(stage string, i, n int)

// progressHook is a nil-default package global, set exactly once at
// boot by single-threaded consumers (the planet-explorer wasm worker)
// and nil everywhere else. Do not set it concurrently with renders.
var progressHook ProgressFunc

// SetProgressHook installs fn as the pipeline progress callback.
// Pass nil to disable.
func SetProgressHook(fn ProgressFunc) { progressHook = fn }

func reportProgress(stage string, i, n int) {
	if progressHook != nil {
		progressHook(stage, i, n)
	}
}

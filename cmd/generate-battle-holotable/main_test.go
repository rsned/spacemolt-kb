package main

import "testing"

func TestBattleIDPatternRejectsPathTraversal(t *testing.T) {
	valid := "a2619bbe328676445828b4e1007fe9aa"
	if !battleIDPattern.MatchString(valid) {
		t.Errorf("battleIDPattern rejected a real battle id %q", valid)
	}

	for _, bad := range []string{
		"", "../../etc/passwd", "a2619bbe328676445828b4e1007fe9a", // one char short
		"a2619bbe328676445828b4e1007fe9aaZ",                       // uppercase / extra char
		"../" + valid,
		"/" + valid,
	} {
		if battleIDPattern.MatchString(bad) {
			t.Errorf("battleIDPattern accepted %q, which would reach filepath.Join unvalidated", bad)
		}
	}
}

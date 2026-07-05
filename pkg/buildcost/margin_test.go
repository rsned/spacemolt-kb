package buildcost

import "testing"

func TestMargin_SavingsAndProfit(t *testing.T) {
	m := Margin{FinishedAsk: 6000, FinishedBid: 4500, HasAsk: true, HasBid: true}
	if s, ok := m.SavingsVsAsk(1894); !ok || !approx(s, 4106) {
		t.Fatalf("savings: got %v ok=%v want 4106", s, ok)
	}
	if p, ok := m.ProfitVsBid(1894); !ok || !approx(p, 2606) {
		t.Fatalf("profit: got %v ok=%v want 2606", p, ok)
	}
}

func TestMargin_Absent(t *testing.T) {
	m := Margin{}
	if _, ok := m.SavingsVsAsk(100); ok {
		t.Fatalf("expected no ask")
	}
	if _, ok := m.ProfitVsBid(100); ok {
		t.Fatalf("expected no bid")
	}
}

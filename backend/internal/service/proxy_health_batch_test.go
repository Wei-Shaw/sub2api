package service

import "testing"

func TestTakeBatchWindowRotates(t *testing.T) {
	s := &ProxyHealthService{}
	cands := make([]Proxy, 0, 5)
	for i := 1; i <= 5; i++ {
		cands = append(cands, Proxy{ID: int64(i), Name: "p"})
	}
	first := s.takeBatchWindow(cands, 2)
	if len(first) != 2 || first[0].ID != 1 || first[1].ID != 2 {
		t.Fatalf("first window=%v", first)
	}
	second := s.takeBatchWindow(cands, 2)
	if len(second) != 2 || second[0].ID != 3 || second[1].ID != 4 {
		t.Fatalf("second window=%v", second)
	}
	third := s.takeBatchWindow(cands, 2)
	// wraps: 5,1
	if len(third) != 2 || third[0].ID != 5 || third[1].ID != 1 {
		t.Fatalf("third window=%v", third)
	}
}

package store

import "testing"

func TestAudit_WriteAndList(t *testing.T) {
	s := newTestStore(t)
	s.WriteAudit("admin", "test.action", "target-1", map[string]any{"k": "v"}, "127.0.0.1")
	s.WriteAudit("admin2", "test.action2", "target-2", nil, "10.0.0.1")
	out, err := s.ListAudit(50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 audit rows, got %d", len(out))
	}
	// 倒序：最新在前
	if out[0].Actor != "admin2" {
		t.Fatalf("first row actor = %q, want admin2", out[0].Actor)
	}
	if out[1].Detail == "" {
		t.Fatalf("second row detail empty, want JSON")
	}
	n, err := s.CountAudit()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("CountAudit = %d, want 2", n)
	}
}

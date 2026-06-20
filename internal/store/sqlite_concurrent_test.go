package store

import (
	"sync"
	"testing"
)

// TestSQLite_SingleConn_ConcurrentReads 验证 SQLite 单 conn 设置下并发读不阻塞
// 写场景不在单元测试范围（生产应改 PostgreSQL，见 VULN-014）
func TestSQLite_SingleConn_ConcurrentReads(t *testing.T) {
	s := newTestStore(t)
	// 写一行
	s.WriteAudit("admin", "concurrent.test", "t", nil, "127.0.0.1")

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := s.ListAudit(10, 0)
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent read failed: %v", err)
	}
}

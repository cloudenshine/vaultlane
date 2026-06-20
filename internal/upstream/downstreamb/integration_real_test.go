//go:build integration
// +build integration

package downstreamb

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

func newIntegrationClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("RUN_DOWNSTREAM_INTEGRATION") != "1" {
		t.Skip("set RUN_DOWNSTREAM_INTEGRATION=1 to run downstream integration tests")
	}
	appKey, err := strconv.ParseInt(os.Getenv("DOWNSTREAM_APP_KEY"), 10, 64)
	if err != nil || appKey <= 0 {
		t.Fatal("DOWNSTREAM_APP_KEY is required")
	}
	appSecret := os.Getenv("DOWNSTREAM_APP_SECRET")
	if appSecret == "" {
		t.Fatal("DOWNSTREAM_APP_SECRET is required")
	}
	return New(Config{AppKey: appKey, AppSecret: appSecret}, nil)
}

// TestIntegration_Connect 真实连通测试（需要环境变量凭据）
// 运行：RUN_DOWNSTREAM_INTEGRATION=1 DOWNSTREAM_APP_KEY=... DOWNSTREAM_APP_SECRET=... go test ./internal/upstream/downstreamb/... -tags integration -run TestIntegration -v
func TestIntegration_Connect(t *testing.T) {
	c := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := c.GetUserAuthorizeList(ctx, &GetUserAuthorizeListReq{})
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	t.Logf("connected: %d authorizations", len(resp.List))
	for _, a := range resp.List {
		t.Logf("  - %s (shop=%s, valid=%v)", a.UserName, a.ShopName, a.IsValid)
	}
}

func TestIntegration_Products(t *testing.T) {
	c := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := c.GetOpenProductList(ctx, &GetOpenProductListReq{PageNo: 1, PageSize: 5})
	if err != nil {
		t.Fatalf("list products failed: %v", err)
	}
	t.Logf("products count = %d", resp.Count)
	for _, p := range resp.List {
		t.Logf("  - id=%d title=%q price=%d stock=%d", p.ProductID, p.Title, p.Price, p.Stock)
	}
}

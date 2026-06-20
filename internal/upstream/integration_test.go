package upstream

import (
	"context"
	"os"
	"testing"
	"time"

	"faka-gateway/internal/config"
)

func TestIntegration_RealUpstream(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("RUN_UPSTREAM_INTEGRATION") != "1" {
		t.Skip("set RUN_UPSTREAM_INTEGRATION=1 to run real upstream integration test")
	}

	cfg := config.UpstreamConfig{
		BaseURL:   os.Getenv("UPSTREAM_BASE_URL"),
		AppID:     os.Getenv("UPSTREAM_APP_ID"),
		AppKey:    os.Getenv("UPSTREAM_APP_KEY"),
		Timeout:   15,
		RetryMax:  1,
		UserAgent: "FakaGateway-Test/1.0",
	}
	if cfg.BaseURL == "" || cfg.AppID == "" || cfg.AppKey == "" {
		t.Fatal("UPSTREAM_BASE_URL, UPSTREAM_APP_ID and UPSTREAM_APP_KEY are required")
	}
	c := New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("categories", func(t *testing.T) {
		cats, err := c.Categories(ctx)
		if err != nil {
			t.Fatalf("categories: %v", err)
		}
		t.Logf("got %d categories", len(cats))
		if len(cats) == 0 {
			t.Fatal("empty categories")
		}
		for _, c := range cats[:min(3, len(cats))] {
			t.Logf("  - %s (id=%d, count=%d)", c.Name, c.ID, c.CommodityCount)
		}
	})

	t.Run("commodities_list", func(t *testing.T) {
		items, total, err := c.Commodities(ctx, 1, 5, 0, "")
		if err != nil {
			t.Fatalf("commodities: %v", err)
		}
		t.Logf("got %d items, total=%d", len(items), total)
		if len(items) == 0 {
			t.Fatal("empty commodities")
		}
		t.Logf("first: %s ¥%.2f", items[0].Name, items[0].Price)
	})

	t.Run("commodities_search", func(t *testing.T) {
		items, total, err := c.Commodities(ctx, 1, 5, 0, "示例A")
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		t.Logf("search 示例A: %d items, total=%d", len(items), total)
	})

	t.Run("commodity_detail", func(t *testing.T) {
		detail, err := c.CommodityDetail(ctx, 51)
		if err != nil {
			t.Fatalf("detail: %v", err)
		}
		t.Logf("detail: %s, ¥%.2f, min=%d max=%d", detail.Name, detail.Price, detail.Minimum, detail.Maximum)
		if detail.Config != nil {
			t.Logf("  config keys: %d", len(detail.Config))
		}
	})
}

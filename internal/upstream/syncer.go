package upstream

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"faka-gateway/internal/config"
	"faka-gateway/internal/store"
)

// Syncer 自动定时同步与价格熔断器
type Syncer struct {
	mgr      *Manager
	store    *store.Store
	cfg      *config.Config
	logger   *slog.Logger
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewSyncer 构造同步器
func NewSyncer(mgr *Manager, st *store.Store, cfg *config.Config, logger *slog.Logger) *Syncer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Syncer{
		mgr:      mgr,
		store:    st,
		cfg:      cfg,
		logger:   logger,
		stopChan: make(chan struct{}),
	}
}

// Start 启动后台定时同步任务
func (s *Syncer) Start(interval time.Duration) {
	if !s.cfg.Modules.UpstreamSync {
		s.logger.Info("upstream_sync module is disabled, syncer will not run")
		return
	}
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		s.logger.Info("upstream background syncer started", "interval", interval.String())

		// 启动后执行首次同步
		s.SyncOnce()

		for {
			select {
			case <-s.stopChan:
				return
			case <-ticker.C:
				s.SyncOnce()
			}
		}
	}()
}

// Stop 停止同步器
func (s *Syncer) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}

// SyncOnce 执行单次全量同步与价格熔断检查
func (s *Syncer) SyncOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	s.logger.Info("starting periodic upstream sync...")
	for _, a := range s.mgr.Adapters() {
		if a == nil || !a.Enabled() {
			continue
		}
		snap, err := FetchOneSnapshot(ctx, a, 3)
		if err != nil {
			s.logger.Warn("periodic sync failed for adapter", "upstream", a.Name(), "err", err)
			continue
		}

		// 1. 同步分类
		for _, cat := range snap.Categories {
			_ = s.store.UpsertCategory(&store.Category{
				Name:   cat.Name,
				Icon:   cat.Icon,
				Sort:   int(cat.ID),
				Source: "upstream:" + a.Name(),
			})
		}

		// 2. 同步商品与价格熔断校验
		syncedCount := 0
		for _, it := range snap.Commodity {
			cfgJSON, _ := json.Marshal(map[string]any{"config": it.Config})
			com := &store.Commodity{
				CategoryID:    it.CategoryID,
				Name:          it.Name,
				Cover:         it.Cover,
				Description:   it.Description,
				CostPrice:     it.Price,
				Stock:         it.Stock,
				SharedCode:    it.OuterID,
				OuterID:       it.OuterID,
				Source:        it.Source,
				Tags:          it.Tags,
				Minimum:       it.Minimum,
				Maximum:       it.Maximum,
				UpstreamExtra: it.Extra,
				Config:        string(cfgJSON),
				DeliveryWay:   2,
			}
			id, isNew, err := s.store.UpsertFromUpstream(com)
			if err != nil {
				continue
			}
			syncedCount++

			// 价格熔断保护：若上游涨价导致售价低于成本价，自动防亏本上调或下架预警
			if !isNew && id > 0 {
				existing, gerr := s.store.GetCommodityByID(id)
				if gerr == nil && existing != nil && existing.CostPrice > existing.SalePrice && existing.SalePrice > 0 {
					s.logger.Warn("circuit breaker: cost price exceeded sale price! auto adjusting",
						"commodity_id", existing.ID, "name", existing.Name,
						"old_sale", existing.SalePrice, "new_cost", existing.CostPrice)
					existing.SalePrice = existing.CostPrice * 1.1
					existing.Price = existing.SalePrice
					_ = s.store.UpdateCommodity(existing)
				}
			}
		}

		_ = s.store.UpsertUpstreamRuntime(&store.UpstreamRuntime{
			Name:        a.Name(),
			Enabled:     true,
			LastStatus:  "ok",
			SyncedCount: syncedCount,
		})
	}
	s.logger.Info("periodic upstream sync completed successfully")
}

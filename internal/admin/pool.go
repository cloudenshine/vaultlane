package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"faka-gateway/internal/store"
	"faka-gateway/internal/upstream"
)

// PoolHandlers 商品池专用处理器
type PoolHandlers struct {
	Store   *store.Store
	Manager *upstream.Manager
	Logger  *slog.Logger
}

// RegisterPoolRoutes 注册 /api/pool 路由
func (h *Handlers) RegisterPoolRoutes(api *gin.RouterGroup, pool *PoolHandlers) {
	g := api.Group("/pool")
	{
		// 商品池列表（含下架，含筛选）
		g.GET("", pool.HandlePoolList)
		// 批量上下架
		g.POST("/status", pool.HandlePoolBatchStatus)
		// 批量改价
		g.POST("/price", h.RequireTOTP(), pool.HandlePoolBatchPrice)
		// 单条更新
		g.PUT("/:id", pool.HandlePoolUpdate)
		// 单条删除
		g.DELETE("/:id", h.RequireTOTP(), pool.HandlePoolDelete)
		// 从上游同步
		g.POST("/sync", pool.HandlePoolSync)
		// 上游运行状态
		g.GET("/upstreams", pool.HandlePoolUpstreams)
	}
}

// PoolListResp 商品池列表响应
type PoolListResp struct {
	Code  int               `json:"code"`
	Msg   string            `json:"msg"`
	Data  []store.Commodity `json:"data"`
	Total int               `json:"total"`
	Page  int               `json:"page"`
	Limit int               `json:"limit"`
	Stats map[string]int    `json:"stats"`
}

// HandlePoolList 商品池列表
func (p *PoolHandlers) HandlePoolList(c *gin.Context) {
	source := c.Query("source")
	keyword := c.Query("keyword")
	page, _ := parseIntQuery(c, "page", 1)
	limit, _ := parseIntQuery(c, "limit", 50)
	items, total, err := p.Store.ListAllCommodities(source, keyword, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	bySource, _ := p.Store.CountCommoditiesBySource()
	c.JSON(http.StatusOK, gin.H{
		"code":  200,
		"msg":   "success",
		"data":  items,
		"total": total,
		"page":  page,
		"limit": limit,
		"stats": gin.H{"by_source": bySource},
	})
}

// HandlePoolBatchStatus 批量上下架
func (p *PoolHandlers) HandlePoolBatchStatus(c *gin.Context) {
	var req struct {
		IDs    []int64 `json:"ids"`
		Status int     `json:"status"` // 0=下架 1=上架
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "ids 必填"})
		return
	}
	n, err := p.Store.BatchSetStatus(req.IDs, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已更新", "data": gin.H{"affected": n}})
}

// HandlePoolBatchPrice 批量改价
func (p *PoolHandlers) HandlePoolBatchPrice(c *gin.Context) {
	var req struct {
		IDs    []int64 `json:"ids"`
		Type   int     `json:"type"` // 0=固定 1=百分比
		Op     string  `json:"op"`   // +/-
		Amount float64 `json:"amount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "ids 必填"})
		return
	}
	if req.Op != "+" && req.Op != "-" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "op 必为 + 或 -"})
		return
	}
	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "amount 必填且 >0"})
		return
	}
	n, err := p.Store.BatchAdjustPrice(req.IDs, req.Type, req.Op, req.Amount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已更新", "data": gin.H{"affected": n}})
}

// HandlePoolUpdate 单条更新
func (p *PoolHandlers) HandlePoolUpdate(c *gin.Context) {
	id, err := parseInt64(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid id"})
		return
	}
	var com store.Commodity
	if err := c.ShouldBindJSON(&com); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	com.ID = id
	if err := p.Store.UpdateCommodity(&com); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已更新", "data": com})
}

// HandlePoolDelete 单条删除
func (p *PoolHandlers) HandlePoolDelete(c *gin.Context) {
	id, err := parseInt64(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid id"})
		return
	}
	if err := p.Store.DeleteCommodity(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "已删除"})
}

// HandlePoolSync 从上游同步
func (p *PoolHandlers) HandlePoolSync(c *gin.Context) {
	upstreamName := c.Query("upstream")
	if upstreamName == "" {
		upstreamName = "all"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// 选择要同步的 adapter
	adapters := []upstream.Adapter{}
	if upstreamName == "all" {
		for _, a := range p.Manager.Adapters() {
			adapters = append(adapters, a)
		}
	} else if a, ok := p.Manager.Adapter(upstreamName); ok {
		adapters = append(adapters, a)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "未找到上游: " + upstreamName})
		return
	}

	type Result struct {
		Upstream   string `json:"upstream"`
		Added      int    `json:"added"`
		Updated    int    `json:"updated"`
		Categories int    `json:"categories"`
		Error      string `json:"error"`
	}
	results := []Result{}
	for _, a := range adapters {
		r := Result{Upstream: a.Name()}
		snap, err := upstream.FetchOneSnapshot(ctx, a, 3)
		if err != nil {
			r.Error = err.Error()
			p.Logger.Warn("pool sync failed", "upstream", a.Name(), "err", err)
			// 写 runtime
			_ = p.Store.UpsertUpstreamRuntime(&store.UpstreamRuntime{
				Name:        a.Name(),
				Enabled:     true,
				LastStatus:  "error",
				LastError:   err.Error(),
				SyncedCount: 0,
			})
			results = append(results, r)
			continue
		}
		r.Categories = len(snap.Categories)
		// 1) 落分类（如果有的话）
		for _, cat := range snap.Categories {
			// 直接用 categories 表存（去重 upsert）
			_ = p.Store.UpsertCategory(&store.Category{
				Name:   cat.Name,
				Icon:   cat.Icon,
				Sort:   int(cat.ID),
				Source: "upstream:" + a.Name(),
			})
		}
		// 2) 落商品
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
				DeliveryWay:   2, // 上游发货
			}
			_, isNew, err := p.Store.UpsertFromUpstream(com)
			if err != nil {
				p.Logger.Warn("upsert failed", "outer", it.OuterID, "err", err)
				continue
			}
			if isNew {
				r.Added++
			} else {
				r.Updated++
			}
		}
		// 3) 写 runtime
		_ = p.Store.UpsertUpstreamRuntime(&store.UpstreamRuntime{
			Name:        a.Name(),
			Enabled:     true,
			LastStatus:  "ok",
			SyncedCount: r.Added + r.Updated,
		})
		results = append(results, r)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "同步完成",
		"data": gin.H{"results": results},
	})
}

// HandlePoolUpstreams 上游状态列表
func (p *PoolHandlers) HandlePoolUpstreams(c *gin.Context) {
	adapters := p.Manager.Adapters()
	runtimes, _ := p.Store.ListUpstreamRuntimes()
	rtByName := map[string]store.UpstreamRuntime{}
	for _, r := range runtimes {
		rtByName[r.Name] = r
	}
	type UpstreamInfo struct {
		Name        string          `json:"name"`
		Type        string          `json:"type"`
		Enabled     bool            `json:"enabled"`
		BaseURL     string          `json:"base_url"`
		LastSyncAt  *time.Time      `json:"last_sync_at"`
		LastStatus  string          `json:"last_status"`
		LastError   string          `json:"last_error"`
		SyncedCount int             `json:"synced_count"`
		Metrics     upstream.Metric `json:"metrics"`
		MockStatus  map[string]any  `json:"mock_status"`
	}
	out := []UpstreamInfo{}
	for n, a := range adapters {
		rt := rtByName[n]
		out = append(out, UpstreamInfo{
			Name:        n,
			Type:        a.Type(),
			Enabled:     a.Enabled(),
			LastSyncAt:  rt.LastSyncAt,
			LastStatus:  rt.LastStatus,
			LastError:   rt.LastError,
			SyncedCount: rt.SyncedCount,
			Metrics:     a.Metrics(),
			MockStatus:  a.MockStatus(),
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": out})
}

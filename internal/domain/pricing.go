package domain

import "faka-gateway/internal/config"

// PremiumEngine 加价规则引擎
type PremiumEngine struct {
	cfg config.PricingConfig
}

// NewPremiumEngine 构造
func NewPremiumEngine(cfg config.PricingConfig) *PremiumEngine {
	return &PremiumEngine{cfg: cfg}
}

// Apply 计算下游售价
// 返回 (cost 成本, sale 售价)
func (p *PremiumEngine) Apply(categoryID, commodityID int, userPrice, costPrice float64) (cost, sale float64) {
	cost = costPrice
	if cost <= 0 {
		cost = userPrice
	}

	sale = userPrice
	if sale < cost {
		sale = cost
	}

	// 商品级覆盖 > 分类级覆盖 > 全局
	rule := p.cfg.Global
	if v, ok := p.cfg.CommodityOverrides[commodityID]; ok {
		rule = v
	} else if v, ok := p.cfg.CategoryOverrides[categoryID]; ok {
		rule = v
	}

	switch rule.Type {
	case 0: // 固定金额
		sale += rule.Amount
	case 1: // 百分比
		sale += sale * rule.Amount
	}

	// 最低保本
	if sale < cost {
		sale = cost
	}

	return
}

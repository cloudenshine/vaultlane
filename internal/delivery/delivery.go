// Package delivery 发货引擎（多种方式）
package delivery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"faka-gateway/internal/store"
	"faka-gateway/internal/upstream"
)

// Engine 发货接口
type Engine interface {
	Way() int // 0=手动 1=自动卡密 2=上游 3=邮件
	Deliver(ctx context.Context, o *store.Order) error
}

// SecretEngine 自营卡密（已有逻辑迁移）
type SecretEngine struct {
	Store *store.Store
}

func (s *SecretEngine) Way() int { return 1 }

func (s *SecretEngine) Deliver(ctx context.Context, o *store.Order) error {
	if o.Status >= 2 {
		return nil
	}
	if o.Contents != "" {
		// 已发过
		o.Status = 2
		return nil
	}
	secrets := []string{}
	for i := 0; i < o.Num; i++ {
		cs, err := s.Store.PullSecret(int64(o.CommodityID), o.ID)
		if err != nil {
			return err
		}
		if cs == nil {
			return fmt.Errorf("库存不足：已发 %d/%d", i, o.Num)
		}
		secrets = append(secrets, cs.Content)
	}
	o.Contents = strings.Join(secrets, "\n")
	o.UpstreamCode = 200
	o.UpstreamMsg = "success (self)"
	o.Status = 2
	return nil
}

// UpstreamEngine 上游（upstreama / 下游网关 / mock）
type UpstreamEngine struct {
	Up      upstream.Adapter
	Manager *upstream.Manager
	Logger  *slog.Logger
}

func (u *UpstreamEngine) Way() int { return 2 }

func (u *UpstreamEngine) Deliver(ctx context.Context, o *store.Order) error {
	if o.Status >= 2 {
		return nil
	}
	if o.Contents != "" {
		o.Status = 2
		return nil
	}
	params := map[string][]string{
		"shared_code": {o.SharedCode},
		"contact":     {o.Contact},
		"num":         {itoa(o.Num)},
		"request_no":  {o.RequestNo},
	}
	if o.Race != "" {
		params["race"] = []string{o.Race}
	}
	if o.Password != "" {
		params["password"] = []string{o.Password}
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	adp := u.Up
	if u.Manager != nil && strings.HasPrefix(o.Source, "upstream:") {
		upName := strings.TrimPrefix(o.Source, "upstream:")
		if target, ok := u.Manager.Adapter(upName); ok && target != nil {
			adp = target
		}
	}
	trade, err := upstream.Trade(adp, cctx, params)
	if err != nil {
		o.UpstreamCode = -1
		o.UpstreamMsg = err.Error()
		return err
	}
	o.UpstreamCode = trade.Code
	o.UpstreamMsg = trade.Msg
	if trade.Contents != "" {
		o.Contents = trade.Contents
	}
	if trade.TradeNo != "" {
		o.TradeNo = trade.TradeNo
	}
	o.Status = 2
	return nil
}

// ManualEngine 手动发货：标记待处理，等待后台填卡密
type ManualEngine struct{}

func (m *ManualEngine) Way() int { return 0 }

func (m *ManualEngine) Deliver(ctx context.Context, o *store.Order) error {
	o.UpstreamCode = 0
	o.UpstreamMsg = "待人工发货"
	o.Status = 1 // 已支付待发
	return nil
}

// EmailEngine 邮件发货（占位）
type EmailEngine struct {
	SMTPHost string
	SMTPPort int
	User     string
	Pass     string
	From     string
}

func (e *EmailEngine) Way() int { return 3 }

func (e *EmailEngine) Deliver(ctx context.Context, o *store.Order) error {
	// 真实实现：连 SMTP 发邮件
	// 这里把卡密写到 contents 即可，前端/后台展示
	if o.Contents == "" {
		return errors.New("email: 需要先有卡密内容")
	}
	o.Status = 2
	o.UpstreamMsg = "success (email)"
	return nil
}

// Dispatcher 路由（按 way）
type Dispatcher struct {
	engines map[int]Engine
}

func NewDispatcher(engines ...Engine) *Dispatcher {
	d := &Dispatcher{engines: map[int]Engine{}}
	for _, e := range engines {
		d.engines[e.Way()] = e
	}
	return d
}

func (d *Dispatcher) Register(e Engine) { d.engines[e.Way()] = e }

// Dispatch 选择 way 并执行
func (d *Dispatcher) Dispatch(ctx context.Context, o *store.Order) error {
	// 自营卡密 vs 上游：这里按 Source 优先判断
	if o.Source == "self" {
		if e, ok := d.engines[1]; ok {
			return e.Deliver(ctx, o)
		}
	} else {
		if e, ok := d.engines[2]; ok {
			return e.Deliver(ctx, o)
		}
	}
	// 兜底：手动
	if e, ok := d.engines[0]; ok {
		return e.Deliver(ctx, o)
	}
	return errors.New("delivery: no engine for order")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"faka-gateway/internal/delivery"
	"faka-gateway/internal/payment"
	"faka-gateway/internal/store"
	"faka-gateway/internal/upstream"
)

// PaymentHandler 支付 + 下单闭环
type PaymentHandler struct {
	Store   *store.Store
	Up      upstream.Adapter
	Pay     *payment.Manager
	Logger  *slog.Logger
	Notify  *Notifier
	UserH   *UserHandler
	Limiter *RateLimiter // VULN-013 业务维度限流（uid 维度，独立于 IP 限流）
}

// RegisterRoutes 注册支付与下单路由
func (p *PaymentHandler) RegisterRoutes(g *gin.RouterGroup, requireUser gin.HandlerFunc) {
	// 下单：必须登录（VULN-001 修复）
	g.POST("/orders", p.UserH.RequireUserStrict(), p.handleOrderCreateV3)
	// 查询订单：必须登录
	g.GET("/orders/:trade_no", p.UserH.RequireUserStrict(), p.handleOrderGetV3)
	// 支付：必须登录
	g.POST("/payment/create", p.UserH.RequireUserStrict(), p.handlePaymentCreate)
	g.GET("/payment/check/:id", p.UserH.RequireUserStrict(), p.handlePaymentCheck)
	// 异步回调（公开，验签）
	g.POST("/payment/callback/:method", p.handlePaymentCallback)
	g.GET("/payment/callback/:method", p.handlePaymentCallback)                           // 易支付常用 GET
	g.POST("/payment/balance/finish", p.UserH.RequireUserStrict(), p.handleBalanceFinish) // 余额内部结束
	// 可用支付方式（公开）
	g.GET("/payment/methods", p.handlePaymentMethods)
	// 优惠券校验（公开）
	g.POST("/coupon/verify", p.handleCouponVerify)
}

// Notifier 发货引擎抽象（避免 payment 包依赖 upstream/store 实现）
type Notifier struct {
	Store      *store.Store
	Up         upstream.Adapter
	Manager    *upstream.Manager
	Logger     *slog.Logger
	Dispatcher DispatcherIface
	Email      *delivery.EmailSender
}

// DispatcherIface 解耦 delivery 包
type DispatcherIface interface {
	Dispatch(ctx context.Context, o *store.Order) error
}

// FinishPaidOrder 标记订单已支付并自动发货（idempotent）
func (n *Notifier) FinishPaidOrder(tradeNo string) error {
	order, err := n.Store.GetOrderByTradeNo(tradeNo)
	if err != nil {
		return err
	}
	if order == nil {
		return errors.New("order not found")
	}
	if order.Status >= 2 {
		// 已发货，幂等返回
		return nil
	}
	// 余额支付：扣减在 CreatePay 完成
	// 这里只标记已支付 + 发卡
	order.Status = 1
	if err := n.Store.UpdateOrder(order); err != nil {
		return err
	}
	// 发卡
	if err := n.deliver(order); err != nil {
		n.Logger.Error("deliver failed", "trade", tradeNo, "err", sanitizeLogValue(err.Error()))
		return err
	}
	// 优惠券核销
	if order.CouponID > 0 {
		_ = n.Store.UseCoupon(order.CouponID, order.UserID, order.ID, order.Discount)
	}
	// 分销返佣 (5%)
	if order.UserID > 0 && order.Amount > 0 {
		_ = n.Store.ProcessOrderCommission(order.ID, order.UserID, order.Amount, 0.05)
	}
	// 邮件发卡通知
	if n.Email != nil && order.Contact != "" {
		go func(ord store.Order) {
			_ = n.Email.SendDeliveryEmail(&ord)
		}(*order)
	}
	return nil
}

// deliver 根据 source 走不同发货链路
func (n *Notifier) deliver(o *store.Order) error {
	if n.Dispatcher != nil {
		return n.Dispatcher.Dispatch(context.Background(), o)
	}
	// 兜底（避免包循环：直接 self/上游分支）
	if o.Source == "self" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	trade, err := upstream.Trade(n.Up, ctx, map[string][]string{
		"shared_code": {o.SharedCode},
		"contact":     {o.Contact},
		"num":         {strconv.Itoa(o.Num)},
		"request_no":  {o.RequestNo},
	})
	if err != nil {
		o.UpstreamCode = -1
		o.UpstreamMsg = err.Error()
		_ = n.Store.UpdateOrder(o)
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
	return n.Store.UpdateOrder(o)
}

// ---- v3.0 下单 ----

type orderReqV3 struct {
	CommodityID int    `json:"commodity_id"`
	SharedCode  string `json:"shared_code"`
	Contact     string `json:"contact"`
	Num         int    `json:"num"`
	Race        string `json:"race"`
	Password    string `json:"password"`
	PayMethod   string `json:"pay_method"` // balance / epay / "" = 不支付
	NeedPay     bool   `json:"need_pay"`   // true: 等待支付; false: 直接发卡(自营/老链路)
	CouponCode  string `json:"coupon_code"`// 优惠券码
	InviteCode  string `json:"invite_code"`// 邀请码
}

func (p *PaymentHandler) handleCouponVerify(c *gin.Context) {
	var req struct {
		Code   string  `json:"code"`
		Amount float64 `json:"amount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	uid := currentUserID(c)
	coupon, discount, err := p.Store.ValidateCoupon(req.Code, req.Amount, uid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "success",
		"data": gin.H{
			"coupon_id":   coupon.ID,
			"code":        coupon.Code,
			"name":        coupon.Name,
			"discount":    discount,
			"final_price": req.Amount - discount,
		},
	})
}

func (p *PaymentHandler) handleOrderCreateV3(c *gin.Context) {
	var req orderReqV3
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	if req.CommodityID <= 0 && req.SharedCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "需要提供 commodity_id 或 shared_code"})
		return
	}
	if req.Contact == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "联系方式不能为空"})
		return
	}
	if !isValidContact(req.Contact) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "联系方式格式不正确"})
		return
	}
	if req.Num <= 0 {
		req.Num = 1
	}

	// 解析商品
	sharedCode := req.SharedCode
	detailName := ""
	detailPrice := 0.0
	detailMin, detailMax := 0, 0
	detailCardID := 0
	isSelf := false

	var com *store.Commodity
	if req.CommodityID > 0 {
		com, _ = p.Store.GetCommodityByID(int64(req.CommodityID))
	}

	if sharedCode == "" {
		if com != nil {
			detailName = com.Name
			detailPrice = com.SalePrice
			if detailPrice <= 0 {
				detailPrice = com.Price
			}
			detailMin, detailMax = com.Minimum, com.Maximum
			if detailMax <= 0 {
				detailMax = com.Stock
			}
			if detailMin <= 0 {
				detailMin = 1
			}
			if com.Source == "self" {
				isSelf = true
				sharedCode = "self-" + strconv.FormatInt(com.ID, 10)
			} else {
				sharedCode = com.SharedCode
				if sharedCode == "" {
					sharedCode = com.OuterID
				}
			}
		} else {
			ctx, cancel := ctxWithTimeout(c, 15*time.Second)
			defer cancel()
			detail, err := p.Up.FetchCommodityDetail(ctx, req.CommodityID)
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "获取商品信息失败: " + err.Error()})
				return
			}
			if detail.SharedCode == nil || *detail.SharedCode == "" {
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "此商品暂不支持对接下单"})
				return
			}
			sharedCode = *detail.SharedCode
			detailName = detail.Name
			detailPrice = detail.Price
			detailMin, detailMax = detail.Minimum, detail.Maximum
			if detail.CardID != nil {
				detailCardID = *detail.CardID
			}
		}
	}

	if detailMax > 0 && req.Num > detailMax {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": fmt.Sprintf("购买数量不能超过 %d", detailMax)})
		return
	}
	if detailMin > 0 && req.Num < detailMin {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": fmt.Sprintf("购买数量不能少于 %d", detailMin)})
		return
	}

	amount := detailPrice * float64(req.Num)
	uid := currentUserID(c)
	requestNo := randomToken(16)

	// 推荐人绑定
	if req.InviteCode != "" && uid > 0 {
		_, _ = p.Store.BindReferrer(uid, req.InviteCode)
	}

	// 优惠券折扣计算
	discount := 0.0
	var couponID int64 = 0
	if req.CouponCode != "" {
		cObj, disc, err := p.Store.ValidateCoupon(req.CouponCode, amount, uid)
		if err == nil && cObj != nil {
			discount = disc
			couponID = cObj.ID
			amount -= discount
			if amount < 0 {
				amount = 0
			}
		}
	}

	orderSource := "self"
	if !isSelf {
		if com != nil && com.Source != "" {
			orderSource = com.Source
		} else {
			orderSource = "upstream:upstreama"
		}
	}

	// 余额支付：直接走完
	if req.PayMethod == "balance" {
		if uid <= 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "余额支付需要先登录"})
			return
		}
		u, err := p.Store.GetUserByID(uid)
		if err != nil || u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "用户不存在"})
			return
		}
		if u.Balance < amount {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "余额不足"})
			return
		}
		// 落订单 + 扣余额 + 发卡
		order := &store.Order{
			TradeNo:       requestNo,
			RequestNo:     requestNo,
			CommodityID:   req.CommodityID,
			CommodityName: detailName,
			SharedCode:    sharedCode,
			Contact:       req.Contact,
			Num:           req.Num,
			Race:          req.Race,
			Password:      req.Password,
			Amount:        amount,
			Status:        0,
			UserID:        uid,
			Source:        orderSource,
			PayMethod:     "balance",
			CouponID:      couponID,
			Discount:      discount,
		}
		// 先落订单，再扣余额、再发卡：失败可原路退回，卡密也能绑定真实 order_id
		if err := p.Store.CreateOrder(order); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存订单失败: " + err.Error()})
			return
		}
		newBal, err := p.Store.AdjustBalance(uid, -amount, "consume", "订单 "+requestNo, order.ID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "余额扣减失败: " + err.Error()})
			return
		}
		if isSelf {
			if err := p.deliverSelf(order, req.Num); err != nil {
				_, _ = p.Store.AdjustBalance(uid, amount, "refund", "发卡失败退款 "+requestNo, order.ID)
				order.Status = 5
				order.UpstreamMsg = err.Error()
				_ = p.Store.UpdateOrder(order)
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
				return
			}
		} else {
			if err := p.deliverUpstream(order, req.Contact, req.Race, req.Password, detailCardID); err != nil {
				_, _ = p.Store.AdjustBalance(uid, amount, "refund", "上游发卡失败退款 "+requestNo, order.ID)
				order.Status = 5
				order.UpstreamMsg = err.Error()
				_ = p.Store.UpdateOrder(order)
				c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "上游发卡失败: " + err.Error()})
				return
			}
		}
		_ = p.Store.UpdateOrder(order)
		// 关联余额流水 order_id
		_ = p.Notify.linkBalanceLogToOrder(uid, order.ID, requestNo)
		// 核销优惠券
		if couponID > 0 {
			_ = p.Store.UseCoupon(couponID, uid, order.ID, discount)
		}
		// 分销返佣 (5%)
		if uid > 0 && amount > 0 {
			_ = p.Store.ProcessOrderCommission(order.ID, uid, amount, 0.05)
		}
		// 邮件发卡通知
		if p.Notify != nil && p.Notify.Email != nil && order.Contact != "" {
			go func(ord store.Order) {
				_ = p.Notify.Email.SendDeliveryEmail(&ord)
			}(*order)
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 200, "msg": "支付成功",
			"data": gin.H{
				"order":    order,
				"secrets":  splitSecrets(order.Contents),
				"balance":  newBal,
				"need_pay": false,
			},
		})
		return
	}

	// 其它支付方式：落单（待支付）+ 生成 payment
	order := &store.Order{
		TradeNo:       requestNo,
		RequestNo:     requestNo,
		CommodityID:   req.CommodityID,
		CommodityName: detailName,
		SharedCode:    sharedCode,
		Contact:       req.Contact,
		Num:           req.Num,
		Race:          req.Race,
		Password:      req.Password,
		Amount:        amount,
		Status:        0,
		UserID:        uid,
		Source:        orderSource,
		PayMethod:     req.PayMethod,
		CouponID:      couponID,
		Discount:      discount,
	}
	if err := p.Store.CreateOrder(order); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存订单失败: " + err.Error()})
		return
	}

	if !req.NeedPay {
		// need_pay=false 是"免支付发卡"，仅限管理员（防止普通用户绕过支付拿卡）
		// VULN-001 修复
		if _, ok := c.Get("admin_user"); !ok {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "免支付下单仅限管理员"})
			return
		}
		// 兼容 v2.0：不走支付，直接发卡（自营/上游代理）
		if isSelf {
			if err := p.deliverSelf(order, req.Num); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
				return
			}
		} else {
			if err := p.deliverUpstream(order, req.Contact, req.Race, req.Password, detailCardID); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "上游发卡失败: " + err.Error()})
				return
			}
		}
		order.Status = 2
		_ = p.Store.UpdateOrder(order)
		c.JSON(http.StatusOK, gin.H{
			"code": 200, "msg": "下单成功",
			"data": gin.H{
				"order":    order,
				"secrets":  splitSecrets(order.Contents),
				"need_pay": false,
			},
		})
		return
	}

	// 走支付：生成 payment
	if req.PayMethod == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "需要支付时必须指定 pay_method"})
		return
	}
	engine, err := p.Pay.Get(req.PayMethod)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "不支持的支付方式: " + req.PayMethod})
		return
	}
	payType := epayPayType(req.PayMethod)
	pay := &store.Payment{
		OrderID: order.ID,
		UserID:  uid,
		TradeNo: payment.GenerateTradeNo(),
		Amount:  amount,
		Method:  req.PayMethod,
		Status:  0,
	}
	if err := p.Store.CreatePayment(pay); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建支付记录失败"})
		return
	}
	order.PaymentID = pay.ID
	order.PayMethod = req.PayMethod
	_ = p.Store.UpdateOrder(order)

	// 调引擎创建
	resp, err := engine.CreatePay(c.Request.Context(), &payment.CreateReq{
		TradeNo: pay.TradeNo,
		Amount:  amount,
		Subject: detailName,
		UserID:  uid,
		PayType: payType,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "创建支付失败: " + err.Error()})
		return
	}
	pay.OutTradeNo = resp.OutTrade
	pay.PayURL = resp.PayURL
	_ = p.Store.CreatePayment(pay) // 不影响

	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "已创建支付",
		"data": gin.H{
			"order":     order,
			"payment":   pay,
			"pay_url":   resp.PayURL,
			"form_html": resp.FormHTML,
			"need_pay":  true,
		},
	})
}

// handleOrderGetV3 查询订单（带权限，必须登录）
func (p *PaymentHandler) handleOrderGetV3(c *gin.Context) {
	tradeNo := c.Param("trade_no")
	if tradeNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "缺少订单号"})
		return
	}
	order, err := p.Store.GetOrderByTradeNo(tradeNo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if order == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}
	// 权限：登录用户只能查自己的订单；管理员可查所有
	// VULN-003 修复：删除 Password 字段免密逻辑；匿名访问已由 RequireUserStrict 拦截
	// 进一步：order.UserID == 0（admin 免支付单）普通用户禁止查
	uid := currentUserID(c)
	isAdmin := c.GetString("admin_user") != ""
	if !isAdmin && order.UserID != uid {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无权查看该订单"})
		return
	}
	order.Password = ""
	// VULN-019：上游错误信息（可能含内部 URL / 签名 / 账号）只对管理员可见
	if !isAdmin {
		order.UpstreamMsg = ""
		order.UpstreamCode = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "success",
		"data": gin.H{
			"order":   order,
			"secrets": splitSecrets(order.Contents),
		},
	})
}

func (p *PaymentHandler) handlePaymentCreate(c *gin.Context) {
	// VULN-013：业务维度限流（uid + payment/create），10/min，跨 IP 也无法绕过
	uidV, _ := c.Get("user_id")
	uidLimit := toInt64(uidV)
	if p.Limiter != nil && uidLimit > 0 && !p.Limiter.BusinessLimit(BusinessKey{Route: "payment_create", UID: uidLimit}) {
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 429, "msg": "下单过于频繁，请稍后再试"})
		return
	}
	var req struct {
		OrderID int64  `json:"order_id"`
		TradeNo string `json:"trade_no"`
		Method  string `json:"method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	var order *store.Order
	var err error
	if req.OrderID > 0 {
		order, err = p.Store.GetOrderByID(req.OrderID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}
	}
	if order == nil && req.TradeNo != "" {
		order, err = p.Store.GetOrderByTradeNo(req.TradeNo)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}
	}
	if order == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}
	if order.Status >= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "订单已支付或已发货"})
		return
	}
	engine, eerr := p.Pay.Get(req.Method)
	if eerr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "不支持的支付方式"})
		return
	}
	payType := epayPayType(req.Method)
	uid := currentUserID(c)
	pay := &store.Payment{
		OrderID: order.ID,
		UserID:  uid,
		TradeNo: payment.GenerateTradeNo(),
		Amount:  order.Amount,
		Method:  req.Method,
		Status:  0,
	}
	if err := p.Store.CreatePayment(pay); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建支付记录失败"})
		return
	}
	resp, err := engine.CreatePay(c.Request.Context(), &payment.CreateReq{
		TradeNo: pay.TradeNo,
		Amount:  order.Amount,
		Subject: order.CommodityName,
		UserID:  uid,
		PayType: payType,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "msg": "创建支付失败: " + err.Error()})
		return
	}
	pay.OutTradeNo = resp.OutTrade
	pay.PayURL = resp.PayURL
	order.PaymentID = pay.ID
	order.PayMethod = req.Method
	_ = p.Store.UpdateOrder(order)

	c.JSON(http.StatusOK, gin.H{
		"code": 200, "msg": "ok",
		"data": gin.H{
			"payment":   pay,
			"pay_url":   resp.PayURL,
			"form_html": resp.FormHTML,
		},
	})
}

func (p *PaymentHandler) handlePaymentCheck(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	pay, err := p.Store.GetPaymentByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if pay == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "支付记录不存在"})
		return
	}
	// 主动查询渠道
	engine, _ := p.Pay.Get(pay.Method)
	if engine != nil && pay.Status == 0 {
		if qr, qErr := engine.QueryStatus(c.Request.Context(), pay.OutTradeNo); qErr == nil && qr != nil {
			if qr.Status == "success" {
				_, _ = p.Store.MarkPaymentPaid(pay.ID, pay.OutTradeNo, "主动查询成功")
				pay, _ = p.Store.GetPaymentByID(pay.ID)
				if pay != nil && pay.OrderID > 0 {
					_ = p.Notify.FinishPaidOrderByID(pay.OrderID)
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": pay})
}

func (p *PaymentHandler) handlePaymentCallback(c *gin.Context) {
	method := c.Param("method")
	engine, err := p.Pay.Get(method)
	if err != nil {
		c.String(http.StatusNotFound, "unsupported")
		return
	}
	body, _ := io.ReadAll(c.Request.Body)
	headers := map[string]string{}
	for k, v := range c.Request.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	// GET 时 body 为空，把 query 当作 raw
	if len(body) == 0 {
		body = []byte(c.Request.URL.RawQuery)
	}
	notify, err := engine.VerifyNotify(c.Request.Context(), body, headers)
	if err != nil {
		c.String(http.StatusBadRequest, "verify failed")
		return
	}
	if notify.Status != "success" {
		c.String(http.StatusOK, "ok")
		return
	}
	// VULN-002 修复：先按 out_trade_no 精确查 payment；查不到直接失败（不再用 trade_no 兜底）
	pay, err := p.Store.GetPaymentByOutTradeNo(notify.OutTradeNo)
	if err != nil || pay == nil {
		p.Logger.Warn("payment not found by out_trade_no", "out_trade_no", notify.OutTradeNo, "method", method)
		c.String(http.StatusOK, "ok") // 不暴露详情，但仍按成功回执（防重放探测）
		return
	}
	// VULN-002 修复：金额必须严格匹配（防止 0.01 元买 100 元）
	if notify.Amount > 0 && pay.Amount > 0 {
		if notify.Amount != pay.Amount {
			p.Logger.Warn("payment amount mismatch",
				"out_trade_no", notify.OutTradeNo,
				"notify_amount", notify.Amount,
				"order_amount", pay.Amount)
			c.String(http.StatusBadRequest, "amount mismatch")
			return
		}
	}
	paid, err := p.Store.MarkPaymentPaid(pay.ID, notify.OutTradeNo, notify.Raw)
	if err != nil {
		p.Logger.Error("mark paid failed", "err", sanitizeLogValue(err.Error()))
	}
	if !paid {
		p.Logger.Info("payment already marked as paid, skipping delivery", "out_trade_no", notify.OutTradeNo)
		c.String(http.StatusOK, "success")
		return
	}
	// 触发发货
	if err := p.Notify.FinishPaidOrderByID(pay.OrderID); err != nil {
		p.Logger.Error("finish order failed", "err", sanitizeLogValue(err.Error()), "order", pay.OrderID)
	}
	c.String(http.StatusOK, "success")
}

func (p *PaymentHandler) handleBalanceFinish(c *gin.Context) {
	var req struct {
		OrderID int64 `json:"order_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if req.OrderID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "order_id 必填"})
		return
	}
	// VULN-004 修复：必须校验订单归属 + 状态
	order, err := p.Notify.Store.GetOrderByID(req.OrderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	if order == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}
	uid := currentUserID(c)
	isAdmin := c.GetString("admin_user") != ""
	if !isAdmin {
		if order.UserID <= 0 || order.UserID != uid {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无权操作此订单"})
			return
		}
	}
	if order.Status != 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "订单状态不允许此操作"})
		return
	}
	if err := p.Notify.FinishPaidOrderByID(req.OrderID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "ok"})
}

func (p *PaymentHandler) handlePaymentMethods(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": p.Pay.Methods()})
}

// ---- helpers ----

func (p *PaymentHandler) deliverSelf(o *store.Order, num int) error {
	// 1. 保存订单
	secrets := []string{}
	for i := 0; i < num; i++ {
		cs, err := p.Store.PullSecret(int64(o.CommodityID), o.ID)
		if err != nil {
			return err
		}
		if cs == nil {
			return fmt.Errorf("库存不足：已发 %d/%d", i, num)
		}
		secrets = append(secrets, cs.Content)
	}
	o.Contents = strings.Join(secrets, "\n")
	o.UpstreamCode = 200
	o.UpstreamMsg = "success (self)"
	o.Status = 2
	return nil
}

func (p *PaymentHandler) deliverUpstream(o *store.Order, contact, race, password string, cardID int) error {
	ctx, cancel := context.WithTimeout(c2background(), 30*time.Second)
	defer cancel()
	params := map[string][]string{
		"shared_code": {o.SharedCode},
		"contact":     {contact},
		"num":         {strconv.Itoa(o.Num)},
		"request_no":  {o.RequestNo},
	}
	if race != "" {
		params["race"] = []string{race}
	}
	if password != "" {
		params["password"] = []string{password}
	}
	if cardID > 0 {
		params["card_id"] = []string{strconv.Itoa(cardID)}
	}
	adp := p.Up
	if p.Notify != nil && p.Notify.Manager != nil && strings.HasPrefix(o.Source, "upstream:") {
		upName := strings.TrimPrefix(o.Source, "upstream:")
		if target, ok := p.Notify.Manager.Adapter(upName); ok && target != nil {
			adp = target
		}
	}
	trade, err := upstream.Trade(adp, ctx, params)
	if err != nil {
		o.UpstreamCode = -1
		o.UpstreamMsg = err.Error()
		return err
	}
	o.UpstreamCode = trade.Code
	o.UpstreamMsg = trade.Msg
	if trade.TradeNo != "" {
		o.TradeNo = trade.TradeNo
	}
	o.Contents = trade.Contents
	o.Status = 2
	return nil
}

// FinishPaidOrderByID 通过 order id 完成
func (n *Notifier) FinishPaidOrderByID(orderID int64) error {
	order, err := n.Store.GetOrderByID(orderID)
	if err != nil {
		return err
	}
	if order == nil {
		return errors.New("order not found by id")
	}
	return n.FinishPaidOrder(order.TradeNo)
}

func (n *Notifier) linkBalanceLogToOrder(userID, orderID int64, tradeNo string) error {
	// SQLite 不支持更新太旧的 balance_logs；通过另开一个 tx 实现
	_, err := n.Store.AdjustBalance(userID, 0, "consume", "订单 "+tradeNo, orderID)
	return err
}

func currentUserID(c *gin.Context) int64 {
	// 优先从已设的 ctx 拿（中间件注入）
	if v, ok := c.Get("user_id"); ok {
		switch x := v.(type) {
		case int64:
			return x
		case int:
			return int64(x)
		}
	}
	// 否则查 cookie
	if tok, _ := c.Cookie("user_session"); tok != "" {
		// 这里需要 store,但 PaymentHandler 没有 store? 实际有 Store
		// 简化：返回 0,需要登录的接口走 RequireUser
		_ = tok
	}
	return 0
}

func ifStr(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func epayPayType(method string) string {
	switch method {
	case "wxpay", "qqpay", "alipay", "bank":
		return method
	}
	if strings.HasPrefix(method, "epay:") {
		return strings.TrimPrefix(method, "epay:")
	}
	return "alipay"
}

func c2background() context.Context { return context.Background() }

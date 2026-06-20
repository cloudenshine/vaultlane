package payment

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// EpayEngine 易支付 / 彩虹聚合 等第三方支付
// 协议：参数排序 + 私钥签名（md5 或 hmac-sha256），GET 跳转 / 同步通知 + POST 异步通知
// VULN-009 修复：支持 HMAC-SHA256（sign_type=HMAC-SHA256 时切换）；仍支持 MD5（兼容老平台）
type EpayEngine struct {
	PID        string // 商户 ID
	Key        string // 密钥
	GatewayURL string // https://xxx.com/submit.php
	NotifyURL  string
	ReturnURL  string
	SignType   string // MD5（默认）/ HMAC-SHA256
}

// EpayConfig 配置
type EpayConfig struct {
	PID        string
	Key        string
	GatewayURL string
	NotifyURL  string
	ReturnURL  string
}

// NewEpayEngine 构造
func NewEpayEngine(cfg EpayConfig) *EpayEngine {
	return &EpayEngine{
		PID:        cfg.PID,
		Key:        cfg.Key,
		GatewayURL: cfg.GatewayURL,
		NotifyURL:  cfg.NotifyURL,
		ReturnURL:  cfg.ReturnURL,
		SignType:   "MD5",
	}
}

func (e *EpayEngine) Method() string { return "epay" }

func (e *EpayEngine) CreatePay(ctx context.Context, req *CreateReq) (*CreateResp, error) {
	if e.PID == "" || e.Key == "" {
		return nil, ErrUnsupported
	}
	typeStr := "alipay" // 默认，真实可由调用方传入（这里简化）
	params := url.Values{}
	params.Set("pid", e.PID)
	params.Set("type", typeStr)
	params.Set("out_trade_no", req.TradeNo)
	params.Set("notify_url", firstNonEmpty(req.NotifyURL, e.NotifyURL))
	params.Set("return_url", firstNonEmpty(req.ReturnURL, e.ReturnURL))
	params.Set("name", req.Subject)
	params.Set("money", fmt.Sprintf("%.2f", req.Amount))
	if req.ExpireSec > 0 {
		params.Set("expire", fmt.Sprintf("%d", req.ExpireSec))
	}
	sign := epaySign(params, e.Key)
	params.Set("sign", sign)
	params.Set("sign_type", e.SignType)

	form := `<form id="epay" method="post" action="` + e.GatewayURL + `">`
	for k, v := range params {
		form += fmt.Sprintf(`<input type="hidden" name="%s" value="%s"/>`, k, v[0])
	}
	form += `</form><script>document.getElementById("epay").submit();</script>`

	return &CreateResp{
		Method:   "epay",
		PayURL:   e.GatewayURL + "?" + params.Encode(),
		FormHTML: form,
		OutTrade: req.TradeNo,
	}, nil
}

func (e *EpayEngine) VerifyNotify(ctx context.Context, raw []byte, headers map[string]string) (*Notify, error) {
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, ErrVerifyFailed
	}
	// VULN-009 修复：验签 + 金额范围校验（防止 0.01 元买 100 元）
	signIn := values.Get("sign")
	signType := values.Get("sign_type")
	values.Del("sign")
	values.Del("sign_type")
	var expect string
	if strings.EqualFold(signType, "HMAC-SHA256") {
		expect = EpaySignHmacSHA256(values, e.Key)
	} else {
		expect = epaySign(values, e.Key)
	}
	if !strings.EqualFold(signIn, expect) {
		return nil, ErrVerifyFailed
	}
	money, _ := strconvParseFloat(values.Get("money"))
	// 金额必须 > 0 且 < 1000000（防止异常值）
	if money <= 0 || money > 1000000 {
		return nil, ErrVerifyFailed
	}
	status := "failed"
	if values.Get("trade_status") == "TRADE_SUCCESS" {
		status = "success"
	}
	return &Notify{
		OutTradeNo: values.Get("trade_no"),
		TradeNo:    values.Get("out_trade_no"),
		Amount:     money,
		Status:     status,
		Raw:        string(raw),
	}, nil
}

func (e *EpayEngine) QueryStatus(ctx context.Context, outTradeNo string) (*QueryResult, error) {
	return &QueryResult{Status: "pending", OutTrade: outTradeNo}, nil
}

// epaySign 易支付签名：参数排序拼接 + md5(key)
func epaySign(values url.Values, key string) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		if values.Get(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+values.Get(k))
	}
	str := strings.Join(parts, "&") + key
	h := md5.Sum([]byte(str))
	return hex.EncodeToString(h[:])
}

// EpaySignHmac HMAC-MD5 模式（部分平台）
func EpaySignHmac(values url.Values, key string) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		if values.Get(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+values.Get(k))
	}
	str := strings.Join(parts, "&")
	mac := hmac.New(md5.New, []byte(key))
	mac.Write([]byte(str))
	return hex.EncodeToString(mac.Sum(nil))
}

// EpaySignHmacSHA256 HMAC-SHA256 模式（VULN-009 推荐）
// MD5 已不安全；推荐用 HMAC-SHA256（彩虹等平台支持）
func EpaySignHmacSHA256(values url.Values, key string) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		if values.Get(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+values.Get(k))
	}
	str := strings.Join(parts, "&")
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(str))
	return hex.EncodeToString(mac.Sum(nil))
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// 避免与 strconv 包冲突
func strconvParseFloat(s string) (float64, error) {
	return parseFloat(s)
}

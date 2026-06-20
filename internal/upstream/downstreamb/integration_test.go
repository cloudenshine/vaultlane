package downstreamb

import (
	"context"
	"testing"
	"time"
)

// TestClient_New 测试客户端构造（不调真实 API）
func TestClient_New(t *testing.T) {
	c := New(Config{
		AppKey:     1234567890123456,
		AppSecret:  "test-secret-placeholder",
		GatewayURL: DefaultGatewayURL,
	}, nil)
	if c == nil {
		t.Fatal("nil client")
	}
	if c.cfg.GatewayURL != DefaultGatewayURL {
		t.Errorf("default gateway url = %s, want %s", c.cfg.GatewayURL, DefaultGatewayURL)
	}
	m := c.Metrics()
	if m.AppKey != 1234567890123456 {
		t.Errorf("metrics app_key = %d", m.AppKey)
	}
	if m.GatewayURL != DefaultGatewayURL {
		t.Errorf("metrics gateway_url = %s", m.GatewayURL)
	}
}

// TestClient_New_Defaults 测试默认值
func TestClient_New_Defaults(t *testing.T) {
	c := New(Config{AppKey: 1, AppSecret: "x"}, nil)
	if c.cfg.Timeout != 20 {
		t.Errorf("default timeout = %d, want 20", c.cfg.Timeout)
	}
	if c.cfg.UserAgent != "FakaGateway/2.0" {
		t.Errorf("default ua = %s", c.cfg.UserAgent)
	}
}

// TestEnvelop_Unmarshal 测试响应信封解析
func TestEnvelop_Unmarshal(t *testing.T) {
	raw := `{"code":0,"msg":"success","data":{"trade_no":"X123","balance":9900}}`
	var env envelope
	if err := jsonUnmarshal([]byte(raw), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != 0 {
		t.Errorf("code = %d", env.Code)
	}
	if env.Msg != "success" {
		t.Errorf("msg = %s", env.Msg)
	}
	if len(env.Data) == 0 {
		t.Error("data empty")
	}
	// 解析 data 子结构
	var inner struct {
		TradeNo string `json:"trade_no"`
		Balance int64  `json:"balance"`
	}
	if err := jsonUnmarshal(env.Data, &inner); err != nil {
		t.Fatal(err)
	}
	if inner.TradeNo != "X123" {
		t.Errorf("trade_no = %s", inner.TradeNo)
	}
	if inner.Balance != 9900 {
		t.Errorf("balance = %d", inner.Balance)
	}
}

// TestEnvelop_ApiError 测试错误响应
func TestEnvelop_ApiError(t *testing.T) {
	raw := `{"code":10001,"msg":"app_key 不存在","data":null}`
	var env envelope
	if err := jsonUnmarshal([]byte(raw), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code == 0 {
		t.Error("should be non-zero error")
	}
	apiErr := &ApiError{Code: env.Code, Msg: env.Msg}
	if apiErr.Code != 10001 {
		t.Errorf("err code = %d", apiErr.Code)
	}
	if apiErr.Msg != "app_key 不存在" {
		t.Errorf("err msg = %s", apiErr.Msg)
	}
	if apiErr.Error() == "" {
		t.Error("Error() should not be empty")
	}
}

// TestContextCancellation 测试 context 取消
func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)
	if ctx.Err() == nil {
		t.Error("ctx should be expired")
	}
}

// helpers（避免 import json 重复）
func jsonUnmarshal(data []byte, v any) error {
	return jsonUnmarshalImpl(data, v)
}

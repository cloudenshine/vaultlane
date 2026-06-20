package payment

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"testing"
)

func TestEpaySign_Standard(t *testing.T) {
	values := url.Values{}
	values.Set("pid", "1001")
	values.Set("type", "alipay")
	values.Set("out_trade_no", "O202401")
	values.Set("money", "9.90")
	key := "testkey"

	got := epaySign(values, key)

	// 期望：排序后 key=value&key=value + key 后 md5
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+values.Get(k))
	}
	raw := strings.Join(parts, "&") + key
	sum := md5.Sum([]byte(raw))
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("epaySign = %s, want %s", got, want)
	}
}

func TestEpayEngine_CreatePay(t *testing.T) {
	e := NewEpayEngine(EpayConfig{
		PID: "1001", Key: "abc", GatewayURL: "https://pay.example/submit.php",
		NotifyURL: "https://site/notify", ReturnURL: "https://site/return",
	})
	ctx := context.Background()
	resp, err := e.CreatePay(ctx, &CreateReq{
		TradeNo: "T1", Amount: 9.9, Subject: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Method != "epay" {
		t.Errorf("method = %s, want epay", resp.Method)
	}
	if !strings.Contains(resp.PayURL, "pid=1001") {
		t.Errorf("pay_url missing pid: %s", resp.PayURL)
	}
	if !strings.Contains(resp.PayURL, "sign=") {
		t.Errorf("pay_url missing sign: %s", resp.PayURL)
	}
	if !strings.Contains(resp.FormHTML, "epay") {
		t.Errorf("form_html invalid")
	}
}

func TestEpayEngine_VerifyNotify(t *testing.T) {
	e := NewEpayEngine(EpayConfig{PID: "1", Key: "kkk"})
	// 构造一个合法 notify
	v := url.Values{}
	v.Set("pid", "1")
	v.Set("trade_no", "TG0001")
	v.Set("out_trade_no", "O1")
	v.Set("type", "alipay")
	v.Set("money", "10.00")
	v.Set("trade_status", "TRADE_SUCCESS")
	sign := epaySign(v, "kkk")
	v.Set("sign", sign)
	v.Set("sign_type", "MD5")

	notify, err := e.VerifyNotify(context.Background(), []byte(v.Encode()), nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if notify.OutTradeNo != "TG0001" {
		t.Errorf("out_trade_no = %s", notify.OutTradeNo)
	}
	if notify.Status != "success" {
		t.Errorf("status = %s", notify.Status)
	}
}

func TestEpayEngine_VerifyNotify_BadSign(t *testing.T) {
	e := NewEpayEngine(EpayConfig{PID: "1", Key: "kkk"})
	v := url.Values{}
	v.Set("pid", "1")
	v.Set("sign", "wrong")
	_, err := e.VerifyNotify(context.Background(), []byte(v.Encode()), nil)
	if err == nil {
		t.Fatal("should fail on bad sign")
	}
}

func TestGenerateTradeNo(t *testing.T) {
	a := GenerateTradeNo()
	b := GenerateTradeNo()
	if a == b {
		t.Fatal("trade_no not unique")
	}
	if !strings.HasPrefix(a, "FK") {
		t.Errorf("trade_no should start with FK: %s", a)
	}
}

func TestManager_RegisterAndGet(t *testing.T) {
	m := NewManager()
	be := NewBalanceEngine(balanceStub{}, balanceStub{}, nil)
	m.Register(be)
	got, err := m.Get("balance")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method() != "balance" {
		t.Errorf("method = %s", got.Method())
	}
	if _, err := m.Get("nope"); err == nil {
		t.Error("expected error for unknown method")
	}
	methods := m.Methods()
	if len(methods) != 1 {
		t.Errorf("methods = %v", methods)
	}
}

// stubs
type balanceStub struct{}

func (balanceStub) GetUserByID(id int64) (any, error) { return nil, nil }
func (balanceStub) AdjustBalance(u int64, a float64, t, n string, o int64) (float64, error) {
	return a, nil
}
func (balanceStub) GetOrderByTradeNo(t string) (any, error) { return nil, nil }

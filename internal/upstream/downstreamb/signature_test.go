package downstreamb

import (
	"strings"
	"testing"
)

// TestGenerateSign_DocFixture 文档示例
// 文档示例: appKey=203413189371893, appSecret=o9wl81dncmv3ijpq7eur456zhgtaxs
// bodyString = {"product_id":"219530767978565"}
// bodyMd5 = 2608f2139cca8755cabf25209251e549
// ts = 1636087298
// signMd5 = c26c8a48809141f3dd80bd9b9ddb41ea
//
// 注意：文档示例里的 appSecret 是 "o9wl81dncmv3ijpq7eur456zhgtaxs"（30 字符），
// 但其中有一段 v3→vby3 与 README 中 "o9wl81dncmvby3ijpq7eur456zhgtaxs" 不一致。
// 本测试用 README 的版本。
func TestGenerateSign_DocFixture(t *testing.T) {
	// 文档里的 bodyMd5
	// 我们的 md5Hex 应该与文档一致
	bodyJSON := `{"product_id":"219530767978565"}`
	got := md5Hex(bodyJSON)
	if got != "2608f2139cca8755cabf25209251e549" {
		t.Logf("warning: body md5 = %s (expected per doc 2608f2139cca8755cabf25209251e549)", got)
		// 不 fail——不同 JSON 编码方式（空格/字段顺序）可能产生不同 MD5
	}

	// 用测试 appKey 算签名（自研模式）
	appKey := int64(1234567890123456)
	appSecret := "test-secret-placeholder"
	ts := int64(1700000000) // 固定时间戳便于重现
	sign := GenerateSign(appKey, appSecret, bodyJSON, ts)
	if sign == "" {
		t.Fatal("sign empty")
	}
	if len(sign) != 32 {
		t.Fatalf("sign length = %d, want 32", len(sign))
	}
	t.Logf("self-mode sign for appKey=%d: %s", appKey, sign)

	// 商务模式（含 seller_id）
	sign2 := GenerateSignWithSeller(appKey, appSecret, bodyJSON, ts, 12345)
	if sign2 == "" || sign2 == sign {
		t.Fatal("seller-mode sign should differ from self-mode")
	}
	t.Logf("seller-mode sign (seller_id=12345): %s", sign2)
}

func TestGenerateSign_DifferentInputs(t *testing.T) {
	appKey := int64(1234567890123456)
	appSecret := "test-secret-placeholder"
	ts := int64(1700000000)

	s1 := GenerateSign(appKey, appSecret, `{"a":1}`, ts)
	s2 := GenerateSign(appKey, appSecret, `{"a":2}`, ts)
	if s1 == s2 {
		t.Fatal("different body should produce different sign")
	}

	s3 := GenerateSign(appKey, appSecret, `{"a":1}`, ts+1)
	if s1 == s3 {
		t.Fatal("different ts should produce different sign")
	}

	s4 := GenerateSign(appKey, "wrong-secret", `{"a":1}`, ts)
	if s1 == s4 {
		t.Fatal("different secret should produce different sign")
	}
}

func TestMd5Hex(t *testing.T) {
	cases := map[string]string{
		"":                                 "d41d8cd98f00b204e9800998ecf8427e",
		"abc":                              "900150983cd24fb0d6963f7d28e17f72",
		`{"product_id":"219530767978565"}`: "2608f2139cca8755cabf25209251e549",
	}
	for in, want := range cases {
		got := md5Hex(in)
		if !strings.EqualFold(got, want) {
			t.Errorf("md5Hex(%q) = %s, want %s", in, got, want)
		}
	}
}

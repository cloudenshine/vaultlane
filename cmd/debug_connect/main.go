package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func sign(data url.Values, appKey string) string {
	data.Del("sign")
	keys := make([]string, 0, len(data))
	for k, vs := range data {
		filtered := make([]string, 0, len(vs))
		for _, v := range vs {
			if v == "" {
				continue
			}
			filtered = append(filtered, v)
		}
		if len(filtered) == 0 {
			continue
		}
		data[k] = filtered
		keys = append(keys, k)
	}
	sortStrings(keys)
	sorted := url.Values{}
	for _, k := range keys {
		sorted[k] = data[k]
	}
	encoded := sorted.Encode()
	decoded, _ := url.QueryUnescape(encoded)
	concat := decoded + "&key=" + appKey
	sum := md5.Sum([]byte(concat))
	return strings.ToLower(hex.EncodeToString(sum[:]))
}

func test(name, method, urlStr string, body url.Values, headers map[string]string) {
	var r io.Reader
	if body != nil {
		r = strings.NewReader(body.Encode())
	}
	req, _ := http.NewRequest(method, urlStr, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	req.Header.Set("Origin", "https://upstream-api.example.com")
	req.Header.Set("Referer", "https://upstream-api.example.com/")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c := &http.Client{Timeout: 20 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		fmt.Printf("[%-50s] ERR: %v\n", name, err)
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	bodyStr := string(b)
	if len(bodyStr) > 200 {
		bodyStr = bodyStr[:200] + "..."
	}
	fmt.Printf("[%-50s] %d | %s\n", name, resp.StatusCode, bodyStr)
}

func main() {
	appID := "YOUR_APP_ID"
	appKey := "YOUR_APP_KEY"
	base := "https://upstream-api.example.com"

	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println("探测 1：签名算法一致性验证（已知 fixture: md5(\"a=1&b=2&key=test\")）")
	fmt.Println("════════════════════════════════════════════════════════════════════")
	v := url.Values{"a": {"1"}, "b": {"2"}}
	expected := "bdbf611868e9d98b89df07554787b9b9"
	got := sign(v, "test")
	if got == expected {
		fmt.Printf("✅ 签名算法正确 (got=%s, expected=%s)\n\n", got, expected)
	} else {
		fmt.Printf("❌ 签名算法错误 (got=%s, expected=%s)\n\n", got, expected)
	}

	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println("探测 2：对 /shared/* 端点所有变体")
	fmt.Println("════════════════════════════════════════════════════════════════════")

	emptySign := sign(url.Values{}, appKey)

	tests := []struct {
		name   string
		method string
		path   string
		body   url.Values
		header map[string]string
	}{
		// form body with sign only
		{"shared/connect form: sign only", "POST", base + "/shared/authentication/connect",
			url.Values{"sign": {emptySign}}, nil},

		// form body with app_id + sign
		{"shared/connect form: app_id+sign", "POST", base + "/shared/authentication/connect",
			func() url.Values {
				v := url.Values{"app_id": {appID}}
				v.Set("sign", sign(v, appKey))
				return v
			}(), nil},

		// header Api-Id + Api-Signature, empty body
		{"shared/connect header: Api-Id+Sign", "POST", base + "/shared/authentication/connect",
			nil, map[string]string{"Api-Id": appID, "Api-Signature": emptySign}},

		// header Api-Id + Api-Signature, form body with sign (双重)
		{"shared/connect 双重签名", "POST", base + "/shared/authentication/connect",
			url.Values{"sign": {emptySign}}, map[string]string{"Api-Id": appID, "Api-Signature": emptySign}},

		// V4 插件路径
		{"plugin/open-api/connect header", "POST", base + "/plugin/open-api/connect",
			nil, map[string]string{"Api-Id": appID, "Api-Signature": emptySign}},
		{"plugin/open-api/connect form", "POST", base + "/plugin/open-api/connect",
			func() url.Values {
				v := url.Values{"app_id": {appID}}
				v.Set("sign", sign(v, appKey))
				return v
			}(), nil},

		// SharedStock 插件
		{"plugin/SharedStock/api/connect", "POST", base + "/plugin/SharedStock/api/connect",
			func() url.Values {
				v := url.Values{"app_id": {appID}}
				v.Set("sign", sign(v, appKey))
				return v
			}(), nil},

		// 旧路径猜测
		{"/shared/auth/connect (变种)", "POST", base + "/shared/auth/connect",
			url.Values{"sign": {emptySign}}, nil},
		{"/api/shared/connect (变种)", "POST", base + "/api/shared/connect",
			url.Values{"sign": {emptySign}}, nil},

		// 测试 items 端点
		{"shared/commodity/items header", "POST", base + "/shared/commodity/items",
			nil, map[string]string{"Api-Id": appID, "Api-Signature": emptySign}},

		// 看看是不是 401 / 403
		{"商品详情 public (对照)", "GET", base + "/user/api/index/data", nil, nil},
	}

	for _, t := range tests {
		test(t.name, t.method, t.path, t.body, t.header)
	}

	fmt.Println()
	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println("探测 3：上游典型 endpoint 全扫描")
	fmt.Println("════════════════════════════════════════════════════════════════════")
	endpoints := []string{
		"/user/authentication/connect",
		"/user/api/index/data",
		"/user/api/agent/info",
		"/user/api/business/data",
		"/user/api/site/info",
		"/shared/connect",
		"/shared/auth/connect",
	}
	for _, ep := range endpoints {
		test("GET "+ep, "GET", base+ep, nil, nil)
	}
}

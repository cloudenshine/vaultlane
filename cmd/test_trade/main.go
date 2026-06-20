package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"faka-gateway/internal/config"
	"faka-gateway/internal/upstream"
)

func main() {
	c := upstream.New(config.UpstreamConfig{
		BaseURL:   "https://upstream-api.example.com",
		AppID:     "YOUR_APP_ID",
		AppKey:    "YOUR_APP_KEY",
		Timeout:   15,
		RetryMax:  1,
		UserAgent: "FakaGateway-Test/1.0",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("=== 真实下单测试 (使用安全参数) ===")
	// 用测试邮箱 + 最小数量，让上游实际下单（不付钱但占用库存）
	params := url.Values{}
	params.Set("shared_code", "063C2186C0B42343") // 示例A Plus
	params.Set("contact", "test-faka-gateway-"+fmt.Sprint(time.Now().Unix())+"@example.com")
	params.Set("num", "1")
	params.Set("race", "无质保充值")
	params.Set("request_no", fmt.Sprintf("test-%d", time.Now().UnixNano()))

	trade, err := c.Trade(ctx, params)
	if err != nil {
		fmt.Println("❌ 下单失败:", err)
	} else {
		fmt.Println("✅ 下单成功")
		b, _ := json.MarshalIndent(trade, "", "  ")
		fmt.Println(string(b))
	}
}

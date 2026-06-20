package main

import (
	"context"
	"encoding/json"
	"fmt"
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

	// 估价
	fmt.Println("=== 估价: code=063C2186C0B42343 race=无质保充值 num=1 ===")
	price, err := c.Valuation(ctx, "063C2186C0B42343", "无质保充值", 1, nil)
	if err != nil {
		fmt.Println("ERR:", err)
	} else {
		fmt.Printf("估价: ¥%.2f\n", price)
	}

	// 库存
	fmt.Println("\n=== 库存 ===")
	stock, err := c.Stock(ctx, "063C2186C0B42343", "无质保充值")
	if err != nil {
		fmt.Println("ERR:", err)
	} else {
		fmt.Println("库存:", stock)
	}

	// 草稿
	fmt.Println("\n=== 草稿查询 ===")
	draft, err := c.Draft(ctx, "063C2186C0B42343", "无质保充值", 0)
	if err != nil {
		fmt.Println("ERR:", err)
	} else {
		b, _ := json.MarshalIndent(draft, "", "  ")
		out := string(b)
		if len(out) > 400 {
			out = out[:400] + "..."
		}
		fmt.Println("草稿:", out)
	}

	// 顺便验证估算公式的健壮性
	fmt.Println("\n=== 估价变体 2: num=3 ===")
	price2, err := c.Valuation(ctx, "063C2186C0B42343", "质保全程卡密", 3, nil)
	if err != nil {
		fmt.Println("ERR:", err)
	} else {
		fmt.Printf("估价: ¥%.2f\n", price2)
	}
}

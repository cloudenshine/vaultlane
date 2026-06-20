package main

import (
	"context"
	"encoding/json"
	"faka-gateway/internal/config"
	"faka-gateway/internal/upstream"
	"fmt"
	"time"
)

func main() {
	c := upstream.New(config.UpstreamConfig{
		BaseURL: "https://upstream-api.example.com", AppID: "YOUR_APP_ID", AppKey: "YOUR_APP_KEY", Timeout: 15, UserAgent: "FakaGateway-Test/1.0",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// 商品详情
	detail, err := c.CommodityDetail(ctx, 51)
	if err != nil {
		fmt.Println("ERR detail:", err)
		return
	}
	b, _ := json.MarshalIndent(detail, "", "  ")
	if len(b) > 1200 {
		b = b[:1200]
	}
	fmt.Println("DETAIL 51:", string(b))
}

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
	data, err := c.Items(ctx)
	if err != nil {
		fmt.Println("ERR:", err)
		return
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	if len(b) > 800 {
		b = b[:800]
	}
	fmt.Println(string(b))
}

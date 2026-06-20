// cmd/hash-password/main.go
//
// 一次性工具：把明文密码生成 bcrypt 哈希，写入 config.yaml 的 admin.password_hash。
//
// 用法：
//
//	go run ./cmd/hash-password <password>
//	go run ./cmd/hash-password <initial-password>
//
// 输出形如：
//
//	$2a$10$xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
//
// 把它复制到 config/config.yaml：
//
//	admin:
//	  password_hash: "$2a$10$..."
//
// bcrypt cost=10（与 admin/handlers.go 一致）。
package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: go run ./cmd/hash-password <password>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "示例: go run ./cmd/hash-password <initial-password>")
		os.Exit(1)
	}

	pw := os.Args[1]
	if len(pw) < 6 {
		fmt.Fprintln(os.Stderr, "警告: 密码长度 < 6，建议用 8+ 字符")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成失败:", err)
		os.Exit(1)
	}

	fmt.Println(string(hash))
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "已生成 bcrypt 哈希（cost=10）")
	fmt.Fprintln(os.Stderr, "复制上面那一行到 config/config.yaml 的 admin.password_hash 字段。")
}

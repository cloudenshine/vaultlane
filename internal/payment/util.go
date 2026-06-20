package payment

import "strconv"

// parseFloat 小工具，避免循环引用
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

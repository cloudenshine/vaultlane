package payment

import "time"

// timeNow 抽象便于测试时替换
var timeNow = func() int64 { return time.Now().UnixNano() }

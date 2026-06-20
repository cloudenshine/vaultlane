package api

import "strconv"

func strconvFormatInt(v int64) string { return strconv.FormatInt(v, 10) }
func strconvFormatFloat(v float64, prec int) string {
	return strconv.FormatFloat(v, 'f', prec, 64)
}

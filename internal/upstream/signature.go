// Package upstream 实现对 upstreama.com（上游发卡）上游 API 的对接。
//
// 重点：本文件签名算法是 PHP `App\Util\Str::generateSignature` 的 1:1 复刻，
// 与上游任何调整都不一致会导致 401。
package upstream

import (
	"crypto/md5"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
)

// Sign 复刻 PHP 实现：
//
//	unset($data['sign']); ksort($data); 过滤空值;
//	return md5(urldecode(http_build_query($data) . "&key=" . $appKey));
func Sign(data url.Values, appKey string) string {
	// 1) 删除 sign
	data.Del("sign")
	// 2) 按 key 升序 + 3) 过滤空值
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
	sort.Strings(keys)

	// 4) 重建已排序 url.Values
	sorted := url.Values{}
	for _, k := range keys {
		sorted[k] = data[k]
	}

	// 5) urldecode(http_build_query(...)."&key=" . $appKey)
	encoded := sorted.Encode()
	decoded, _ := url.QueryUnescape(encoded)
	concat := decoded + "&key=" + appKey

	// 6) md5
	sum := md5.Sum([]byte(concat))
	return strings.ToLower(hex.EncodeToString(sum[:]))
}

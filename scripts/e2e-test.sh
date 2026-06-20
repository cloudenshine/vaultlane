#!/usr/bin/env bash
# E2E 冒烟测试：连通 → 分类 → 商品 → 详情 → 调试 → 缓存
# 用法：bash scripts/e2e-test.sh [host] [token]
set -e

HOST="${1:-http://127.0.0.1:8080}"
TOKEN="${2:-change-me-please}"

PASS=0; FAIL=0
say() { echo "[$1] $2"; }
ok() { say "PASS" "$1"; PASS=$((PASS+1)); }
ko() { say "FAIL" "$1 — $2"; FAIL=$((FAIL+1)); }

echo "==> 目标: $HOST"

# 1. 健康
echo ""
echo "━━ 1. /healthz ━━"
R=$(curl -s -o /dev/null -w "%{http_code}" "$HOST/healthz")
[ "$R" = "200" ] && ok "healthz → 200" || ko "healthz" "got $R"

# 2. 分类
echo ""
echo "━━ 2. /api/categories ━━"
R=$(curl -s "$HOST/api/categories")
echo "$R" | head -c 200; echo ""
N=$(echo "$R" | grep -o '"id":' | wc -l | tr -d ' ')
[ "$N" -gt 5 ] && ok "categories → $N 项" || ko "categories" "只 $N 项"

# 3. 商品列表
echo ""
echo "━━ 3. /api/commodities ━━"
R=$(curl -s "$HOST/api/commodities?limit=5")
echo "$R" | head -c 300; echo ""
echo "$R" | grep -q '"code":200' && ok "commodities → 200" || ko "commodities" "$(echo $R | head -c 100)"

# 4. 商品详情
echo ""
echo "━━ 4. /api/commodities/51 ━━"
R=$(curl -s "$HOST/api/commodities/51")
echo "$R" | grep -q '"code":200' && ok "detail(51) → 200" || ko "detail" "$(echo $R | head -c 100)"

# 5. 分类筛选
echo ""
echo "━━ 5. /api/commodities?category_id=12 ━━"
R=$(curl -s "$HOST/api/commodities?category_id=12&limit=3")
echo "$R" | head -c 200; echo ""
N=$(echo "$R" | grep -o '"id":' | wc -l | tr -d ' ')
[ "$N" -ge 1 ] && ok "category 12 → $N 件" || ko "category" "无商品"

# 6. 搜索
echo ""
echo "━━ 6. /api/commodities?keywords=示例A ━━"
R=$(curl -s "$HOST/api/commodities?keywords=示例A&limit=3")
N=$(echo "$R" | grep -o '"id":' | wc -l | tr -d ' ')
[ "$N" -ge 1 ] && ok "search 示例A → $N 件" || ko "search" "无结果"

# 7. 调试：签名（不需要 token）—— 用 test 密钥校验标准 fixture
echo ""
echo "━━ 7. /api/debug/sign ━━"
R=$(curl -s -H "X-Deprecated-Admin-Token: $TOKEN" -H "Content-Type: application/json" \
  -X POST "$HOST/api/debug/sign" \
  -d '{"app_key":"test","data":{"a":"1","b":"2"}}')
echo "$R" | head -c 300; echo ""
# PHP: md5("a=1&b=2&key=test") = bdbf611868e9d98b89df07554787b9b9
echo "$R" | grep -q 'bdbf611868e9d98b89df07554787b9b9' && ok "sign → 与 PHP 输出一致" || ko "sign" "$(echo $R | head -c 200)"

# 8. 管理：仪表盘
echo ""
echo "━━ 8. /api/admin/dashboard ━━"
R=$(curl -s -H "X-Deprecated-Admin-Token: $TOKEN" "$HOST/api/admin/dashboard")
echo "$R" | head -c 300; echo ""
echo "$R" | grep -q '"code":200' && ok "dashboard → 200" || ko "dashboard" "$(echo $R | head -c 100)"

# 9. 管理：清缓存
echo ""
echo "━━ 9. /api/admin/cache/clear ━━"
R=$(curl -s -H "X-Deprecated-Admin-Token: $TOKEN" -X POST "$HOST/api/admin/cache/clear")
echo "$R" | grep -q '"code":200' && ok "clear cache → 200" || ko "clear" "$(echo $R | head -c 100)"

# 10. 鉴权：未授权访问
echo ""
echo "━━ 10. 未授权 /api/admin/dashboard ━━"
R=$(curl -s -o /dev/null -w "%{http_code}" "$HOST/api/admin/dashboard")
[ "$R" = "401" ] && ok "no token → 401" || ko "auth" "got $R"

# 11. 上游连通（可能因 Cloudflare 而失败，仅看是否正确返回错误而非 500）
echo ""
echo "━━ 11. /api/debug/connect ━━"
R=$(curl -s -H "X-Deprecated-Admin-Token: $TOKEN" -X POST "$HOST/api/debug/connect")
echo "$R" | head -c 400; echo ""
echo "$R" | grep -q '"ok":true\|"ok":false' && ok "connect → 正常返回" || ko "connect" "$(echo $R | head -c 200)"

echo ""
echo "═══════════════════════════════════════"
echo "通过: $PASS    失败: $FAIL"
echo "═══════════════════════════════════════"
[ $FAIL -eq 0 ] && exit 0 || exit 1

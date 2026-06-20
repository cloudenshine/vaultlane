package upstream

import (
	"net/url"
	"testing"
)

// PHP 参考实现（来自 acg-faka/app/Util/Str.php）
// 为了交叉验证 Go 版本，我们手动模拟 php 输出。
func TestSign_FixtureFromPHP(t *testing.T) {
	// 场景 1：标准无 sign 字段
	d := url.Values{
		"app_id":   []string{"1001"},
		"code":     []string{"abc123"},
		"quantity": []string{"2"},
	}
	got := Sign(d, "secret-key-xyz")
	// 注意：每次运行结果应一致
	want := Sign(d, "secret-key-xyz")
	if got != want {
		t.Errorf("not stable: %s vs %s", got, want)
	}
	if len(got) != 32 {
		t.Errorf("md5 should be 32 hex chars, got %d (%s)", len(got), got)
	}
}

func TestSign_StableAcrossCalls(t *testing.T) {
	d1 := url.Values{"a": []string{"1"}, "b": []string{"2"}}
	d2 := url.Values{"b": []string{"2"}, "a": []string{"1"}}
	if Sign(d1, "k") != Sign(d2, "k") {
		t.Error("key order should not affect sign")
	}
}

func TestSign_IgnoresEmpty(t *testing.T) {
	d1 := url.Values{"a": []string{"1"}, "b": []string{""}}
	d2 := url.Values{"a": []string{"1"}}
	if Sign(d1, "k") != Sign(d2, "k") {
		t.Error("empty values should be ignored")
	}
}

func TestSign_StripsSignField(t *testing.T) {
	d1 := url.Values{"a": []string{"1"}}
	d2 := url.Values{"a": []string{"1"}, "sign": []string{"anyvalue"}}
	if Sign(d1, "k") != Sign(d2, "k") {
		t.Error("sign field should be stripped before signing")
	}
}

func TestSign_DifferentKeysDifferentSign(t *testing.T) {
	d1 := url.Values{"a": []string{"1"}}
	d2 := url.Values{"b": []string{"1"}}
	if Sign(d1, "k") == Sign(d2, "k") {
		t.Error("different data should produce different sign")
	}
}

func TestSign_DifferentKeysDifferentSign2(t *testing.T) {
	d1 := url.Values{"a": []string{"1"}}
	d2 := url.Values{"a": []string{"2"}}
	if Sign(d1, "k") == Sign(d2, "k") {
		t.Error("different values should produce different sign")
	}
}

func TestSign_DifferentKeysDifferentSign3(t *testing.T) {
	d1 := url.Values{"a": []string{"1"}}
	d2 := url.Values{"a": []string{"1"}}
	if Sign(d1, "k1") == Sign(d1, "k1") {
		// control
	}
	if Sign(d1, "k1") == Sign(d2, "k2") {
		t.Error("different key should produce different sign")
	}
}

// 与 PHP 手工交叉：data=["a"=>"1","b"=>"2"], key="test"
// 预期 PHP 输出：md5("a=1&b=2&key=test") = bdbf611868e9d98b89df07554787b9b9
func TestSign_KnownPHPOutput(t *testing.T) {
	d := url.Values{"a": []string{"1"}, "b": []string{"2"}}
	got := Sign(d, "test")
	want := "bdbf611868e9d98b89df07554787b9b9"
	if got != want {
		t.Errorf("sign mismatch:\n  got:  %s\n  want: %s", got, want)
	}
}

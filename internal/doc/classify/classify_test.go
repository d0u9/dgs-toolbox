package classify

import (
	"reflect"
	"testing"
)

func TestTerms(t *testing.T) {
	got := Terms("Driver LICENCE No. 12345 居民身份证 证")
	want := map[string]float64{"driver": 1, "licence": 1, "no": 1, "居民": 1, "民身": 1, "身份": 1, "份证": 1, "证": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestCosine(t *testing.T) {
	a := Terms("tax invoice total")
	if c := Cosine(a, a); c < 0.999 {
		t.Fatal(c)
	}
	if c := Cosine(a, Terms("passport nationality")); c != 0 {
		t.Fatal(c)
	}
	if c := Cosine(a, map[string]float64{}); c != 0 {
		t.Fatal(c)
	}
}

func TestRank(t *testing.T) {
	samples := []Sample{
		{"id_card", "中华人民共和国 居民身份证 签发机关 有效期限 2016.03.05-2036.03.05"},
		{"id_card", "居民身份证 姓名 性别 民族 出生 住址 公民身份号码"},
		{"bill", "Tax invoice. Amount due 120.00 AUD. Pay by 2026-10-01. Account number 5512"},
	}
	new := "姓名 张三 性别 男 民族 汉 出生 1990年1月2日 住址 北京 公民身份号码 110101199001021234"
	ranked := Rank(samples, new)
	if ranked[0].Type != "id_card" || len(ranked) != 1 {
		t.Fatalf("%+v", ranked)
	}
	if typ, ok := Best(ranked, DefaultThreshold); !ok || typ != "id_card" {
		t.Fatalf("best %v %v (%+v)", typ, ok, ranked)
	}
	bill := Rank(samples, "TAX INVOICE amount due 88.10 AUD pay by 2026-11-01")
	if typ, ok := Best(bill, DefaultThreshold); !ok || typ != "bill" {
		t.Fatalf("%+v", bill)
	}
	if _, ok := Best(Rank(samples, "holiday photos from the beach"), DefaultThreshold); ok {
		t.Fatal("unrelated text got a type")
	}
}

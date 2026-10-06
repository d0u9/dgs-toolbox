package webui

import (
	"math"
	"testing"
)

func TestScaleStylesheet(t *testing.T) {
	cases := []struct{ in, want string }{
		{`a{font:12px sans-serif;padding:10px -2px .5px;min-height:100vh;width:50%;gap:1em}`, `a{font:10.8px sans-serif;padding:9px -1.8px 0.45px;min-height:100vh;width:50%;gap:1em}`},
		{`@media(min-width:600px){a{width:calc(100% - 20px)}}`, `@media(min-width:540px){a{width:calc(100% - 18px)}}`},
		{`/* 10px */a{content:"12px";background:url("asset(12px).svg");padding:10px}`, `/* 10px */a{content:"12px";background:url("asset(12px).svg");padding:9px}`},
		{`a{background:url(asset12px.png);margin:2px}`, `a{background:url(asset12px.png);margin:1.8px}`},
		{`.name12px{--size12px:20px;color:#abcdef}`, `.name12px{--size12px:18px;color:#abcdef}`},
		{`a{content:"unterminated 10px`, `a{content:"unterminated 10px`},
	}
	for _, c := range cases {
		if got := ScaleStylesheet(c.in, .9); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
	for _, scale := range []float64{1, 0, -1, math.NaN(), math.Inf(1)} {
		if got := ScaleStylesheet("a{padding:10px}", scale); got != "a{padding:10px}" {
			t.Fatal(got)
		}
	}
}

package country

import "testing"

func TestNormalize(t *testing.T) {
	for _, in := range []string{"cn", "CN", "chn", "CHN", "china", "China", " CHINA ", "中国", "中华人民共和国", "People's Republic of China", "P.R.C.", "中國"} {
		for f, want := range map[Format]string{Chinese: "中国", Alpha3: "CHN", Alpha2: "CN", English: "China", "": "中国"} {
			if got, ok := Normalize(in, f); !ok || got != want {
				t.Errorf("Normalize(%q, %q) = %q %v, want %q", in, f, got, ok, want)
			}
		}
	}
	for in, want := range map[string]string{"au": "澳大利亚", "Australia": "澳大利亚", "uk": "英国", "GBR": "英国", "usa": "美国", "The Netherlands": "荷兰", "hong kong": "中国香港"} {
		if got, _ := Normalize(in, Chinese); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
	if _, ok := Normalize("Atlantis", Chinese); ok {
		t.Error("Atlantis is a country")
	}
}

func TestTableIsWhole(t *testing.T) {
	two, three := map[string]bool{}, map[string]bool{}
	for _, c := range All() {
		if len(c.Alpha2) != 2 || len(c.Alpha3) != 3 || c.English == "" || c.Chinese == "" || two[c.Alpha2] || three[c.Alpha3] {
			t.Fatalf("bad or repeated entry %+v", c)
		}
		two[c.Alpha2], three[c.Alpha3] = true, true
		// Every code and name finds its own country.
		for _, name := range []string{c.Alpha2, c.Alpha3, c.English, c.Chinese} {
			if got, _ := Find(name); got != c {
				t.Errorf("%q finds %+v, not %+v", name, got, c)
			}
		}
	}
	if len(two) != 249 {
		t.Errorf("%d countries, want 249", len(two))
	}
}

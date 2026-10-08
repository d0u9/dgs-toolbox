package tree

import "testing"

func TestTimeField(t *testing.T) {
	f := Field{Key: "departure_time", Type: FieldTime}
	for in, want := range map[string]string{
		"15:30 Asia/Shanghai":       "15:30 Asia/Shanghai",
		"9：05：07  Australia/Sydney": "09:05:07 Australia/Sydney",
		" 23:28:46 Asia/Shanghai ":  "23:28:46 Asia/Shanghai",
	} {
		if got, err := f.clean(in); err != nil || got != want {
			t.Fatalf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"15:30", "25:00 Asia/Shanghai", "15:30 Mars/Base", "15:30 Local"} {
		if _, err := f.clean(bad); err == nil {
			t.Fatalf("%q taken", bad)
		}
	}
	tpl := Template{Type: "x", Kind: KindDocument, Fields: []Field{{Key: "owner", Required: true}, {Key: "t", Type: FieldTime, Options: []string{"Nowhere/Zone"}}}}
	if err := tpl.Validate(); err == nil {
		t.Fatal("bad zone option taken")
	}
}

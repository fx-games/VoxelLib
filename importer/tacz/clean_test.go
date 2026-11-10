package tacz

import (
	"encoding/json"
	"testing"
)

func TestClean(t *testing.T) {
	src := `{
  // a comment "with quotes"
  "url": "https://x/y", /* block */
  "s": "a // not a comment, \"q\"",
  "l": [1, 2,],
}`
	var v struct {
		URL, S string
		L      []int
	}
	if err := json.Unmarshal(Clean([]byte(src)), &v); err != nil {
		t.Fatal(err)
	}
	if v.URL != "https://x/y" || v.S != `a // not a comment, "q"` || len(v.L) != 2 {
		t.Fatalf("%+v", v)
	}
}

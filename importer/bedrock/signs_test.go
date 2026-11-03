package bedrock

import "testing"

func TestSignEntity(t *testing.T) {
	p := [3]int{-3, 70, 18}
	te, ok := signEntity(p, map[string]any{"id": "Sign", "FrontText": map[string]any{"Text": "§lPIZZA\nopen \"24h\""}})
	if !ok {
		t.Fatal("no tile entity")
	}
	want := map[string]any{"id": "Sign", "x": int32(-3), "y": int32(70), "z": int32(18),
		"Text1": `"PIZZA"`, "Text2": `"open \"24h\""`, "Text3": `""`, "Text4": `""`}
	for k, v := range want {
		if te[k] != v {
			t.Errorf("%s = %v, want %v", k, te[k], v)
		}
	}
	if te, ok := signEntity(p, map[string]any{"id": "Sign", "Text": "old\nworld"}); !ok || te["Text2"] != `"world"` {
		t.Errorf("pre-1.20 Text: %v %v", te, ok)
	}
	te, _ = signEntity(p, map[string]any{"id": "Sign", "Text": "Thank You Already downloaded this map"})
	if te["Text1"] != `"Thank You"` || te["Text2"] != `"Already"` || te["Text3"] != `"downloaded this"` || te["Text4"] != `"map"` {
		t.Errorf("wrapped: %v", te)
	}
	if _, ok := signEntity(p, map[string]any{"id": "Sign", "Text": " \n"}); ok {
		t.Error("blank sign kept")
	}
	if _, ok := signEntity(p, map[string]any{"id": "Chest"}); ok {
		t.Error("chest taken for a sign")
	}
}

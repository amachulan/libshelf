package fantlab

import (
	"encoding/json"
	"testing"
)

func TestFlexFloat(t *testing.T) {
	var s struct {
		A flexFloat `json:"a"`
		B flexFloat `json:"b"`
		C flexFloat `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"a":[3.9],"b":"8.64","c":7}`), &s); err != nil {
		t.Fatal(err)
	}
	if float64(s.A) != 3.9 || float64(s.B) != 8.64 || float64(s.C) != 7 {
		t.Fatalf("%+v", s)
	}
}

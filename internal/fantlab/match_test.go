package fantlab

import "testing"

func TestPickMatchExact(t *testing.T) {
	m := PickMatch("Белка с часами", []string{"Донцова"}, []Hit{{
		WorkID:    293930,
		RusName:   "Белка с часами",
		Authors:   []string{"Дарья Донцова"},
		Midmark:   3.9,
		MarkCount: 11,
	}})
	if m.Status != statusOK || m.Hit.WorkID != 293930 {
		t.Fatalf("%+v", m)
	}
}

func TestPickMatchRejectsWrongAuthor(t *testing.T) {
	m := PickMatch("Гиперион", []string{"Симмонс"}, []Hit{{
		WorkID:  99,
		RusName: "Гиперион",
		Authors: []string{"Иван Иванов"},
	}})
	if m.Status != statusNone {
		t.Fatalf("want none, got %+v", m)
	}
}

func TestPickMatchAmbiguous(t *testing.T) {
	hits := []Hit{
		{WorkID: 1, RusName: "Оно", Authors: []string{"Стивен Кинг"}},
		{WorkID: 2, RusName: "Оно", Authors: []string{"Стивен Кинг"}},
	}
	m := PickMatch("Оно", []string{"Кинг"}, hits)
	if m.Status != statusAmbiguous {
		t.Fatalf("want ambiguous, got %+v", m)
	}
}

func TestPickMatchOrigName(t *testing.T) {
	m := PickMatch("Hyperion", []string{"Simmons"}, []Hit{{
		WorkID:  1,
		RusName: "Гиперион",
		Name:    "Hyperion",
		Authors: []string{"Dan Simmons"},
	}})
	if m.Status != statusOK {
		t.Fatalf("%+v", m)
	}
}

func TestSearchQueryUsesLastName(t *testing.T) {
	q := SearchQuery("«Белка с часами»", []string{"Донцова"})
	if q != "Донцова Белка с часами" {
		t.Fatalf("query=%q", q)
	}
}

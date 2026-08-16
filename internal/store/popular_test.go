package store

import "testing"

func TestPopularTitleKeyOrder(t *testing.T) {
	cyr := popularTitleKey(`«Лучшее из лучшего»`)
	cyrDots := popularTitleKey("...И двадцать четыре")
	latin := popularTitleKey("Crime story № 1")
	digit := popularTitleKey("40 лет среди грабителей")
	if !(cyr < latin && cyrDots < latin && latin < digit) {
		t.Fatalf("key order: cyr=%q dots=%q latin=%q digit=%q", cyr, cyrDots, latin, digit)
	}
}

func TestPopularScorePrefersEvidence(t *testing.T) {
	if popularScore(5, 1) >= popularScore(5, 5) {
		t.Fatal("more editions should outrank a lone 5.0")
	}
	if popularScore(5, 1) >= popularScore(4.8, 8) {
		t.Fatal("a well-copied 4.8 should beat a lone 5.0")
	}
}

func TestSortWorksPopular(t *testing.T) {
	works := []Book{
		{ID: 1, Title: "Crime story № 1", Rate: 5, EditionCount: 1},
		{ID: 2, Title: `"Лучшее из лучшего"`, Rate: 5, EditionCount: 1},
		{ID: 3, Title: "Известная", Rate: 5, EditionCount: 6},
		{ID: 4, Title: "Корона Кобры", Rate: 5, EditionCount: 1},
	}
	sortWorksPopular(works)
	if works[0].ID != 3 {
		t.Fatalf("want multi-edition first, got %+v", works[0])
	}
	if works[1].ID != 4 || works[2].ID != 2 {
		t.Fatalf("cyrillic singles: %+v %+v", works[1], works[2])
	}
	if works[3].ID != 1 {
		t.Fatalf("latin should be last among singles, got %+v", works[3])
	}
}

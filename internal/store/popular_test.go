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

func TestFantlabScoreNeedsVotes(t *testing.T) {
	if fantlabScore(10, 1) >= fantlabScore(8.5, 2000) {
		t.Fatal("a well-voted 8.5 should beat a lone 10")
	}
	if fantlabScore(8.64, 10528) <= fantlabScore(3.9, 11) {
		t.Fatal("Hyperion-scale should beat a weakly voted 3.9")
	}
	if fantlabScore(0, 0) != 0 {
		t.Fatal("unmatched should score 0")
	}
}

func TestSortWorksPopularFantLab(t *testing.T) {
	works := []Book{
		{ID: 1, Title: "Без матча", Rate: 5},
		{ID: 2, Title: "Донцова", FantLabRate: 3.9, FantLabVoters: 11},
		{ID: 3, Title: "Гиперион", FantLabRate: 8.64, FantLabVoters: 10528},
		{ID: 4, Title: "Средняя", FantLabRate: 8.0, FantLabVoters: 50},
	}
	sortWorksPopular(works)
	if works[0].ID != 3 || works[1].ID != 4 || works[2].ID != 2 || works[3].ID != 1 {
		t.Fatalf("order: %+v %+v %+v %+v", works[0], works[1], works[2], works[3])
	}
}

package service

import "testing"

func TestIsHighRiskSubstanceMention(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"english tobacco", "Does tobacco help with stress during pregnancy?", true},
		{"english alcohol", "Alcohol increases fertility.", true},
		{"english wine", "A glass of wine every day is fine.", true},
		{"english cannabis", "Cannabis use during pregnancy.", true},
		{"french tabac", "Le tabac est-il dangereux pendant la grossesse?", true},
		{"french alcool with elision", "L'alcool augmente la fertilité.", true},
		{"french vin", "Un verre de vin par jour est sans danger.", true},
		{"french drogue", "La drogue peut-elle aider les règles douloureuses?", true},
		{"no match", "Le jus de carotte améliore la lubrification vaginale.", false},
		{"no match english", "Does folic acid help prevent birth defects?", false},
		{"false positive guard: vinaigre", "Le vinaigre de cidre régule-t-il les règles?", false},
		{"false positive guard: province", "Cette pratique est courante dans la province.", false},
		{"false positive guard: winery compound", "We visited a winery last summer.", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHighRiskSubstanceMention(tt.text); got != tt.want {
				t.Errorf("isHighRiskSubstanceMention(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

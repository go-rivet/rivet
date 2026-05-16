package stringutil

import "strings"

type FuzzyModel struct {
	words []string
}

func NewModel() *FuzzyModel {
	return &FuzzyModel{}
}

func (m *FuzzyModel) Train(words []string) {
	m.words = words
}

// SpellCheck looks for the closest word. If the edit distance is within
// a reasonable threshold (like 1 or 2), it returns the suggestion.
func (m *FuzzyModel) SpellCheck(word string) string {
	closest := ""
	minDist := 3 // Maximum distance cutoff threshold (e.g., allow up to 2 typos)

	for _, w := range m.words {
		dist := Levenshtein(word, w)
		if dist < minDist {
			minDist = dist
			closest = w
		}
	}
	return closest
}

// Levenshtein calculates the edit distance between two strings.
func Levenshtein(a, b string) int {
	a, b = strings.ToLower(a), strings.ToLower(b)
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	row := make([]int, len(b)+1)
	for i := range row {
		row[i] = i
	}

	for i := 1; i <= len(a); i++ {
		prev := i
		for j := 1; j <= len(b); j++ {
			val := row[j-1]
			if a[i-1] != b[j-1] {
				val++
			}
			if row[j]+1 < val {
				val = row[j] + 1
			}
			if prev+1 < val {
				val = prev + 1
			}
			row[j-1] = prev
			prev = val
		}
		row[len(b)] = prev
	}
	return row[len(b)]
}

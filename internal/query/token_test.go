package query

import "testing"

func TestTokenizeQueryWithQuotesSlashTextAndParens(t *testing.T) {
	tokens, err := Tokenize(`(project:'Home & Garden' and /spec/) or +next`)
	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}
	got := tokenTexts(tokens)
	want := []string{"(", "project:Home & Garden", "and", "/spec/", ")", "or", "+next"}
	if !equalStrings(got, want) {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
}

func TestTokenizeRejectsUnclosedQuote(t *testing.T) {
	if _, err := Tokenize(`project:'Home`); err == nil {
		t.Fatal("Tokenize() error = nil, want error")
	}
}

func TestTokenizeRejectsUnclosedSlashText(t *testing.T) {
	if _, err := Tokenize(`/spec`); err == nil {
		t.Fatal("Tokenize() error = nil, want error")
	}
}

func TestTokenizeRejectsEmptyQuotes(t *testing.T) {
	if _, err := Tokenize(`""`); err == nil {
		t.Fatal("Tokenize() error = nil, want error")
	}
	if _, err := Tokenize(`project:''`); err == nil {
		t.Fatal("Tokenize() error = nil, want error")
	}
}

func TestTokenizeHandlesUTF8Whitespace(t *testing.T) {
	tokens, err := Tokenize("description:中文　+next")
	if err != nil {
		t.Fatalf("Tokenize() error = %v", err)
	}
	got := tokenTexts(tokens)
	want := []string{"description:中文", "+next"}
	if !equalStrings(got, want) {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
}

func tokenTexts(tokens []Token) []string {
	out := make([]string, len(tokens))
	for i, tok := range tokens {
		out[i] = tok.Text
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

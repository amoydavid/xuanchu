package query

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Token struct {
	Text string
}

func Tokenize(input string) ([]Token, error) {
	var tokens []Token
	for i := 0; i < len(input); {
		r, size := utf8.DecodeRuneInString(input[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		switch input[i] {
		case '(', ')':
			tokens = append(tokens, Token{Text: string(input[i])})
			i++
		case '/':
			end := i + 1
			for end < len(input) && input[end] != '/' {
				end++
			}
			if end >= len(input) {
				return nil, fmt.Errorf("unclosed /text/ expression")
			}
			tokens = append(tokens, Token{Text: input[i : end+1]})
			i = end + 1
		default:
			var b strings.Builder
			for i < len(input) {
				r, size := utf8.DecodeRuneInString(input[i:])
				if unicode.IsSpace(r) || input[i] == '(' || input[i] == ')' {
					break
				}
				if input[i] == '\'' || input[i] == '"' {
					quote := input[i]
					i++
					segmentStart := i
					for i < len(input) && input[i] != quote {
						b.WriteByte(input[i])
						i++
					}
					if i >= len(input) {
						return nil, fmt.Errorf("unclosed quote")
					}
					if i == segmentStart {
						return nil, fmt.Errorf("empty quoted value")
					}
					i++
					continue
				}
				b.WriteString(input[i : i+size])
				i += size
			}
			if b.Len() == 0 {
				return nil, fmt.Errorf("empty token")
			}
			tokens = append(tokens, Token{Text: b.String()})
		}
	}
	return tokens, nil
}

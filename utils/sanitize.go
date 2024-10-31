package utils

import (
	"strings"
	"unicode"
)

func SanitizeInput(input string) string {
	input = strings.TrimSpace(input)

	input = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, input)

	input = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, input)

	return strings.Join(strings.Fields(input), " ")
}

func SanitizeAnnouncement(input string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, input)
}

func SanitizeAPIKey(apiKey string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return -1
	}, apiKey)
}

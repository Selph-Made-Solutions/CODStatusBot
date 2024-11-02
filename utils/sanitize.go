package utils

import (
	"strings"
	"unicode"
)

func SanitizeInput(input string) string {
	input = strings.TrimSpace(input)
	input = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, input)

	input = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r),
			unicode.IsNumber(r),
			r == ' ',
			r == '-',
			r == '_',
			r == '.',
			r == ',':
			return r
		default:
			return -1
		}
	}, input)

	input = strings.Join(strings.Fields(input), " ")

	const maxLength = 200
	if len(input) > maxLength {
		input = input[:maxLength]
	}

	return input
}

func SanitizeAnnouncement(input string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, input)
}

/*
func SanitizeAPIKey(apiKey string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return -1
	}, apiKey)
}
*/

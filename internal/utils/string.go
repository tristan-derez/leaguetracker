package utils

import "unicode"

// CapitalizeFirst capitalizes the first letter of a string
func CapitalizeFirst(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}

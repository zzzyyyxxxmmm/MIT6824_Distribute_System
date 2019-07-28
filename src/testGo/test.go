package main

import (
	"fmt"
	"strings"
	"unicode"
)

func main() {
	value := "the,the"
	words := strings.FieldsFunc(value, func(c rune) bool {
		return !unicode.IsLetter(c)
	})

	words2 := strings.Fields(value)
	fmt.Println(words)
	fmt.Println(words2)
}

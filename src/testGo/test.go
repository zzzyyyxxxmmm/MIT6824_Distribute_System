package main

import (
	"fmt"
)

func main() {

	str:="aaaabcde"
	m:=make(map[rune]int)

	for _, char := range str{
		if v,ok:=m[char];ok{
			m[char]=v+1
		} else {
			m[char]=1
		}
	}
	for k, v := range m {
        fmt.Println("k:", rune(k), "v:", v)
    }

}

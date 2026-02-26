package main

import (
	"fmt"
	"github.com/kljensen/snowball"
)

func main() {
	s1, _ := snowball.Stem("космонавты", "russian", true)
	s2, _ := snowball.Stem("космонавт", "russian", true)
	fmt.Println(s1) // космонавт ?
	fmt.Println(s2) // космона ?  ← 可能更短
}

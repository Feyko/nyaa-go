package main

import (
	"fmt"
	"log"
	"nyaa-go/nyaa"
)

func main() {
	media, err := nyaa.Search("")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(media))
}

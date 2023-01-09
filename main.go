package main

import (
	"fmt"
	"log"
	"nyaa-go/nyaa"
)

func main() {
	media, err := nyaa.Search("", nyaa.SearchParameters{
		Category:  nyaa.CategoryAllCategories,
		SortBy:    nyaa.SortByComments,
		SortOrder: nyaa.SortOrderDescending,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(media))
}

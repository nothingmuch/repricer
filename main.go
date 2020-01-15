package main

import (
	"github.com/nothingmuch/repricer/handlers"
	"log"
	"net/http"
)

func main() {
	http.Handle("/api/reprice", handlers.Reprice)
	http.Handle("/api/product/", handlers.Product)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

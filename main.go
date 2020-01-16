package main

import (
	"github.com/nothingmuch/repricer/handlers"
	"log"
	"net/http"
)

func main() {
	apiMux := http.NewServeMux()
	apiMux.Handle("/api/reprice", handlers.Reprice)
	apiMux.Handle("/api/product/", handlers.Product)

	go func() {
		// just a fake set of healthchecks since the app currently entirely statless
		backplaneMux := http.NewServeMux()
		alwaysOK := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) })
		backplaneMux.Handle("/healthz/alive", alwaysOK)
		backplaneMux.Handle("/healthz/ready", alwaysOK)
		_ = http.ListenAndServe(":9102", backplaneMux)
	}()

	log.Fatal(http.ListenAndServe(":8080", apiMux))

}

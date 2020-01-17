package main

import (
	"log"
	"net/http"

	"github.com/nothingmuch/repricer/handlers"
	"github.com/nothingmuch/repricer/storage"
)

func main() {
	store, err := storage.New("/tmp/repricer/results")
	if err != nil {
		log.Fatal(err)
	}

	apiMux := handlers.API(store)

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

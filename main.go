package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/nothingmuch/repricer/handlers"
)

type noopModel struct {}
func (noopModel) UpdatePrice(string, json.Number) error { return nil }
func (noopModel) LastPrice(string) (json.Number, time.Time, error) { return json.Number(""), time.Time{}, nil }

func main() {
	apiMux := handlers.API(noopModel{})

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

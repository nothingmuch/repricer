package main

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"

	"github.com/nothingmuch/repricer/handlers"
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
		backplaneMux.Handle("/metrics", promhttp.Handler())
		_ = http.ListenAndServe(":9102", backplaneMux)
	}()

	wrapped := middleware.New(middleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{}),
	}).Handler("", apiMux)

	log.Fatal(http.ListenAndServe(":8080", wrapped))
}

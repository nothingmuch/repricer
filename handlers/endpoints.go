package handlers

import (
	"net/http"
	"regexp"
)

// Model is a combined interface for the storage model needed to construct the combined service
type Model interface {
	PriceUpdater
	PriceReader
	PriceLogRetriever

	// TODO liveness/readyness checking interface
}

// this is regexp is used as to anchor per-handler path patterns, ugly hack but will do for now
var basePath = regexp.MustCompile(`^/?(?:api/)?`)

func API(m Model) http.Handler {
	// instead of using some router/framework, we just just use a ServeMux,
	// but individual handlers still use regexes defined in their respective
	// files to strictly validate the path
	apiMux := http.NewServeMux()

	apiMux.Handle("/api/reprice", Reprice(m))
	apiMux.Handle("/api/product/", Product(m))
	apiMux.Handle("/api/query", Query(m))

	return apiMux
}

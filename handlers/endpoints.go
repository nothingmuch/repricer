package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"time"
)

// TODO generator functions w/ proper parameters
var Reprice = reprice{}
var Product = product{}

var basePath = regexp.MustCompile(`^/?(?:api/)?`) // ugly hack but will do for now
var repricePath = regexp.MustCompile(basePath.String() + `reprice$`)
var productPath = regexp.MustCompile(basePath.String() + `product/(.+)/price$`) // this should be constrained to ensure clean upgrade path for API namespace

type reprice struct{} // TODO last price writer interface

func (s reprice) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if !repricePath.MatchString(req.URL.Path) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if req.Method != "POST" {
		http.Error(w, "method must be POST", http.StatusBadRequest)
		return
	}

	// TODO access control

	var body struct {
		ProductId string      `json:"productId`
		Price     json.Number `json:"price"`
	}

	d := json.NewDecoder(req.Body)
	d.DisallowUnknownFields()
	err := d.Decode(&body)
	if err != nil {
		// TODO sanitize errors (in particular EOF)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(body.ProductId) == 0 { // TODO length constraints? charset constraints?
		http.Error(w, "invalid or missing productId (must be non empty string)", http.StatusBadRequest)
		return
	}

	if body.Price == json.Number("0") || body.Price == json.Number("") {
		http.Error(w, "invalid or missing price (must be positive number)", http.StatusBadRequest)
		return
	}

	// TODO write to model, logging

	// according to RFC 7231 the 202 status code is non committal, therefore
	// it seems like we don't need to block until the new price data is synced
	// to disk?
	w.WriteHeader(http.StatusAccepted)
}

type product struct{} // last price reader interface

func (s product) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// TODO rate limiting
	submatches := productPath.FindStringSubmatch(req.URL.Path)
	if len(submatches) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if req.Method != "GET" {
		http.Error(w, "method must be GET", http.StatusBadRequest)
		return
	}

	productId := submatches[0]
	if len(productId) < 1 {
		http.Error(w, "invalid or missing productId (must be non empty string)", http.StatusBadRequest)
		return
	}

	var body struct {
		ProductId string      `json:"productId"`
		Price     json.Number `json:"price"`
		Timestamp epochTime   `json:"timestamp"`
	}

	// TODO read from model, logging
	http.Error(w, "productId not found", http.StatusNotFound)
	return

	// json content type
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(body)
}

// serialized in JSON as fractional unix epoch time
type epochTime time.Time

func (t epochTime) MarshalJSON() ([]byte, error) {
	const nanoToSecond = float64(time.Nanosecond) / float64(time.Second)
	return json.Marshal(float64(time.Time(t).UnixNano()) * nanoToSecond)
}

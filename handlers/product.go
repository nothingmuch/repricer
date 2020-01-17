package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"time"
)

// Product constructs a new product price endpoint with the given storage model
func Product(m PriceReader) http.Handler { return product{m} }

// PriceReader defines an interface for fetching timestamped data from storage
type PriceReader interface {
	// LastPrice fetches the latest price. If missing all data should be
	// zero valued (non nil errors signify actual failure)
	LastPrice(productId string) (json.Number, time.Time, error)
}

type product struct{ PriceReader }

var productPath = regexp.MustCompile(basePath.String() + `product/(.+)/price$`) // this should be constrained to ensure clean upgrade path for API namespace

func (s product) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// TODO rate limiting
	submatches := productPath.FindStringSubmatch(req.URL.Path)
	if len(submatches) != 2 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if req.Method != "GET" {
		http.Error(w, "method must be GET", http.StatusBadRequest)
		return
	}

	productId := submatches[1]
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
	price, t, err := s.LastPrice(productId)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		// TODO log err
		return
	}

	if price == json.Number("") {
		http.Error(w, "productId not found", http.StatusNotFound)
		return
	}

	body.ProductId = productId // this is silly
	body.Price = price
	body.Timestamp = epochTime(t)

	// json content type
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	e := json.NewEncoder(w)
	e.SetIndent("", "\t") // specification example has literal tabs in it, but this is also silly
	_ = e.Encode(body) // TODO log error if any, only likely to be IO errors
}

// serialized in JSON as fractional unix epoch time
type epochTime time.Time

func (t epochTime) MarshalJSON() ([]byte, error) {
	const nanoToSecond = float64(time.Nanosecond) / float64(time.Second)
	return json.Marshal(float64(time.Time(t).UnixNano()) * nanoToSecond)
}

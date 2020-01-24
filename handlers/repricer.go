package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
)

// Reprice constructs a new reprice endpoint handler with the given storage model
func Reprice(m PriceUpdater) http.Handler { return reprice{m} }

// PriceUpdater defines an interface for writing new price data to storage
type PriceUpdater interface {
	// UpdatePrice sets the latest price for a productId
	UpdatePrice(productId string, price json.Number) error
}

type reprice struct{ PriceUpdater }

var repricePath = regexp.MustCompile(basePath.String() + `reprice$`)

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
		ProductId string      `json:"productId"`
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

	// write the new price data to storage.
	err = s.UpdatePrice(body.ProductId, body.Price)
	if err != nil {
		code := http.StatusInternalServerError

		if _, ok := err.(interface{ Temporary() bool }); ok {
			code = http.StatusServiceUnavailable
		}

		// TODO log err, if code = 503, only warn

		http.Error(w, http.StatusText(code), code)
		return
	}

	// according to RFC 7231 the 202 status code is non committal, therefore
	// it seems like we don't need to block until the new price data is synced
	// to disk? UpdatePrice() could be called in a new goroutine instead
	w.WriteHeader(http.StatusAccepted)
}

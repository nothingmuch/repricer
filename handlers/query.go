package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/nothingmuch/repricer/errors"
)

// Query constructs a new query price endpoint with the given storage model
func Query(m PriceLogRetriever) http.Handler { return query{m} }

// PriceLogRetriever defines an interface for fetching historical price data
type PriceLogRetriever interface {
	PriceLog(
		productId string,
		startTime, endTime time.Time,
		offset int64, limit int,
	) (
		[]struct {
			// TODO make Entry/Record types public?
			ProductId string
			Price     json.Number
			Timestamp time.Time
		}, error)
}

type query struct{ PriceLogRetriever }

var queryPath = regexp.MustCompile(basePath.String() + `query`)

func (s query) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// TODO rate limiting
	if !queryPath.MatchString(req.URL.Path) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if req.Method != "GET" {
		http.Error(w, "method must be GET", http.StatusBadRequest)
		return
	}

	// Validate inputs
	// TODO error on multiple values, except for possibly productId, which
	// can be handled as a union (disjoint union, which makes things easier)
	params := req.URL.Query()
	var productId string
	var pageSize, pageNumber int // TODO int64? disallow negative values?
	var startTime, endTime time.Time
	var inputErrors error
	var err error
	if v, exists := params["productId"]; exists && len(v) == 1 {
		productId = v[0]
	}
	if v, exists := params["pagesize"]; exists && len(v) == 1 { // note inconsistent capitalization
		pageSize, err = strconv.Atoi(v[0])
		errors.Collect(&inputErrors, err)
	}
	if v, exists := params["pageNumber"]; exists && len(v) == 1 {
		pageNumber, err = strconv.Atoi(v[0])
		errors.Collect(&inputErrors, err)
	}
	if v, exists := params["from"]; exists && len(v) == 1 {
		startTime, err = time.Parse(time.RFC3339, v[0])
		errors.Collect(&inputErrors, err)
	}
	if v, exists := params["to"]; exists && len(v) == 1 {
		endTime, err = time.Parse(time.RFC3339, v[0])
		errors.Collect(&inputErrors, err)
	}
	if inputErrors != nil {
		http.Error(w, inputErrors.Error(), http.StatusBadRequest) // FIXME sanitize errors
		return

	}

	if pageSize == 0 {
		pageSize = 25 // FIXME arbitrary
	}

	// convert pagination information to more convenient representation for data
	offset := int64(pageNumber-1) * int64(pageSize)
	if offset < 0 {
		offset = 0
	}
	limit := pageSize

	entries, err := s.PriceLog(productId, startTime, endTime, offset, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		// TODO log err
		return
	}

	body := make([]struct {
		ProductId string      `json:"productId"`
		Price     json.Number `json:"price"`
		Timestamp epochTime   `json:"timestamp"`
	}, len(entries))

	for i, ent := range entries {
		body[i].ProductId = ent.ProductId
		body[i].Price = ent.Price
		body[i].Timestamp = epochTime(ent.Timestamp)
	}

	// json content type
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	e := json.NewEncoder(w)
	e.SetIndent("", "\t") // specification example has literal tabs in it, but this is also silly
	_ = e.Encode(body)    // TODO log error if any, only likely to be IO errors
}

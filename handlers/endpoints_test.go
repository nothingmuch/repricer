package handlers_test

/* these are just basic sanity tests written to aid development, not
comprehensive unit tests for quality assurance (due to exam limitations)

TODO:
- input validation errors, e.g. out of range values, edge/corner cases
- handling of "strange" chars in productId (esp. URL meaningful chars, like '/')
- exhaustive checking of failure modes, e.g. unreliable storage
- 100% coverage
- refactor TestStatefulness to extract reusable functionality, esp. for table
- driven regression testing
*/

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nothingmuch/repricer/handlers"
)

func TestRepriceEndpoint(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com/api/reprice", strings.NewReader((`{"productId":"foo","price":3.50}`)))

	w := httptest.NewRecorder()
	handlers.Reprice(noopModel{}).ServeHTTP(w, req)

	resp := w.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted {
		t.Error("response code should be 202")
	}

	if len(body) != 0 {
		t.Error("body should be empty")
	}
}

func TestStatefulness(t *testing.T) {
	h := handlers.API(simpleMap{t, make(map[string]entry)})

	// all returned times must be later than this
	t0 := float64(time.Now().UnixNano()) * (float64(time.Nanosecond) / float64(time.Second))

	post := func(productId string, price json.Number) int {
		by, err := json.Marshal(struct {
			ProductId string      `json:"productId"`
			Price     json.Number `json:"price"`
		}{productId, price})

		t.Log("POST", string(by))

		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest("POST", "http://example.com/api/reprice", bytes.NewReader(by))

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		resp := w.Result()

		if resp.StatusCode != http.StatusAccepted {
			t.Error("response code should be 202")
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Error(err)
		} else if len(body) != 0 {
			t.Error("body should be empty")
		}

		return resp.StatusCode
	}

	get := func(productId string) (int, json.Number, json.Number) {
		req := httptest.NewRequest("GET", "http://example.com/api/product/"+productId+"/price", nil)

		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		resp := w.Result()

		var body struct {
			ProductId string      `json:"productId"`
			Price     json.Number `json:"price"`
			Timestamp json.Number `json:"timestamp"`
		}

		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Error(err)
			}

			if body.ProductId != productId {
				t.Error("productId in result (" + body.ProductId + ") should match queried path (" + productId + ")")
			}

			if body.Price == json.Number("") || body.Price == json.Number("0") {
				t.Error("price should be nonzero")
			}

			if body.Timestamp == json.Number("") || body.Price == json.Number("0") {
				t.Error("timestamp should be nonzero")
			}

			if t1, err := body.Timestamp.Float64(); t1 < t0 {
				if err != nil {
					t.Error(err)
				} else {
					t.Error("timestamp should not be before test started")
				}
			}
		}

		t.Log("GET", productId, resp.Status, body.Price, body.Timestamp)
		return resp.StatusCode, body.Price, body.Timestamp
	}

	expectMissing := func(productId string) {
		code, price, t1 := get(productId)

		if code != http.StatusNotFound {
			t.Error("product " + productId + " should be missing")
		}

		if price != json.Number("") || t1 != json.Number("") {
			t.Error("product " + productId + " should have no field values")

		}
	}

	expectPrice := func(productId string, expectedPrice json.Number) {
		code, price, t1 := get(productId)

		if code != http.StatusOK {
			t.Error("getting price product " + productId + " should return OK but got " + fmt.Sprint(code))
		}

		if price == json.Number("") || t1 == json.Number("") {
			t.Error("product " + productId + " should have values for all fields")
		}

		if price != expectedPrice {
			t.Error("price for " + productId + " should be " + string(expectedPrice) + " but got " + string(price))
		}
	}

	// TODO table driven subtests
	expectMissing("foo")
	expectMissing("bar")
	post("foo", json.Number("3.50"))
	expectPrice("foo", json.Number("3.50"))
	expectMissing("bar")
	post("bar", json.Number("21.00"))
	expectPrice("foo", json.Number("3.50"))
	expectPrice("bar", json.Number("21.00"))
	post("foo", json.Number("42.0"))
	expectPrice("foo", json.Number("42.0"))
	expectPrice("bar", json.Number("21.00"))
}

type noopModel struct{}

func (noopModel) UpdatePrice(string, json.Number) error { return nil }

// simple in memory model to check state updates
type entry struct {
	Price json.Number
	time.Time
}
type simpleMap struct {
	*testing.T
	data map[string]entry
}

func (m simpleMap) UpdatePrice(productId string, price json.Number) error {
	m.data[productId] = entry{price, time.Now()}
	m.Log("->state", productId, m.data[productId])
	return nil
}
func (m simpleMap) LastPrice(productId string) (json.Number, time.Time, error) {
	ent := m.data[productId]
	m.Log("<-state", productId, ent)
	return ent.Price, ent.Time, nil
}

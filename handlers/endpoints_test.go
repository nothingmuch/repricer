package handlers_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nothingmuch/repricer/handlers"
)

func TestRepriceEndpoint(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.com/api/reprice", strings.NewReader((`{"productId":"foo","price":3.50}`)))

	w := httptest.NewRecorder()
	handlers.Reprice.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted {
		t.Error("response code should be 202")
	}

	if len(body) != 0 {
		t.Error("body should be empty")
	}
}

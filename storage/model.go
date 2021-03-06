package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	NullPrice = json.Number("")
)

func New(path string) extendedPriceModel { // TODO return error, plumb errors & context
	err := os.MkdirAll(filepath.Join(path, ResultsSubdirectory), 0777)
	if err != nil {
		panic(err)
	}
	return newFromFS(osFS(path))
}

type entry struct {
	Price json.Number `json:"newPrice"`
	Time  time.Time   `json:"timestamp"` // TODO rename Timestamp
}

type record struct {
	ProductId     string      `json:"productId"`
	PreviousPrice json.Number `json:"previousPrice,omitempty"`
	entry
}

type priceUpdater interface {
	UpdatePrice(productId string, price json.Number) error
}

type priceReader interface {
	HasPrice(string) bool
	LastPrice(string) (json.Number, time.Time, error) // TODO(bikeshedding): (Entry, error) ?  (*Entry, error) to eliminate HasPrice?
}

type priceLogRetriever interface {
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
		},
		error,
	)
}

type priceSetter interface {
	SetPrice(productId string, price json.Number, timestamp time.Time) error
}

type priceSetterAtomic interface {
	SetPriceIfMissing(productId string, price json.Number, timestamp time.Time) error
}

type recordWriter interface {
	writeRecord(record *record /*TODO chan struct{} closed on sync */) error
}

type priceState interface {
	priceReader
	priceSetter
	priceSetterAtomic
}

type priceModel interface {
	priceReader
	priceUpdater
}

// price resource model, consumed by REST api
type extendedPriceModel interface {
	priceModel
	priceLogRetriever
}

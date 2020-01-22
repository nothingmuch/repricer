package storage

import (
	"encoding/json"
	"time"
)

const (
	NullPrice = json.Number("")
)

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

// price resource model, consumed by REST api
type priceModel interface {
	priceReader
	priceUpdater
}

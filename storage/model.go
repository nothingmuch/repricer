package storage

import (
	"encoding/json"
	"sync"
	"time"
)

type entry struct {
	Price json.Number
	time.Time
}

func New() interface {
	UpdatePrice(string, json.Number) error
	LastPrice(string) (json.Number, time.Time, error)
} {
	return &store{}
}

type store struct{ sync.Map }

func (s *store) UpdatePrice(productId string, price json.Number) error {
	s.Map.Store(productId, &entry{price, time.Now()})
	return nil
}

func (s *store) LastPrice(productId string) (json.Number, time.Time, error) {
	if ent, exists := s.Map.Load(productId); exists {
		ent := ent.(*entry)
		return ent.Price, ent.Time, nil
	} else {
		return json.Number(""), time.Time{}, nil
	}
}

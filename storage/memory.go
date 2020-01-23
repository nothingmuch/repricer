package storage

import (
	"encoding/json"
	"sync"
	"time"
)

type memStore struct{ m sync.Map }

func (s *memStore) SetPrice(productId string, price json.Number, t time.Time) error {
	s.m.Store(productId, &entry{price, t})
	return nil
}

func (s *memStore) SetPriceIfMissing(productId string, price json.Number, t time.Time) error {
	s.m.LoadOrStore(productId, &entry{price, t})
	return nil
}

func (s *memStore) HasPrice(productId string) bool {
	_, exists := s.m.Load(productId)
	return exists
}

func (s *memStore) LastPrice(productId string) (json.Number, time.Time, error) {
	if ent, exists := s.m.Load(productId); exists {
		ent := ent.(*entry)
		return ent.Price, ent.Time, nil
	} else {
		return json.Number(""), time.Time{}, nil
	}
}

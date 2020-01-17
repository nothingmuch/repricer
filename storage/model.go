package storage

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

type model interface {
	UpdatePrice(string, json.Number) error
	LastPrice(string) (json.Number, time.Time, error)
}

func New(path string) (model, error) {
	d, err := open(path)
	if err != nil {
		return nil, err
	}

	return &store{dir: d}, nil
}

type store struct {
	dir
	sync.Map
}

// in memory entry, but json serialzation applies to persisted files
type entry struct {
	Price json.Number `json:"newPrice"`
	Time  time.Time   `json:"timestamp"`
}

// data at rest
type record struct {
	ProductId string `json:"productId"`
	entry
}

func (s *store) UpdatePrice(productId string, price json.Number) error {
	// note that the specification showed a time zone offset +03:00
	// i'm assuming this only signifies the format (RFC 3339 / ISO-8601)
	// since it's not clear where that value comes from, and the server's
	// localtime should not be combined with user inputs in this way
	// and also because non-UTC timestamps (even if only in a single time
	// zone) add a lot of edge/corner cases or not even be totally ordered
	now := time.Now() // FIXME timestamp assignments should serialize write ordering, needs to happen synchronously

	ent := entry{price, now}
	s.Map.Store(productId, &ent)

	filename := now.Format(time.RFC3339Nano) + ".json" // FIXME add sequence numbers
	return s.dir.atomicWriteFile(filename, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "\t")                             // provided example in spec was tab indented
		return enc.Encode([]record{record{productId, ent}}) // FIXME write grouped entries, not singletons
	})
}

func (s *store) LastPrice(productId string) (json.Number, time.Time, error) {
	if ent, exists := s.Map.Load(productId); exists {
		ent := ent.(*entry)
		return ent.Price, ent.Time, nil
	} else {
		return json.Number(""), time.Time{}, nil
	}
}

package storage

import (
	"encoding/json"
	"path/filepath"
	"time"
)

// priceLoader provides a priceReader interface from a readerFS
type priceLoader struct{ readFS }

var _ priceReader = priceLoader{}

func (s priceLoader) HasPrice(_ string) bool { return false } // FIXME refactor
func (s priceLoader) LastPrice(productId string) (p json.Number, t time.Time, err error) {
	d := s.readFS.Sub(filepath.Join(ProductSubdirectory, ProductIdHash(productId)))

	files, err := d.Files()
	if err != nil {
		return
	}

	if len(files) == 0 {
		return
	}

	lastFile := files[len(files)-1] // TODO func LastFile(readFS) string which checks for d.LastFile and to do this more cheaply?

	f, err := d.Open(lastFile)
	if err != nil {
		return
	}

	var r []record
	err = json.NewDecoder(f).Decode(&r)
	if err != nil {
		return
	}

	for i := len(r) - 1; i >= 0; i-- {
		if r[i].ProductId == productId {
			return r[i].Price, r[i].Time, nil
		}
	}

	panic("should have found product")
}

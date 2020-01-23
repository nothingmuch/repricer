package storage

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/nothingmuch/repricer/errors"
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

	r, err := s.loadFile(d, lastFile)
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

func (s priceLoader) PriceLog(
	productId string,
	startTime, endTime time.Time,
	offset int64, limit int,
) (
	ret []struct {
		// FIXME make Entry/Record types public? need to refactor this
		// but there's still some bikeshedding to do, and the question
		// of the extendedPriceModel and whether or not most most
		// modeltypes should implement it or not
		ProductId string
		Price     json.Number
		Timestamp time.Time
	},
	err error,
) {
	var d readFS
	if productId == "" {
		d = s.readFS.Sub(ResultsSubdirectory)
	} else {
		d = s.readFS.Sub(filepath.Join(ProductSubdirectory, ProductIdHash(productId)))
	}

	files, err := d.Files()
	if err != nil {
		return
	}

	if len(files) == 0 {
		return
	}

	// TODO
	// - search for beginning of interval, find latest file that is a lower
	//   bound
	// - sort.search for offset entrySeq
	//   - time-GLB entrySeq + n is base, where n = nRecords, add this to offset... open file to find out n? or can it be handled after?

	// - sort.Search to skip ahead to entrySeq
	// - find 1st entry's seq# to re-base offsets
	// - search for entrySeq interval, slice filenames

	skip := offset
	for _, file := range files {
		records, loadErr := s.loadFile(d, file)
		if loadErr != nil {
			errors.Collect(&err, loadErr)
			continue
		}

		for _, rec := range records {
			// FIXME seek
			if skip > 0 {
				skip--
				continue
			}

			// FIXME check that end seq isn't exceeded, otherwise break
			ret = append(ret, struct {
				ProductId string
				Price     json.Number
				Timestamp time.Time
			}{
				ProductId: rec.ProductId,
				Price:     rec.entry.Price,
				Timestamp: rec.entry.Time,
			})

			if len(ret) == limit {
				return
			}
		}
	}

	return
}

func (s priceLoader) loadFile(d readFS, filename string) (r []record, err error) {
	f, err := d.Open(filename)
	if err != nil {
		return
	}

	err = json.NewDecoder(f).Decode(&r)
	return
}

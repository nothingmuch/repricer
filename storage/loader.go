package storage

import (
	"encoding/json"
	"path/filepath"
	"sort"
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

	// set up a reasonable upper bound if not set
	if endTime.IsZero() {
		endTime = time.Now()
	}

	// parse filenames to search over metadata fields
	parsed := make([]filename, len(files))
	for i, filename := range files {
		err = parsed[i].FromString(filename)
		if err != nil {
			return
		}
	}

	skip := offset           // number of entries to skip before emitting any records
	baseEntrySeq := int64(1) // entrySeq of the 1st entry in the time interval, where offset starts counting

	// search for beginning of interval, find file that is a greatest
	// lower bound on timestamp, and slice filename list to suffix
	if skipFiles := sort.Search(len(parsed), func(i int) bool {
		return !startTime.After(parsed[i].start)
	}); 0 < skipFiles {
		// start time older than all data
		if skipFiles == len(files) {
			return
		}

		// time-GLB.entrySeq + n == t0-entrySeq < time-GLB.entrySeq + nReceords
		// open file to get t0-entrySeq (seq of first record in time interval)
		//
		// target entrySeq = time-GLB.entrySeq + n + offset
		records, _ := s.loadFile(d, files[skipFiles]) // TODO error
		var offsetInFile int
		for i, rec := range records {
			if rec.entry.Time.After(startTime) {
				offsetInFile = i
				break
			}
		}

		// calculate a new base offset
		baseEntrySeq = parsed[skipFiles].entrySeq + int64(offsetInFile)

		// slice off uninteresting prefix
		parsed = parsed[skipFiles:]
		files = files[skipFiles:]
	}

	// search for file where desired offset resides
	if skipFiles := sort.Search(len(parsed), func(i int) (r bool) {
		return parsed[i].entrySeq+parsed[i].nRecords >= baseEntrySeq+offset
	}); 0 < skipFiles {
		// offset is past end of results
		if skipFiles == len(files) {
			return
		}

		// we only need to skip the entries that remain inside the file
		skip = baseEntrySeq + offset - parsed[skipFiles].entrySeq

		// and again slice off the uninteresting prefix
		parsed = parsed[skipFiles:]
		files = files[skipFiles:]
	}

	for i, filename := range files {
		// since time values are totally ordered, once we see a file
		// with a timestamp outside of the interval, we can terminate
		if parsed[i].start.After(endTime) {
			return
		}

		records, loadErr := s.loadFile(d, filename)
		if loadErr != nil {
			// TODO handle parse errors (partly written data)
			errors.Collect(&err, loadErr)
			continue
		}

		for _, rec := range records {
			// omit leading entries that may be in the files of interest
			// and don't count them towards offset
			if rec.entry.Time.Before(startTime) {
				continue
			}

			// since time values are totally ordered, once we see an
			// entry past the end we're also done
			if rec.entry.Time.After(endTime) {
				return
			}

			if skip > 0 {
				skip--
				continue
			}

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

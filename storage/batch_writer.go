package storage

import (
	"encoding/json"
	"math"
	"path/filepath"
	"sync"
	"time"
)

const (
	FlushInterval       = time.Second
	MaxRecordsPerFile   = 10
	ResultsSubdirectory = "results"
	ProductSubdirectory = "results_by_product"
)

// batchWriter is a recordwriter that writes records in batches.
// it is not safe for concurrent use
// init with a file seq #s
// implement recordWriter interface
type batchWriter struct {
	fs writeFS

	fileSeq  int64
	entrySeq int64

	*batch
}

func (w *batchWriter) writeRecord(r *record) (err error) {
	err = w.startBatchIfNeeded(r.entry.Time)
	if err != nil {
		return
	}

	// ensure batch is flushed if it's full
	if w.batch.nRecords+1 == MaxRecordsPerFile {
		defer w.closeBatch()
	}

	defer func() {
		if err == nil {
			w.entrySeq++
		}
	}()

	return w.batch.writeRecord(r)
}

func (w *batchWriter) startBatchIfNeeded(now time.Time) (err error) {
	if w.batch != nil {
		if now.Sub(w.batch.start) < FlushInterval {
			// batch is set, and OK to use
			return nil
		} else {
			// next event is too late for batch
			w.closeBatch()
		}
	}

	b := &batch{
		fs: w.fs,
		filename: filename{
			fileSeq:  w.fileSeq + 1,
			entrySeq: w.entrySeq + 1,
			start:    now,
		},

		end: now,
	}

	// FIXME refactor filepath logic into some abstraction
	b.file, err = w.fs.New(filepath.Join(ResultsSubdirectory, b.filename.String()))
	if err != nil {
		return err
	}

	w.fileSeq++

	err = b.initialize()
	if err != nil {
		return err
	}

	w.batch = b

	return nil
}

func (w *batchWriter) closeBatch() {
	batch := w.batch
	w.batch = nil
	batch.flush()
}

type batch struct {
	fs writeFS

	filename
	end time.Time

	productIds map[string]int

	file   appendFile
	synced chan struct{} // closes when synced

	flushOnce sync.Once

	sync.Mutex // FIXME needed because of outstanding data race
}

func (b *batch) initialize() error {
	b.productIds = make(map[string]int, 10)
	b.synced = make(chan struct{})

	// ensure buffer is always flushed after it can no longer be filled
	time.AfterFunc(FlushInterval, func() {
		b.flush()
	})

	// FIXME redo with ndjson for robustness
	_, err := b.file.Write([]byte("[\n\t")) // start a JSON array as per spec
	if err != nil {
		return err
	}

	return nil
}

func (b *batch) writeRecord(r *record) (err error) {
	if r.entry.Time.Before(b.end) {
		panic("time went backwards")
	}

	// FIXME due to data races on internal fields, should not be necessary in principle
	b.Lock()
	defer b.Unlock()

	// FIXME ndjson to remove this hack while retaining durability of early writes
	if b.nRecords > 0 && err == nil {
		_, err = b.file.Write([]byte(",\n\t"))
		if err != nil {
			return
		}
	}

	by, err := json.MarshalIndent(r, "\t", "\t")
	if err != nil {
		return
	}

	_, err = b.file.Write(by)
	if err != nil {
		return
	}

	old := b.filename
	b.nRecords++
	b.end = r.entry.Time
	if b.productIds[r.ProductId] == 0 {
		b.nProductIds++
	}
	b.productIds[r.ProductId]++

	// keep update nRecords and nProducts fields up to date in the filename
	err = b.fs.Rename(
		// TODO abstract ResultsSubdirectory logic
		filepath.Join(ResultsSubdirectory, old.String()),
		filepath.Join(ResultsSubdirectory, b.filename.String()),
	)

	// TODO how to track entrySeq per product?
	// could keep track and independently load from snapshot
	return
}

func (b *batch) flush() {
	b.flushOnce.Do(func() {
		go func() {
			b.Lock() // FIXME still needed due to data race on f.filename, in principle should not be necessary
			defer b.Unlock()

			_, _ = b.file.Write([]byte("\n]\n")) // TODO error
			_ = b.file.Sync()                    // TODO error
			_ = b.file.Close()                   // TODO error

			// TODO abstract filepath logic
			finalName := filepath.Join(ResultsSubdirectory, b.filename.String())

			// link to product index directories
			for productId, count := range b.productIds {
				productFilename := b.filename
				productFilename.nRecords = int64(count)
				productFilename.entrySeq = math.MaxInt64 // FIXME entrySeq needs to be seq *per product ID*

				_ = b.fs.Link(finalName, filepath.Join(ProductSubdirectory, ProductIdHash(productId), productFilename.String())) // TODO error
			}

			close(b.synced)
		}()
	})
}

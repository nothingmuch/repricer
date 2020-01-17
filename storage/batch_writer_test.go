package storage

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestBatchWriterContents(t *testing.T) {
	fs := newMemFS()
	w := batchWriter{fs: fs}

	t0 := time.Now().UTC().Truncate(0)
	r0 := &record{ProductId: "foo", entry: entry{Price: json.Number("3.50"), Time: t0}}

	err := w.writeRecord(r0)
	if err != nil {
		t.Error(err)
	}

	n0 := filepath.Join(ResultsSubdirectory, w.batch.filename.String())

	// avoid spurious race detections
	fs.Lock()
	fs.m[n0].Lock()

	if len(fs.m[n0].ops) != 2 {
		t.Error("should have had 2 ops events")
	}

	var rd record
	err = json.Unmarshal([]byte(fs.m[n0].ops[1].data), &rd)

	if err != nil {
		t.Error(err)
	}

	if rd != *r0 {
		t.Error("written record should roundtrip")
	}
}

func TestBatchWriterGrouping(t *testing.T) {
	fs := newMemFS()
	w := batchWriter{fs: fs}

	t0 := time.Now().UTC().Truncate(0)
	r0 := &record{ProductId: "foo", entry: entry{Price: json.Number("3.50"), Time: t0}}

	t1 := time.Now().UTC().Truncate(0)
	r1 := &record{ProductId: "foo", entry: entry{Price: json.Number("2.20"), Time: t1}, PreviousPrice: r0.Price}

	err := w.writeRecord(r0)
	if err != nil {
		t.Error(err)
	}

	n0 := filepath.Join(ResultsSubdirectory, w.batch.filename.String())
	fs.Lock()
	fs.m[n0].Lock()
	if len(fs.m[n0].ops) != 2 {
		t.Error("should have had 2 ops events")
	}
	fs.m[n0].Unlock()
	fs.Unlock()

	err = w.writeRecord(r1)
	if err != nil {
		t.Error(err)
	}

	n1 := filepath.Join(ResultsSubdirectory, w.batch.filename.String())

	if n0 == n1 {
		t.Error("filenames should be different")
	}

	fs.Lock()
	if _, exists := fs.m[n0]; exists {
		t.Error("old filename should no longer exist")
	}
	fs.m[n1].Lock()
	if len(fs.m[n1].ops) != 4 {
		t.Error("should have had 4 write events")
	}
	fs.m[n1].Unlock()
	fs.Unlock()

	w.batch.flush()
	<-w.batch.synced

	fs.Lock()
	fs.m[n1].Lock()
	ops := fs.m[n1].ops

	if len(ops) != 7 {
		t.Error("should have had 7 total events")
	}

	if ops[len(ops)-2].name != "sync" {
		t.Error("before last op should be sync")
	}

	if ops[len(ops)-1].name != "close" {
		t.Error("last op should be close")
	}
}

func TestBatchWriterMaxSize(t *testing.T) {
	fs := newMemFS()
	w := batchWriter{fs: fs}

	var prevPrice json.Number
	write := func(n int) {
		for i := 1; i <= n; i++ {
			p := json.Number(fmt.Sprint(i))
			r := &record{ProductId: "foo", entry: entry{Price: p, Time: time.Now().UTC().Truncate(0)}, PreviousPrice: prevPrice}
			prevPrice = p
			err := w.writeRecord(r)
			if err != nil {
				t.Error(err)
			}
		}
	}

	if MaxRecordsPerFile < 3 {
		panic("can't test batching with max size < 3")
	}

	write(1)
	b0 := w.batch
	write(1)
	b1 := w.batch
	if b0 != b1 {
		t.Error("first two writes should have gone to the same batch")
	}
	select {
	case <-b1.synced:
		t.Error("first batch should not have been synced yet")
	default:
	}

	write(MaxRecordsPerFile)

	select {
	case <-b1.synced:
	case <-time.After(FlushInterval / 2):
		t.Error("first batch should have been synced")
	}

	b2 := w.batch
	if b2 == b1 || b2 == nil {
		t.Error("should have started a second batch")
	}
	select {
	case <-b2.synced:
		t.Error("second batch should not have been synced yet")
	case <-time.After(FlushInterval / 2):
	}

	write(MaxRecordsPerFile)
	select {
	case <-b2.synced:
	case <-time.After(FlushInterval / 2):
		t.Error("second batch should have been synced")
	}

	b3 := w.batch
	if b3 == b2 || b3 == nil {
		t.Error("should have started a third batch")
	}
	write(1)

	select {
	case <-b3.synced:
		t.Error("third batch should not have been synced yet")
	case <-time.After(FlushInterval / 2):
	}

	b4 := w.batch
	if b4 != b3 {
		t.Error("additional write should have gone to third batch")
	}

	select {
	case <-b3.synced:
	case <-time.After(FlushInterval):
		t.Error("third batch should have been synced within FlushInterval")
	}

	write(1)
	if b4 == w.batch {
		t.Error("final write should have created a new batch")
	}
}

func TestBatchWriterMonotonicity(t *testing.T) {
	fs := newMemFS()
	w := batchWriter{fs: fs}

	t0 := time.Now().UTC().Truncate(0)
	t1 := time.Now().UTC().Truncate(0)

	r0 := &record{ProductId: "foo", entry: entry{Price: json.Number("3.50"), Time: t1}}
	r1 := &record{ProductId: "foo", entry: entry{Price: json.Number("2.20"), Time: t0}, PreviousPrice: r0.Price}

	err := w.writeRecord(r0)

	if err != nil {
		t.Error(err)
	}

	// should panic, because non monotonic timestamps is a linearizer bug
	// that invalidates storage directory invariants
	var panic interface{}
	func() {
		defer func() { panic = recover() }()
		_ = w.writeRecord(r1)
	}()

	if panic == nil {
		t.Error("writing non monotonic timestamps in a batch should panic")
	}
}

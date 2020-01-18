package storage

import (
	"encoding/json"
	"testing"
	"time"
)

// unit test linearization of state only?
// or entire storage component?

func TestLinearizerSanity(t *testing.T) {
	writes := make(chanRecordWriter, 1)
	mem := simpleMap{"mem", t, make(map[string]entry)}

	model := linearizeUpdates(mem, noop{}, writes)

	price, timestamp, err := model.LastPrice("foo")
	if err != nil {
		t.Error(err)
	}
	if (price != NullPrice || timestamp != time.Time{}) {
		t.Error("should have gotten no data from linearized state model")
	}

	t.Log("setting foo")
	err = model.UpdatePrice("foo", json.Number("3.50"))
	if err != nil {
		t.Error(err)
	}

	<-writes

	price0, t0, err := model.LastPrice("foo")
	if err != nil {
		t.Error(err)
	}
	if (price0 != json.Number("3.50") || t0 == time.Time{}) {
		t.Error("should have gotten stored data from linearized state model")
	}
}

func TestLinearizerUpdateSanity(t *testing.T) {
	writes := make(chanRecordWriter, 1)
	mem := simpleMap{"mem", t, make(map[string]entry)}

	model := linearizeUpdates(mem, noop{}, writes)

	t.Log("setting foo")
	err := model.UpdatePrice("foo", json.Number("3.50"))
	if err != nil {
		t.Error(err)
	}

	rec := <-writes
	if rec.entry.Price != json.Number("3.50") {
		t.Error("written price should have been that of input")
	}

	price0, t0, err := model.LastPrice("foo")
	if err != nil {
		t.Error(err)
	}
	if (price0 != json.Number("3.50") || t0 == time.Time{}) {
		t.Error("should have gotten stored data from linearized state model")
	}

	t.Log("updating foo")
	err = model.UpdatePrice("foo", json.Number("2.20"))
	if err != nil {
		t.Error(err)
	}

	rec = <-writes
	if rec.entry.Price != json.Number("2.20") {
		t.Error("written price should have been that of input")
	}

	price, t1, err := model.LastPrice("foo")
	if err != nil {
		t.Error(err)
	}
	if price != json.Number("2.20") || t1.Before(t0) {
		t.Error("should have gotten updated data with later time from linearized state model", price, t0, t1)
	}

	select {
	case <-writes:
		t.Error("no further writes should have been made")
	default:
	}
}

func TestLinearizerPriorData(t *testing.T) {
	writes := make(chanRecordWriter, 1)
	mem := simpleMap{" mem", t, make(map[string]entry)}
	snap := simpleMap{"snap", t, make(map[string]entry)}

	t0 := time.Now()
	_ = snap.SetPrice("foo", json.Number("4.20"), t0)

	model := linearizeUpdates(mem, snap, writes)

	price, t1, err := model.LastPrice("foo")
	if err != nil {
		t.Error(err)
	}
	if price != json.Number("4.20") || t1.Before(t0) {
		t.Error("should have gotten prior data from snapshot")
	}

	t.Log("setting foo")
	err = model.UpdatePrice("foo", json.Number("3.50"))
	if err != nil {
		t.Error(err)
	}

	<-writes

	price0, t2, err := model.LastPrice("foo")
	if err != nil {
		t.Error(err)
	}
	if price0 != json.Number("3.50") || t2.Before(t1) {
		t.Error("should have gotten stored data from linearized state model")
	}
}

func TestLinearizerConcurrentRead(t *testing.T) {
	writes := make(chanRecordWriter, 1)
	mem := simpleMap{" mem", t, make(map[string]entry)}
	snap := simpleMap{"snap", t, make(map[string]entry)}
	release := make(chan struct{})

	t0 := time.Now()
	_ = snap.SetPrice("foo", json.Number("4.20"), t0)

	model := linearizeUpdates(mem, syncReader{snap, release}, writes)

	lastPriceChan := make(chan entry)

	go func() {
		price, t1, err := model.LastPrice("foo")
		if err != nil {
			t.Error(err)
		}
		if price != json.Number("3.50") || t1.Before(t0) {
			t.Error("should have gotten new data from subsequent update", price, t1)
		}

		lastPriceChan <- entry{price, t1}
	}()

	// ensure read request starts
	release <- struct{}{}

	go func() {
		err := model.UpdatePrice("foo", json.Number("3.50"))
		if err != nil {
			t.Error(err)
		}
	}()

	var t1 time.Time

	// the LastPrice call should return immediately upon updating
	select {
	case ent := <-lastPriceChan:
		if ent.Price != json.Number("3.50") {
			t.Error("last price should have been from update")
		}

		t1 = ent.Time
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for last price response")
	}

	select {
	case <-writes:
		t.Error("no data should have been written yet")
	case <-time.After(time.Millisecond):
	}

	// return from snap.LastReader, to release write
	<-release

	select {
	case rec := <-writes:
		if rec.entry.Price != json.Number("3.50") {
			t.Error("written record should have had newPrice specified in update")
		}
		if rec.PreviousPrice != json.Number("4.20") {
			t.Error("record should have previousPrice from snapshot")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for write")
	}

	// Ensure the new value can be read without requiring the snapshot to be
	// released again
	c := make(chan struct{})
	go func() {
		price0, t2, err := model.LastPrice("foo")
		if err != nil {
			t.Error(err)
		}
		if price0 != json.Number("3.50") || t2.Before(t1) {
			t.Error("should have gotten stored data from linearized state model")
		}
		close(c)
	}()

	select {
	case <-c:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for in memory read")
	}
}

/*
t0   - get last price foo - in memory miss
t0+e - last price from disk - 3.50@t-3
t0+e - serve 3.50@t-3

t0   - get  foo - last price foo - in memory miss miss
t1   - post foo - queue finalize foo 3.50@t0 - previousPrice still blocking -- can only queue finalization *once* per product Id - subsequent prices will always be known
t1+e - serve last price 3.50@t0 - get request terminates
t1+e - queue write foo 1.23@t1 - previousPrice 3.50@t0 but write still blocking
t1+e - last price fromdisk - 2.20@t-2
t1+e - write foo 3.50@t0, previousPrice 2.20@t-2 - flushed
t1+e - write foo 1.23@t0, previousPrice 3.50@t0  */

type noop struct{}

func (noop) UpdatePrice(_ string, _ json.Number) error                { return nil }
func (noop) HasPrice(_ string) bool                                   { return false }
func (noop) LastPrice(_ string) (_ json.Number, _ time.Time, _ error) { return }
func (noop) SetPrice(_ string, _ json.Number, _ time.Time) error      { return nil }

type simpleMap struct {
	name string
	*testing.T
	data map[string]entry
}

func (m simpleMap) SetPrice(productId string, price json.Number, t time.Time) error {
	m.data[productId] = entry{price, t}
	m.Log("->", m.name, productId, m.data[productId])
	return nil
}

func (m simpleMap) SetPriceIfMissing(productId string, price json.Number, t time.Time) error {
	if x, exists := m.data[productId]; exists {
		m.Log("-|", m.name, productId, price, t, "(already set to", x, ")")
		return nil
	}
	return m.SetPrice(productId, price, t)
}

func (m simpleMap) LastPrice(productId string) (json.Number, time.Time, error) {
	ent := m.data[productId]
	m.Log("<-", m.name, productId, ent)
	return ent.Price, ent.Time, nil
}

func (m simpleMap) HasPrice(productId string) (ret bool) {
	_, ret = m.data[productId]
	return
}

type chanRecordWriter chan *record

func (c chanRecordWriter) writeRecord(r *record) error {
	c <- r
	return nil
}

type syncReader struct {
	priceReader
	wait chan struct{}
}

func (r syncReader) LastPrice(productId string) (json.Number, time.Time, error) {
	r.wait <- <-r.wait // pass token
	return r.priceReader.LastPrice(productId)
}

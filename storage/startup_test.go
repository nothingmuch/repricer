package storage

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nothingmuch/repricer/errors"
)

func init() {
	FlushInterval = 10 * time.Millisecond
}

func TestSnapshotConsistency(t *testing.T) {
	// stack of snapshots that should all agree with each other
	m := modelStack{t, newMemFS(), nil}

	checkModelConsistency := func() {
		for _, productId := range []string{"foo", "bar", "baz", "qux", "zot", "wat", "lol"} {
			if _, _, err := m.LastPrice(productId); err != nil {
				t.Error(err)
			}
		}
	}

	_ = m.UpdatePrice("foo", "3.50")
	checkModelConsistency()
	m.checkpoint()
	checkModelConsistency()
	_ = m.UpdatePrice("bar", "2.20")
	checkModelConsistency()
	_ = m.UpdatePrice("bar", "2.21")
	checkModelConsistency()
	_ = m.UpdatePrice("zot", "1000")
	checkModelConsistency()
	_ = m.UpdatePrice("qux", "2.22")
	checkModelConsistency()
	time.Sleep(FlushInterval)
	m.checkpoint()
	checkModelConsistency()
	_ = m.UpdatePrice("foo", "3.75")
	checkModelConsistency()
	_ = m.UpdatePrice("baz", "1.23")
	checkModelConsistency()
	m.checkpoint()
	checkModelConsistency()
	_ = m.UpdatePrice("wat", "0.01")
	checkModelConsistency()
	time.Sleep(FlushInterval)
	checkModelConsistency()
	_ = m.UpdatePrice("baz", "1.79")
	checkModelConsistency()
	m.checkpoint()
	checkModelConsistency()

	// TODO at least: 1 old, 1 new, 1 updated file per checkpoint, 3 snapshot layers deep
}

type modelStack struct {
	testing.TB
	fs
	models []priceModel
}

func (s modelStack) checkpoint() {
	time.Sleep(2 * FlushInterval) // allow all buffers to flush // FIXME really hacky
	s.models = append(s.models, newFromFS(s.fs.(*memFS).clone()))
}

func (s *modelStack) UpdatePrice(productId string, price json.Number) (err error) {
	if len(s.models) == 0 {
		s.models = []priceModel{newFromFS(s.fs)}
	}

	for _, model := range s.models {
		errors.Collect(&err, model.UpdatePrice(productId, price))
	}
	return
}

func (s modelStack) LastPrice(productId string) (p json.Number, t time.Time, err error) {
	for i, model := range s.models {
		mp, mt, merr := model.LastPrice(productId)

		if i == 0 {
			p, t, err = mp, mt, merr
		}

		if p != mp || t != mt || err != merr {
			s.Error("model inconsistency for", productId)
		}
	}

	return
}

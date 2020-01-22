package storage

import (
	"encoding/json"
	"fmt"
	"time"
)

const WriteQueueLength = 50

type Log interface{ Log(...interface{}) }

// linearizedState is stacks an a partial in memory priceModel on top of a read
// only snapshot, to manage consistent population of `previousPrice` in records
// before being written to storage
//
// construct with linearizeUpdates()
type linearizedState struct {
	mem priceReader

	newPriceRecords   chan *record
	lastPriceRequests chan lastPriceRequest
}

var _ priceModel = linearizedState{}

// linearizeUpdates will, given:
// - `mem`, a priceModel used synchronously (ideally nonblocking) TODO interface with LoadOrStore semantics
// - `snapshot`, a priceReader used async defining initial last prices state
// - `persistent`, a sink for finalized price records
// returns a priceModel which will:
// - provides a RW view that shadows/overlays `mem` on top of `snapshot`
// - fill in consistent `previousPrice` values for updates and output to `persistent`
func linearizeUpdates(mem priceState, snapshot priceReader, persistent recordWriter) priceModel {
	// WriteQueueLength is split between two buffered channels: // TODO expose len() as metric
	newPriceRecords := make(chan *record, WriteQueueLength/2) // avoid failing nonblocking UpdatePrice() calls due to minor contention
	writeQueue := make(chan chan *record, WriteQueueLength/2) // avoid avoid blocking linearizer loop due to write contention pending previousPrice

	// no need to buffer read requests
	lastPriceRequests := make(chan lastPriceRequest) // TODO expose len() as metric

	// capture channels needed for implementing model interface as member variables
	l := linearizedState{
		mem:               mem,
		newPriceRecords:   newPriceRecords,
		lastPriceRequests: lastPriceRequests,
	}

	// TODO capture errors from both loop goroutines
	go l.flushWrites(persistent, writeQueue)
	go l.linearizeOperations(mem, snapshot, writeQueue, newPriceRecords, lastPriceRequests)

	return l
}

// this loop waits for records to be finalized and then passes them on to
// persistent storage.
func (linearizedState) flushWrites(persistent recordWriter, writeQueue <-chan chan *record) {
	// process the write queue in order
	for c := range writeQueue {
		// wait for individual records
		rec := <-c

		// perform a blocking write
		_ = persistent.writeRecord(rec)

		// TODO
		// - handle errors from writer
		// - pass chan struct{} to propagate notification of file sync
	}
}

type lastPriceRequest struct {
	productId string
	result    chan entry
}

type prevPriceRequest struct {
	productId string
	result    chan json.Number
}

// this loop linearizes all operations to create a total ordering of state
// updates and reads which are satisfied from memory or from disk
// the only potentially blocking operation should be writing to the writeQueue
// buffer when entries are finalized
//
// *records written into newPriceRecords will be used to update the last known price
// and will have their `previousPrice` set before being written back into
// writeQueue.
//
// during this interval the underlying struct is considered to be owned by the
// linearizer goroutine, which will make mutations to it, and should not be
// accessed by other goroutines
func (linearizedState) linearizeOperations(
	mem priceState,
	snapshot priceReader,
	writeQueue chan<- chan *record,
	newPriceRecords <-chan *record,
	lastPriceRequests <-chan lastPriceRequest,
) {
	// internal state variables to track requests by productId
	prevPriceListener := make(map[string]chan json.Number)
	lastPriceListeners := make(map[string][]chan entry)

	// internal channels for mananging in ongoing snapshot read requests
	// these are buffered so that they never cause the loop to block to
	// avoid needing to spawn additional goroutines
	prevPriceLoaded := make(chan *record, WriteQueueLength+1)
	prevPriceRequests := make(chan prevPriceRequest, WriteQueueLength+1)

	for {
		// case <-ctx.Cancel:
		// TODO handle shutdown?
		select {
		case req := <-lastPriceRequests: // FIXME how do these get created
			// start an inconsistent reads operation, returns latest
			// price from memory, disk or new reprice requests
			if price, time, _ := mem.LastPrice(req.productId); price != NullPrice { // TODO handle error
				// this is not redundant with mem.LastPrice outside of this
				// goroutine because other writes may have been
				// processed by this time
				req.result <- entry{price, time}
			} else {
				// register result channel
				lastPriceListeners[req.productId] = append(lastPriceListeners[req.productId], req.result)

				// make a new snapshot read to satisfy this
				// request

				// since prevPriceRequests is buffered, and
				// there can never be more than WriteQueueLength
				// requests, this channel write will not block,
				// but if this ever changes this may deadlock
				// and will need to be wrapped in a new goro
				prevPriceRequests <- prevPriceRequest{req.productId, make(chan json.Number, 1)}

				// no cancellation context is necessary
				// because if this last price request is
				// satisfied by a price update the result of
				// the snapshot read will still be needed for
				// previousPrice.
				//
				// if no `reprice` request occurs by the time
				// the read is satisfied, the price will be
				// stored in memory and this request will be simply
				// garbage collected, but if it is needed then that
				// request will be borrowed for signalling
				// finalization
			}
		case req := <-prevPriceRequests:
			// log.Log("previous price request for", req.productId)
			// start a snapshot read operation.
			// snapshot reads return the price prior to handling
			// any new `reprice` requests in this process

			// requests are triggered by a LastPrice miss on mem,
			// which can be initiated by either a reprice request
			// (needed for `previousPrice`) or `price` request.
			// in the case of a `price` request, if a `reprice`
			// request follows before the request can be satisfied
			// we consolidate them using this map
			//
			// for `price` requests following `reprice` in memory
			// data will always be served since the last reprice
			// input is the last known price (TODO for consistent
			// reads from `price` endpoint, in memory state should
			// only be updated after Sync)
			//
			// for isolated `price` requests, the old data wil be
			// pre-loaded into the sync map for use by subsequent
			// `price` and `reprice` requests

			if inFlight, exists := prevPriceListener[req.productId]; exists { // if not nil?
				// this shouldn't happen because the LastPrice()
				// operation inside the loop should have succeded
				// there should never be 2 previous price
				// requests per process, let alone concurrently

				// panic("bug: no snapshot read should have been in progress")

				// FIXME the following race condition is still possible:
				//  newPriceReq
				//  | lastPricereq
				//  | |
				//  | prevPriceReq
				//  prevPriceReq

				// as an ugly workaround, make sure the subsequent
				// requestor gets a copy of the value
				go func() {
					req.result <- <-inFlight
				}()

				// suppress normal execution path when this race occurrs
				continue
			}

			prevPriceListener[req.productId] = req.result

			// perform the blocking read in a new goroutine
			go func() {
				prevRec := &record{ProductId: req.productId}
				prevRec.entry.Price, prevRec.entry.Time, _ = snapshot.LastPrice(req.productId) // TODO handle errors
				// log.Log("read", *prevRec, "from snapshot")

				// results can be made available immediately
				// `previousPrice` assignments to unblock any
				// writes
				req.result <- prevRec.entry.Price

				// clean up needs to happen synchronously, so
				// it's handled in the main loop in the next
				// select case
				prevPriceLoaded <- prevRec
			}()
		case prevRec := <-prevPriceLoaded:
			// log.Log("previous price data loaded", *prevRec)
			// synchronous continuation of loadSnapshotPrice

			// if a value is already in memory it originates
			// from a more recent rewrite request, only set the
			// loaded price if none exists
			// it is safe to read and then write non atomically
			// because all writes are serialized by this goroutine
			// but sync.Map already provides LoadOrStore so we use
			// IfMissing variant
			_ = mem.SetPriceIfMissing(prevRec.ProductId, prevRec.entry.Price, prevRec.entry.Time) // TODO handle errors

			// remove listener, the result has already been written
			// to it in the background goroutine
			delete(prevPriceListener, prevRec.ProductId)

			// inconsistent readers need to be notified from the loop
			// goroutine (here) since the lastPriceListeners map &
			// slice have no synchronization primitives
			for _, resultChan := range lastPriceListeners[prevRec.ProductId] {
				resultChan <- prevRec.entry
			}
			delete(lastPriceListeners, prevRec.ProductId)
		case rec := <-newPriceRecords:
			// assign the canonical timestamp for a given reprice event
			// this assumes the OS clock is monotonic
			// FIXME preserve monotonic clock data to maintain
			// storage invariants on systems with a clock that can
			// go backwards
			rec.entry.Time = time.Now()
			// log.Log("assigned time", rec.entry.Time)

			// when the entry has a `previousValue` set it can be
			// written to persistent storage, signalled by `finalized`
			result := make(chan *record, 1)

			// store the new price in memory but first preserve any
			// previous value already in memory
			hasPrice := mem.HasPrice(rec.ProductId)             // FIXME remove
			previousPrice, _, _ := mem.LastPrice(rec.ProductId) // TODO handle errors

			// log.Log("memory has prev price?", hasPrice, previousPrice)

			// TODO for consistent `price` endpoint reads, this write
			// needs to be delayed until sync
			_ = mem.SetPrice(rec.ProductId, rec.entry.Price, rec.entry.Time)

			// finalize the `previousPrice` field
			if hasPrice { // TODO snapshot null prices are written back to memory, use them
				// non-blocking write path, set it and mark final immediately
				// log.Log("setting previous price from memory")
				rec.PreviousPrice = previousPrice // FIXME nullableNumber is an ugly type
				result <- rec
			} else {
				// log.Log("fetching previous price")
				// blocking path, need to wait for previous price
				var prevPriceResult chan json.Number

				if inFlightResult, exists := prevPriceListener[rec.ProductId]; exists {
					// log.Log("borrowing")
					// a snapshot read is already in progress due
					// to a `price` request, so we can just use its
					// result to finalize
					prevPriceResult = inFlightResult

					// independently, we can notify the
					// requests that are waiting for that data
					// that a more recent price is available
					for _, result := range lastPriceListeners[rec.ProductId] {
						// TODO for consistent `price` endpoint
						// reads, this needs to be deferred
						// until the data is written (or synced)
						// to disk
						result <- rec.entry // this should never block
					}
					delete(lastPriceListeners, rec.ProductId)
				} else {
					// log.Log("making previous price request")
					// no load operation in progress, start one
					prevPriceResult = make(chan json.Number, 1)
					prevPriceRequests <- prevPriceRequest{rec.ProductId, prevPriceResult}
				}

				// in either case, wait for the price data
				// and then mark the record final
				go func() {
					rec.PreviousPrice = <-prevPriceResult // FIXME nullableNumber is an ugly type
					// log.Log("got previous price", *rec)
					result <- rec
				}()
			}

			// always queue the records for writing in order as per
			// https://golang.org/ref/spec#Channel_types
			// since writeQueue is buffered, this should only block
			// due to backpressure from write loop
			writeQueue <- result

			// TODO make(chan struct{}) and associate with *record
			// to track syncs for managing consistent reads
		}
	}
}

func (l linearizedState) HasPrice(productId string) bool { return false } // FIXME remove by LastPrice(string) (*Entry, error)

// LastPrice provides the last known price of a given product (including non-durable state)
func (l linearizedState) LastPrice(productId string) (json.Number, time.Time, error) {
	// since productReader interface is concurrency safe, we first try to
	// satisfy a read from memory
	if price, time, err := l.mem.LastPrice(productId); price != NullPrice || err != nil {
		return price, time, err
	}

	// on miss, add a request to be handled by the linearizer loop
	result := make(chan entry, 1)
	l.lastPriceRequests <- lastPriceRequest{productId, result}

	// wait for request to be satisfied
	ent := <-result
	return ent.Price, ent.Time, nil
}

// UpdatePrice updates in memory price and queus a record for writing when processed.
//
// This implementation is non-blocking and will return an error when no writes
// can be accepted.
func (l linearizedState) UpdatePrice(productId string, price json.Number) error {
	select {
	case l.newPriceRecords <- &record{
		ProductId: productId,
		entry:     entry{Price: price},
	}:
		return nil
	default:
		// TODO add a blocking writer for completeness?
		return fmt.Errorf("write capacity exceeded") // TODO interface { Temporary() bool } for correct 503 vs. 500 code
	}
}

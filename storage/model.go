package storage

import (
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	FlushInterval     = time.Second
	MaxRecordsPerFile = 10
)

type errConst string

func (e errConst) Error() string { return string(e) }

const (
	ErrInvalidFilename  = errConst("invalid filename")
	ErrUnknownFormat    = errConst("unknown data in filename")
	ErrDuplicateFileSeq = errConst("duplicate file sequence number") // TODO sequence number and filenames
	ErrMissingFile      = errConst("missing file")                   // TODO sequence number
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

	// all filenames are indexed in memory for simplicity and to facilitate
	// random access for query endpoint eventually (POSIX readdir order is
	// undefined and so requires sorting without specific guarantees from
	// underlying filesystem)
	// the list of files is only read once at startup
	filenames, err := d.listFiles()
	if err != nil {
		return nil, err
	}

	s := store{
		dir:   d,
		files: make([]file, len(filenames)),
	}

	// restore in memory state from persisted files
	err = s.loadFiles(filenames)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

type store struct {
	sync.Mutex

	length int    // total number of prices (not files) not including buffer contents
	files  []file // sorted index of all files

	buffer []record    // pending records to be saved
	timer  *time.Timer // for flushing buffer

	dir

	priceByProductId sync.Map // price & time keyed by productId
}

// in memory entry, but json serialzation applies to persisted files
type entry struct {
	Price json.Number `json:"newPrice"`
	Time  time.Time   `json:"timestamp"`
}

// data at rest
type record struct {
	ProductId     string         `json:"productId"`
	PreviousPrice nullableNumber `json:"previousPrice"`
	entry
}

// represent uninitialized previous prices as null values instead of 0
type nullableNumber struct{ json.Number }

func (n nullableNumber) MarshalJSON() ([]byte, error) {
	if n.Number == "" {
		return []byte(`null`), nil
	} else {
		return json.Marshal(n.Number)
	}
}

func (n *nullableNumber) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &n.Number)
}

func (s *store) LastPrice(productId string) (json.Number, time.Time, error) {
	if ent, exists := s.priceByProductId.Load(productId); exists {
		ent := ent.(*entry)
		return ent.Price, ent.Time, nil
	} else {
		return json.Number(""), time.Time{}, nil
	}
}

func (s *store) UpdatePrice(productId string, price json.Number) error {
	// note that the specification showed a time zone offset +03:00
	// i'm assuming this only signifies the format (RFC 3339 / ISO-8601)
	// since it's not clear where that value comes from, and the server's
	// localtime should not be combined with user inputs in this way
	// and also because non-UTC timestamps (even if only in a single time
	// zone) add a lot of edge/corner cases or not even be totally ordered

	// this mutex protects the mutable fields in store, and also ensures
	// that the timestamps of sequential entries are monotonically
	// increasing which is why it's taken before time.Now()
	s.Lock()
	defer s.Unlock()

	ent := entry{price, time.Now()}

	// note that storePriceEntryNoMutex must be called before updating
	// priceByProductId in order to save the previous price
	s.storePriceEntryNoMutex(productId, ent)
	s.priceByProductId.Store(productId, &ent)

	return nil
}

func (s *store) storePriceEntryNoMutex(productId string, ent entry) {
	previousPrice, _, _ := s.LastPrice(productId)
	rec := record{productId, nullableNumber{previousPrice}, ent}

	if s.buffer == nil {
		s.buffer = make([]record, 1, MaxRecordsPerFile)
		s.buffer[0] = rec
		s.timer = time.AfterFunc(FlushInterval, s.flushBuffer)
	} else {
		s.buffer = append(s.buffer, rec)
		if len(s.buffer) == MaxRecordsPerFile {
			s.flushBufferNoMutex() // mutexes in go aren't recursive
		}
	}
}

func (s *store) flushBuffer() {
	s.Lock()
	defer s.Unlock()

	// if the buffer is already empty it was preemptively flushed after the
	// timer triggered, and there's no need to do anything
	if len(s.buffer) == 0 {
		return
	}

	s.flushBufferNoMutex()
}

func (s *store) flushBufferNoMutex() {
	s.timer.Stop() // no need to check if it already fired, since we've got the mutex

	buffer := s.buffer

	newFile := file{
		time:       buffer[0].Time,
		seq:        len(s.files),
		firstEntry: s.length,
		length:     len(buffer),
	}

	s.buffer = nil
	s.timer = nil
	s.length += len(buffer)
	s.files = append(s.files, newFile)

	go func() {
		err := s.dir.atomicWriteFile(newFile.name(), func(w io.Writer) error {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "\t") // provided example in spec was tab indented
			return enc.Encode(buffer)
		})

		if err != nil {
			return // TODO log error
		}
	}()
}

func (s *store) loadFiles(filenames []string) error {
	// construct the file index by parsing all filenames
	lastSeq, nFiles := -1, 0
	for _, filename := range filenames {
		if filename == "." || filename == ".." {
			// this doesn't seem necessary, as these entries are not
			// returned from file.Readdirnames at least on linux, but i
			// can't find any documentation or comments in the code to
			// that effect. no harm if it's dead code.
			continue
		}

		if strings.HasSuffix(filename, ".tmp") {
			// TODO cleanup? error?
			continue
		}

		var f file
		if err := f.parse(filename); err != nil {
			return err
		}

		if s.files[f.seq] != (file{}) {
			return ErrDuplicateFileSeq // TODO report conflicting filenames
		}

		s.files[f.seq] = f
		if lastSeq < f.seq {
			lastSeq = f.seq
			s.length = f.firstEntry + f.length
		}
		nFiles++
	}

	// ensure no trailing entries due to skipped files
	s.files = s.files[:lastSeq+1]

	// ensure no missing data
	if nFiles != len(s.files) {
		return ErrMissingFile // TODO scan s.files for == file{} and report which sequence number
	}

	// load all price data by scanning backwards and filling in missing productId
	// this needs to be a full scan for now since we don't know the set of
	// productIds, but that could be recovered by indexing files which
	// contain null previousPrice entries after that has been implemented
	var recs []record
	for i := len(s.files) - 1; i >= 0; i-- {
		r, err := s.openFile(s.files[i].name())
		if err != nil {
			return err
		}

		err = json.NewDecoder(r).Decode(&recs)
		if err != nil {
			return err
		}

		for j := len(recs) - 1; j >= 0; j-- {
			ent := recs[j].entry // make a copy
			s.priceByProductId.LoadOrStore(recs[j].ProductId, &ent)
		}
	}

	return nil
}

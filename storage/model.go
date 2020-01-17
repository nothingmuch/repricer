package storage

import (
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"
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
	priceByProductId sync.Map // price & time keyed by productId

	sync.Mutex        // mutex for mutable fields
	length     int    // total number of prices (not files)
	files      []file // sorted index of all files

	dir
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

	// FIXME timestamp assignments should serialize write ordering, needs to
	// happen synchronously to ensure that price timestamps are
	// monotonically increasing
	now := time.Now()

	ent := entry{price, now}
	s.storePriceEntry(productId, ent)
	s.priceByProductId.Store(productId, &ent)

	return nil
}

func (s *store) storePriceEntry(productId string, ent entry) error {
	s.Lock()
	defer s.Unlock()

	newFile := file{
		time:       ent.Time,
		seq:        len(s.files),
		firstEntry: s.length,
		length:     1,
	}

	err := s.dir.atomicWriteFile(newFile.name(), func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "\t")                             // provided example in spec was tab indented
		return enc.Encode([]record{record{productId, ent}}) // FIXME write grouped entries, not singletons
	})

	if err != nil {
		return err
	}

	s.files = append(s.files, newFile)
	s.length++

	return nil
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

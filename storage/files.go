package storage

import (
	"fmt"
	"strings"
	"time"
)

// represents metadata about a data file, encoded in filename
type file struct {
	time       time.Time
	seq        int
	firstEntry int
	length     int
}

// file sequence number is formatted as 64 bit big endian hex so that
// lexicographical ordering is consistent with data ordering (64 bits is way
// overkill, but 32 bits seem a bit myopic)
const filenameFormat = "%016x-%d-%d_%d_%d.json"

func (f file) name() string {
	return fmt.Sprintf(filenameFormat, f.seq, f.firstEntry, f.length, f.time.Unix(), f.time.Nanosecond())
}

func (f *file) parse(filename string) error {
	if !strings.HasSuffix(filename, ".json") {
		return ErrInvalidFilename
	}
	var unix, ns int64
	_, err := fmt.Sscanf(filename, filenameFormat, &f.seq, &f.firstEntry, &f.length, &unix, &ns)
	if err != nil {
		return err
	}

	f.time = time.Unix(unix, ns)

	if f.name() != filename {
		return ErrUnknownFormat
	}

	return nil
}

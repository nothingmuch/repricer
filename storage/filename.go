package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/nothingmuch/repricer/errors"
)

type filename struct {
	fileSeq  int64
	entrySeq int64

	nRecords    int64
	nProductIds int64

	start time.Time
}

func (f filename) String() string {
	var b strings.Builder
	b.Grow(255) // max portable filename

	// big endian for lexicographical order
	err := binary.Write(hex.NewEncoder(&b), binary.BigEndian, []int64{
		f.fileSeq,
		f.entrySeq,
		f.nRecords,
		f.nProductIds,
		f.start.Unix(),
		int64(f.start.Nanosecond()),
	})
	if err != nil {
		panic(err)
	}

	b.WriteString(".json")

	if b.Len() > 255 {
		panic("filename too long, shouldn't happen")
	}

	return b.String()
}

func (f *filename) FromString(s string) (err error) {
	b, err := hex.DecodeString(strings.TrimSuffix(s, ".json"))
	if err != nil {
		return err
	}
	r := bytes.NewReader(b)

	errors.Collect(&err, binary.Read(r, binary.BigEndian, &f.fileSeq))
	errors.Collect(&err, binary.Read(r, binary.BigEndian, &f.entrySeq))
	errors.Collect(&err, binary.Read(r, binary.BigEndian, &f.nRecords))
	errors.Collect(&err, binary.Read(r, binary.BigEndian, &f.nProductIds))

	var unixSec, nanoSec int64
	errors.Collect(&err, binary.Read(r, binary.BigEndian, &unixSec))
	errors.Collect(&err, binary.Read(r, binary.BigEndian, &nanoSec))
	f.start = time.Unix(unixSec, nanoSec)

	return f.check()
}

type fieldError struct {
	name  string
	value int64
}

func (e fieldError) Error() string {
	return fmt.Sprintf("invalid %s %d", e.name, e.value)
}

func (f filename) check() (err error) {
	// TODO version bit?

	if f.fileSeq < 1 {
		errors.Collect(&err, fieldError{"fileSeq", f.fileSeq})
	}
	if f.entrySeq < 1 {
		errors.Collect(&err, fieldError{"entrySeq", f.entrySeq})
	}
	if f.nRecords < 1 {
		errors.Collect(&err, fieldError{"nRecords", f.nRecords})
	}
	if f.nProductIds < 1 {
		errors.Collect(&err, fieldError{"nProductIds", f.nProductIds})
	}

	return
}

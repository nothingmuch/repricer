package storage

import (
	"testing"
	"time"
)

func TestFilenameCheck(t *testing.T) {
	var f filename

	if err := f.check(); err == nil {
		t.Error("blank filename should be invalid")
	}

	f.fileSeq = 1
	f.entrySeq = 1
	f.nRecords = 1
	f.nProductIds = 1

	if err := f.check(); err != nil {
		t.Error("minimum fields should be enough", err)
	}
}

func TestFilenameString(t *testing.T) {
	var f filename

	f.fileSeq = 1
	f.entrySeq = 1
	f.nRecords = 1
	f.nProductIds = 1
	f.start = time.Now().Truncate(0)

	var f2 filename
	err := f2.FromString(f.String())

	if err != nil {
		t.Error(err)
	}

	if f != f2 {
		t.Error("struct data should survive round trip")
	}

	if f.String() != f2.String() {
		t.Error("string form should be identical")
	}
}

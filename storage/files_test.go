package storage

import (
	"testing"
	"time"
)

func TestFilenameSerialization(t *testing.T) {
	orig := file{
		time:       time.Now().Truncate(0), // discard monotonic clock values for struct comparison below
		seq:        3,
		firstEntry: 7,
		length:     2,
	}

	var parsed file
	err := parsed.parse(orig.name())

	if err != nil {
		t.Error(err)
	}

	if parsed != orig {
		t.Error("roundtripped struct should be identical", parsed, orig)
	}

	if parsed.name() != orig.name() {
		t.Error("serialized strings should be identical")
	}

	copy := orig
	copy.seq = 4

	if copy.name() == orig.name() {
		t.Error("serialized strings should be identical")
	}
}

func TestInvalidFilename(t *testing.T) {
	var parsed file

	for _, bad := range []string{
		".",
		"..",
		"foo.json",
		// file{}.name(), negative unix seconds should probably be disallowed
		file{time.Now(), 3, 7, 2}.name() + ".1234.tmp",
	} {
		err := parsed.parse(bad)
		if err == nil {
			t.Error("parsing invalid filename", bad, "should have raised error")
		}
	}

}

package storage

import (
	"sort"
)

// a filesystem that only lists filenames less than some upper bound
type snapshotFS struct {
	bound string
	readFS
}

var _ readFS = snapshotFS{}

func (s snapshotFS) Files() ([]string, error) {
	raw, err := s.readFS.Files()
	if err != nil {
		return nil, err
	}

	return raw[0:sort.SearchStrings(raw, s.bound)], nil
}

func (s snapshotFS) Sub(name string) readFS {
	return snapshotFS{
		bound:  s.bound,
		readFS: s.readFS.Sub(name),
	}
}

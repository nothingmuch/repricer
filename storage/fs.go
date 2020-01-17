package storage

import (
	"io"
)

// fs is the combined interface used for filesystem access
type fs interface {
	writeFS
	readFS
}

type writeFS interface {
	New(string) (appendFile, error) // O_WRONLY|O_APPEND|O_CREAT|O_EXCL
	Link(string, string) error
	Rename(string, string) error
}

type appendFile interface {
	io.WriteCloser
	Sync() error
}

type readFile io.Reader

type readFS interface {
	Open(string) (readFile, error) // readonly
	Files() ([]string, error)      // in lexicographical order
	Sub(string) readFS             // TODO generalize to Sub(string) fs? it's only really important for filescanning
}

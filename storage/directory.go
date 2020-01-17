package storage

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

func open(path string) (dir, error) {
	if err := os.MkdirAll(path, 0777); err != nil { // TODO does this ensure it is of the right type?
		return dir(""), err
	}

	// lockfile?

	return dir(path), nil
}

type dir string

func (d dir) atomicWriteFile(filename string, writeData func(io.Writer) error) (err error) {
	// ioutil.TempFile replaces the final * with randomness, add a fixed
	// suffix so we can filter out during readdir scanning (not actually
	// required for project as specified, but it's the right thing to do)
	f, err := ioutil.TempFile(string(d), filename+".*.tmp")
	if err != nil {
		return err
	}

	// if we reached this far, then a file has been created and a
	// file descriptor is opened, writing, syncing and closing will be
	// attempted unconditionally with any errors being getting accumulated
	// into an error slice.
	defer func() {
		if err != nil {
			fmt.Println("removing", f.Name())
			// if there was any error, attempt to remove the file,
			// and also collect that error
			collectErrors(&err, os.Remove(f.Name()))
		}
	}()

	// even though files are very likely to be smaller than a single block,
	// don't make assumptions since the batch size may change, and user
	// inputs are not length constrained at all right now
	collectErrors(&err, writeData(f))

	collectErrors(&err, f.Sync())

	collectErrors(&err, f.Close())

	if err == nil {
		// only attempt to rename if there have been no errors so far
		collectErrors(&err, os.Rename(f.Name(), filepath.Join(string(d), filename)))
	}

	return
}

type errors []error

func (err errors) Error() string { return fmt.Sprintf("%+v", []error(err)) } // TODO improve formatting?

func collectErrors(accum *error, new error) {
	if new == nil {
		return
	} else if *accum == nil {
		*accum = new
	} else if errs, ok := (*accum).(errors); ok {
		*accum = append(errs, new)
	} else {
		*accum = errors{*accum, new}
	}
}

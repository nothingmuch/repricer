package storage

import (
	"os"
	"path/filepath"
	"sort"
)

func OS(dir string) fs {
	return osFS(dir)
}

type osFS string

func (base osFS) filename(filename string) string {
	return filepath.Join(string(base), filename)
}

func (base osFS) Link(old, new string) error {
	err := os.MkdirAll(filepath.Dir(base.filename(new)), 0777)
	if err != nil {
		return err
	}

	return os.Link(base.filename(old), base.filename(new))
}

func (base osFS) Rename(old, new string) error {
	err := os.MkdirAll(filepath.Dir(base.filename(new)), 0777)
	if err != nil {
		return err
	}

	return os.Rename(base.filename(old), base.filename(new))
}

func (base osFS) New(name string) (appendFile, error) {
	target := base.filename(name)

	err := os.MkdirAll(filepath.Dir(target), 0777)
	if err != nil {
		return nil, err
	}

	if f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644); f != nil {
		return f, err
	} else {
		return nil, err
	}
}

func (base osFS) Open(name string) (readFile, error) {
	if f, err := os.Open(base.filename(name)); f != nil {
		return f, err
	} else {
		return nil, err
	}
}

func (base osFS) Files() ([]string, error) {
	f, err := os.Open(string(base))
	if err != nil {
		if _, ok := err.(*os.PathError); ok || !os.IsNotExist(err) {
			// from the the domain POV these are not errors, just null data
			err = nil
		}

		return nil, err
	}

	names, err := f.Readdirnames(-1)
	sort.Strings(names)
	return names, err
}

func (base osFS) Sub(name string) readFS {
	return osFS(base.filename(name))
}

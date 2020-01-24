package storage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// table driven tests to check osFS and memFS parity
var tests = []struct {
	name string
	body func(t *testing.T, fs fs)
}{
	{"initial", func(t *testing.T, fs fs) {
		files, err := fs.Files()
		if err != nil {
			t.Error(err)
		}

		if len(files) != 0 {
			t.Error("uninitialized fs should have no files")
		}

		f, err := fs.Open("foo")
		if err == nil || f != nil {
			t.Error("opening non existent file should return no reader and an error", err, f)
		}
	}},
	{"write", func(t *testing.T, fs fs) {
		w, err := fs.New("foo")
		if err != nil || w == nil {
			t.Error("new file should have been opened successfully", err)
		}

		_, _ = w.Write([]byte("important\n"))
		_ = w.Sync()
		_ = w.Close()

		files, err := fs.Files()
		if err != nil {
			t.Error(err)
		}

		if len(files) != 1 || files[0] != "foo" {
			t.Error("the new file should be in the list", fmt.Sprintf("%#v", files))
		}

		w2, err := fs.New("foo")
		if err == nil || w2 != nil {
			t.Error("creating duplicate file should return no writer and an error", err, w2)
		}

		r, err := fs.Open("foo")
		if err != nil || r == nil {
			t.Error("written file should have been opened successfully", err)
		}

		b, err := ioutil.ReadAll(r)
		if err != nil {
			t.Error(err)
		}
		if string(b) != "important\n" {
			t.Error("contents of file were not preserved")
		}
	}},
	{"several", func(t *testing.T, fs fs) {
		w1, _ := fs.New("foo")
		files, _ := fs.Files()
		if len(files) != 1 {
			t.Error("the new file should be in the list")
		}

		_, _ = w1.Write([]byte("first\n"))

		w2, _ := fs.New("bar")
		files, _ = fs.Files()
		if len(files) != 2 {
			t.Error("second new file should be in the list")
		}

		_, _ = w2.Write([]byte("second\n"))
		_, _ = w1.Write([]byte("third\n"))

		_ = w2.Sync()
		_ = w2.Close()

		_ = w1.Sync()
		_ = w1.Close()

		files, _ = fs.Files()
		if len(files) != 2 {
			t.Error("second new file should be in the list")
		}

		r1, err := fs.Open("foo")
		if err != nil || r1 == nil {
			t.Error("written file should have been opened successfully", err)
		}

		b1, err := ioutil.ReadAll(r1)
		if err != nil {
			t.Error(err)
		}
		if string(b1) != "first\nthird\n" {
			t.Error("contents of file were not preserved")
		}

		r2, err := fs.Open("bar")
		if err != nil || r2 == nil {
			t.Error("written file should have been opened successfully", err)
		}

		b2, err := ioutil.ReadAll(r2)
		if err != nil {
			t.Error(err)
		}
		if string(b2) != "second\n" {
			t.Error("contents of file were not preserved")
		}
	}},

	// TODO
	// filenames with slashes in them
	// Link
	// Rename
	// Sub
	// link counts

}

func TestFS(t *testing.T) {
	tmpDir, err := ioutil.TempDir(".", "fs-test-tmpdir-")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpDir)

	for fsname, fs := range map[string]func(name string) fs{
		"memfs": func(_ string) fs { return newMemFS() },
		"os": func(name string) fs {
			dir := filepath.Join(tmpDir, name)
			err := os.MkdirAll(dir, 0777)
			if err != nil {
				t.Fatal(err)
			}
			return osFS(dir)
		},
	} {
		t.Run(fsname, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					test.body(t, fs(test.name))
				})
			}
		})
	}
}

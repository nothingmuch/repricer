package storage

import (
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// memory backed implementation of filesystem interfaces, used for testing
// snapshot consistency and to avoid writing to disk in unit tests
type memFS struct {
	sync.Mutex
	m map[string]*memFile
}

var _ fs = &memFS{}

func newMemFS() *memFS {
	return &memFS{m: make(map[string]*memFile)}
}

type memFile struct {
	sync.Mutex
	ops []fileOp
}

type fileOp struct {
	name string
	data string
}

func (f *memFile) log(op fileOp) {
	f.Lock()
	defer f.Unlock()
	f.ops = append(f.ops, op)
}

func (f *memFile) Write(by []byte) (int, error) {
	f.log(fileOp{"write", string(by)})
	return len(by), nil
}
func (f *memFile) Close() error {
	f.log(fileOp{name: "close"})
	return nil
}

func (f *memFile) Sync() error {
	f.log(fileOp{name: "sync"})
	return nil
}

func (m *memFS) Link(old, new string) error {
	m.Lock()
	defer m.Unlock()

	m.m[new] = m.m[old]
	return nil
}

func (m *memFS) Rename(old, new string) error {
	m.Lock()
	defer m.Unlock()

	m.m[new] = m.m[old]
	delete(m.m, old)
	return nil
}

func (m *memFS) New(name string) (appendFile, error) {
	m.Lock()
	defer m.Unlock()

	if _, exists := m.m[name]; exists {
		return nil, fmt.Errorf("already exists")
	}

	f := &memFile{}
	m.m[name] = f
	return f, nil
}

func (m *memFS) Open(name string) (readFile, error) {
	m.Lock()
	file, exists := m.m[name]
	m.Unlock()
	if !exists {
		return nil, fmt.Errorf("no such file")
	}

	var buf bytes.Buffer

	file.Lock()
	defer file.Unlock()

	for _, op := range file.ops {
		switch op.name {
		case "write":
			buf.WriteString(op.data)
		default:
		}
	}

	return &buf, nil
}

func (m *memFS) Files() ([]string, error) {
	files, err := m.allFiles()
	basenames := make([]string, 0, len(files))

	for _, name := range files {
		if strings.IndexByte(name, filepath.Separator) == -1 {
			basenames = append(basenames, name)
		}
	}

	return basenames, err
}

func (m *memFS) allFiles() ([]string, error) {
	m.Lock()
	defer m.Unlock()

	names := make([]string, 0, len(m.m))
	for name := range m.m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (m *memFS) Sub(name string) readFS {
	return subMemFS{name, m}
}

func (src *memFS) clone() *memFS {
	src.Lock()
	defer src.Unlock()

	dst := newMemFS()
	for key, value := range src.m {
		value.Lock()
		dst.m[key] = &memFile{ops: append([]fileOp{}, value.ops...)}
		value.Unlock()
	}
	return dst
}

type subMemFS struct {
	prefix string
	inner  *memFS
}

func (s subMemFS) Sub(name string) readFS {
	return s.inner.Sub(path.Join(s.prefix, name))
}

func (s subMemFS) Open(name string) (readFile, error) {
	return s.inner.Open(path.Join(s.prefix, name))
}

func (s subMemFS) Files() ([]string, error) {
	files, err := s.inner.allFiles()
	start := sort.SearchStrings(files, s.prefix+string(filepath.Separator))
	end := sort.SearchStrings(files, s.prefix+string(filepath.Separator+1))

	files = files[start:end]

	// trim prefix off
	for i, v := range files {
		files[i] = v[len(s.prefix)+1:]
	}

	return files, err
}

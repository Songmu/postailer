package postailer

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

type readSeekCloser interface {
	io.ReadCloser
	io.Seeker
}

// Postailer is main struct of postailer package
type Postailer struct {
	filePath string
	posFile  string

	pos     *position
	oldfile bool
	file    readSeekCloser
}

type position struct {
	Inode uint  `json:"inode"`
	Pos   int64 `json:"pos"`
}

// Open the file with posfile
func Open(filepath, posfile string) (*Postailer, error) {
	pt := &Postailer{
		filePath: filepath,
		posFile:  posfile,
	}
	err := pt.open()
	if err != nil {
		return nil, err
	}
	return pt, nil
}

func loadPos(fname string) (*position, error) {
	pos := &position{}
	_, err := os.Stat(fname)
	if err == nil { // posfile exists
		b, err := ioutil.ReadFile(fname)
		if err == nil {
			err = json.Unmarshal(b, pos)
		}
		return pos, err
	}
	return pos, nil
}

var errFileNotFoundByInode = fmt.Errorf("old file not found")

func (pt *Postailer) open() error {
	pt.pos, _ = loadPos(pt.posFile)
	fi, err := os.Stat(pt.filePath)
	// XXX may be missing the file for a moment while rotating...
	if err != nil {
		return err
	}
	inode := detectInode(fi)
	if pt.pos.Inode > 0 && inode != pt.pos.Inode {
		if oldFile, err := findFileByInode(pt.pos.Inode, filepath.Dir(pt.filePath)); err == nil {
			oldf, err := os.Open(oldFile)
			if err != nil {
				return err
			}
			oldfi, _ := oldf.Stat()
			if oldfi.Size() > pt.pos.Pos {
				oldf.Seek(pt.pos.Pos, io.SeekStart)
				pt.oldfile = true
				pt.file = oldf
				return nil
			}
		} else if err != errFileNotFoundByInode {
			return err
		}
		pt.pos.Pos = 0
	}
	pt.pos.Inode = inode
	f, err := os.Open(pt.filePath)
	if err != nil {
		return err
	}
	if pt.pos.Pos < fi.Size() {
		f.Seek(pt.pos.Pos, io.SeekStart)
	} else {
		pt.pos.Pos = 0
	}
	pt.file = f
	return nil
}

// Read for io.Reader interface
func (pt *Postailer) Read(p []byte) (int, error) {
	n, err := pt.file.Read(p)
	pt.pos.Pos += int64(n)
	if !(pt.oldfile && ((err == nil && n < len(p)) || (err == io.EOF))) {
		return n, err
	}
	pt.file.Close()
	f, err := os.Open(pt.filePath)
	if err != nil {
		return n, err
	}
	fi, err := f.Stat()
	if err != nil {
		return n, err
	}
	pt.pos = &position{Inode: detectInode(fi)}
	pt.oldfile = false
	pt.file = f

	if n == len(p) {
		return n, err
	}
	buf := make([]byte, len(p)-n)
	nn, err := pt.file.Read(buf)
	for i := 0; i < nn; i++ {
		p[n+i] = buf[i]
	}
	pt.pos.Pos = int64(nn)
	return n + nn, err
}

// Seek for io.Seeker interface
func (pt *Postailer) Seek(offset int64, whence int) (int64, error) {
	ret, err := pt.file.Seek(offset, whence)
	if err == nil {
		pt.pos.Pos = ret
	}
	return ret, err
}

// Close for io.Closer interface
func (pt *Postailer) Close() error {
	defer pt.file.Close()
	return savePos(pt.posFile, pt.pos)
}

func savePos(posfile string, pos *position) error {
	b, _ := json.Marshal(pos)
	if err := os.MkdirAll(filepath.Dir(posfile), 0755); err != nil {
		return nil
	}
	return writeFileAtomically(posfile, b)
}

func findFileByInode(inode uint, dir string) (string, error) {
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, fi := range fis {
		if detectInode(fi) == inode {
			return filepath.Join(dir, fi.Name()), nil
		}
	}
	return "", errFileNotFoundByInode
}

func writeFileAtomically(f string, contents []byte) error {
	// MUST be located on same disk partition
	tmpf, err := ioutil.TempFile(filepath.Dir(f), "tmp")
	if err != nil {
		return err
	}
	// os.Remove here works successfully when tmpf.Write fails or os.Rename fails.
	// In successful case, os.Remove fails because the temporary file is already renamed.
	defer os.Remove(tmpf.Name())
	_, err = tmpf.Write(contents)
	tmpf.Close() // should be called before rename
	if err != nil {
		return err
	}
	return os.Rename(tmpf.Name(), f)
}

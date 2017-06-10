package postailer

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
)

type Postailer struct {
	FilePath string
	PosFile  string

	pos     *position
	oldfile bool
	rcloser io.ReadCloser
}

type position struct {
	Inode uint  `json:"inode"`
	Pos   int64 `json:"pos"`
}

func Open(filepath, posfile string) (*Postailer, error) {
	pt := &Postailer{
		FilePath: filepath,
		PosFile:  posfile,
	}
	err := pt.Open()
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
			err := json.Unmarshal(b, pos)
			return pos, err
		}
	}
	return pos, nil
}

func (pt *Postailer) Open() error {
	// XXX error handling
	pt.pos, _ = loadPos(pt.PosFile)
	fi, err := os.Stat(pt.FilePath)
	// XXX may be missing the file for a moment while rotating...
	if err != nil {
		return err
	}
	inode := detectInode(fi)
	if pt.pos.Inode > 0 && inode != pt.pos.Inode {
		if oldFile, err := findFileByInode(pt.pos.Inode, filepath.Dir(pt.FilePath)); err != nil {
			oldf, err := os.Open(oldFile)
			if err != nil {
				return err
			}
			oldfi, _ := oldf.Stat()
			if oldfi.Size() > pt.pos.Pos {
				oldf.Seek(pt.pos.Pos, 0)
				pt.oldfile = true
				pt.rcloser = oldf
				return nil
			}
		}
		// XXX error handling?
	}
	pt.pos.Inode = inode
	f, err := os.Open(pt.FilePath)
	if err != nil {
		return err
	}
	if pt.pos.Pos > fi.Size() {
		f.Seek(pt.pos.Pos, 0)
	} else {
		pt.pos.Pos = 0
	}
	pt.rcloser = f
	return nil
}

func (pt *Postailer) Read(p []byte) (int, error) {
	n, err := pt.rcloser.Read(p)
	pt.pos.Pos += int64(n)
	if err == nil || err != io.EOF || !pt.oldfile {
		return n, err
	}
	pt.rcloser.Close()
	f, err := os.Open(pt.FilePath)
	if err != nil {
		return n, err
	}
	fi, err := f.Stat()
	if err != nil {
		return n, err
	}
	pt.pos = &position{Inode: detectInode(fi)}
	pt.oldfile = false
	pt.rcloser = f

	if n == len(p) {
		return n, err
	}
	buf := make([]byte, len(p)-n)
	nn, err := pt.rcloser.Read(buf)
	for i := 0; i < nn; i++ {
		p[n+i] = buf[i]
	}
	pt.pos.Pos = int64(nn)
	return n + nn, err
}

func (pt *Postailer) Close() error {
	defer pt.rcloser.Close()
	return savePos(pt.PosFile, pt.pos)
}

func savePos(posfile string, pos *position) error {
	b, _ := json.Marshal(pos)
	return writeFileAtomically(posfile, b)
}

// XXX may be dirty, should be return error?
func detectInode(fi os.FileInfo) uint {
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint(stat.Ino)
	}
	return 0
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
	// XXX rough error
	return "", fmt.Errorf("file not found")
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

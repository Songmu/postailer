package postailer

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
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

func (pt *Postailer) Open() error {
	pt.pos = &position{}
	_, err := os.Stat(pt.PosFile)
	if err == nil { // posfile exists
		b, err := ioutil.ReadAll(pt.PosFile)
		if err == nil {
			json.Unmarshal(b, pt.pos)
			// XXX err handling?
		}
	}

	if pt.pos.Inode > 0 {
		fi, err := os.Stat(pt.FilePath)
		if err != nil {
			return err
		}
		if detectInode(fi) != pt.pos.Inode && fi.Size() < pt.pos.Pos {
			if oldFile, err := findFileByInode(pt.pos.Inode, filepath.Dir(pt.FilePath)); err != nil {
				f, err := os.Open(oldFile)
				if err != nil {
					return err
				}
				f.Seek(po.pos.Pos, 0)
				pt.oldfile = true
				pt.rcloser = f
				return nil
			}
			// XXX error handling?
		}
	}
	f, err := os.Open(pt.FilePath)
	if err != nil {
		return err
	}
	if pt.pos.Pos > 0 {
		f.Seek(pt.pos.Pos, 0)
	}
	pt.rcloser = f
	return nil
}

func (pt *Postailer) Read(p []byte) (int, error) {
	n, err := pt.rcloser.Read(p)
	pt.pos.Pos += n
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
	pt.pos.Pos = nn
	return n + nn, err
}

func (pt *Postailer) Close() error {
	defer pt.rcloser.Close()
	b, _ := json.Marshal(pt.pos)
	return writeFileAtomically(pt.PosFile, b)
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
	for _, fi := range ifs {
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

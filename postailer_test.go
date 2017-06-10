package postailer

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fileAndPos(dir string) (string, string) {
	fname := filepath.Join(dir, "log.log")
	posfile := fname + ".json"
	return fname, posfile
}

var readTests = []struct {
	Name    string
	Prepare func() (string, error)
	Output1 []byte
	N1      int
	Error1  error
	Output2 []byte
	N2      int
	Error2  error
	NextPos int64
}{
	{
		Name: "New",
		Prepare: func() (string, error) {
			tmpd, err := ioutil.TempDir("", "")
			if err != nil {
				return "", err
			}
			fname, _ := fileAndPos(tmpd)
			ioutil.WriteFile(fname, []byte("abcdf13579"), 0644)
			return tmpd, nil
		},
		Output1: []byte{'a', 'b', 'c', 'd', 'f'},
		N1:      5,
		Error1:  nil,
		Output2: []byte{'1', '3', '5'},
		N2:      3,
		Error2:  nil,
		NextPos: 8,
	},
	{
		Name: "Simple",
		Prepare: func() (string, error) {
			tmpd, err := ioutil.TempDir("", "")
			if err != nil {
				return "", err
			}
			fname, posfile := fileAndPos(tmpd)
			ioutil.WriteFile(fname, []byte("-skip-abcdf13579"), 0644)
			fi, _ := os.Stat(fname)
			savePos(posfile, &position{
				Inode: detectInode(fi),
				Pos:   6,
			})
			return tmpd, nil
		},
		Output1: []byte{'a', 'b', 'c', 'd', 'f'},
		N1:      5,
		Error1:  nil,
		Output2: []byte{'1', '3', '5'},
		N2:      3,
		Error2:  nil,
		NextPos: 14,
	},
	{
		Name: "EOF",
		Prepare: func() (string, error) {
			tmpd, err := ioutil.TempDir("", "")
			if err != nil {
				return "", err
			}
			fname, _ := fileAndPos(tmpd)
			ioutil.WriteFile(fname, []byte("abcd"), 0644)
			return tmpd, nil
		},
		Output1: []byte{'a', 'b', 'c', 'd', 0},
		N1:      4,
		Error1:  nil,
		Output2: []byte{0, 0, 0},
		N2:      0,
		Error2:  io.EOF,
		NextPos: 4,
	},
	{
		Name: "Roteted",
		Prepare: func() (string, error) {
			tmpd, err := ioutil.TempDir("", "")
			if err != nil {
				return "", err
			}
			fname, posfile := fileAndPos(tmpd)
			ioutil.WriteFile(fname, []byte("-skip-abcd"), 0644)
			fi, _ := os.Stat(fname)
			savePos(posfile, &position{
				Inode: detectInode(fi),
				Pos:   6,
			})
			os.Rename(fname, fname+".bak")
			ioutil.WriteFile(fname, []byte("f135999"), 0644)
			return tmpd, nil
		},
		Output1: []byte{'a', 'b', 'c', 'd', 'f'},
		N1:      5,
		Error1:  nil,
		Output2: []byte{'1', '3', '5'},
		N2:      3,
		Error2:  nil,
		NextPos: 4,
	},
	{
		Name: "Just Roteted",
		Prepare: func() (string, error) {
			tmpd, err := ioutil.TempDir("", "")
			if err != nil {
				return "", err
			}
			fname, posfile := fileAndPos(tmpd)
			ioutil.WriteFile(fname, []byte("-skip-abcdf"), 0644)
			fi, _ := os.Stat(fname)
			savePos(posfile, &position{
				Inode: detectInode(fi),
				Pos:   6,
			})
			os.Rename(fname, fname+".bak")
			ioutil.WriteFile(fname, []byte("135999"), 0644)
			return tmpd, nil
		},
		Output1: []byte{'a', 'b', 'c', 'd', 'f'},
		N1:      5,
		Error1:  nil,
		Output2: []byte{'1', '3', '5'},
		N2:      3,
		Error2:  nil,
		NextPos: 3,
	},
	{
		Name: "Roteted but missing",
		Prepare: func() (string, error) {
			tmpd, err := ioutil.TempDir("", "")
			if err != nil {
				return "", err
			}
			fname, posfile := fileAndPos(tmpd)
			savePos(posfile, &position{
				Inode: ^uint(0) - 10,
				Pos:   6,
			})
			ioutil.WriteFile(fname, []byte("abcdf13579"), 0644)
			return tmpd, nil
		},
		Output1: []byte{'a', 'b', 'c', 'd', 'f'},
		N1:      5,
		Error1:  nil,
		Output2: []byte{'1', '3', '5'},
		N2:      3,
		Error2:  nil,
		NextPos: 8,
	},
	{
		Name: "Same inode, but Truncated",
		Prepare: func() (string, error) {
			tmpd, err := ioutil.TempDir("", "")
			if err != nil {
				return "", err
			}
			fname, posfile := fileAndPos(tmpd)
			ioutil.WriteFile(fname, []byte("abcdf13579"), 0644)
			fi, _ := os.Stat(fname)
			savePos(posfile, &position{
				Inode: detectInode(fi),
				Pos:   10000,
			})
			return tmpd, nil
		},
		Output1: []byte{'a', 'b', 'c', 'd', 'f'},
		N1:      5,
		Error1:  nil,
		Output2: []byte{'1', '3', '5'},
		N2:      3,
		Error2:  nil,
		NextPos: 8,
	},
}

func TestReadClose(t *testing.T) {
	for _, tt := range readTests {
		func() {
			tmpd, err := tt.Prepare()
			if tmpd != "" {
				defer os.RemoveAll(tmpd)
			}
			if err != nil {
				t.Fatalf("error occured while preparing test: %s, %+v", tt.Name, err)
			}
			fname, posfile := fileAndPos(tmpd)
			pt, err := Open(fname, posfile)
			if err != nil {
				t.Fatalf("error should be nil but: %+v", err)
			}

			p1 := make([]byte, 5)
			n1, err1 := pt.Read(p1)
			if !reflect.DeepEqual(p1, tt.Output1) {
				t.Errorf("%s(output1): out=%q want %q", tt.Name, p1, tt.Output1)
			}
			if n1 != tt.N1 {
				t.Errorf("%s(n1): out=%d want %d", tt.Name, n1, tt.N1)
			}
			if err1 != tt.Error1 {
				t.Errorf("%s(err1): out=%+v want %+v", tt.Name, err1, tt.Error1)
			}

			p2 := make([]byte, 3)
			n2, err2 := pt.Read(p2)
			if !reflect.DeepEqual(p2, tt.Output2) {
				t.Errorf("%s(output2): out=%q want %q", tt.Name, p2, tt.Output2)
			}
			if n2 != tt.N2 {
				t.Errorf("%s(n2): out=%d want %d", tt.Name, n2, tt.N2)
			}
			if err2 != tt.Error2 {
				t.Errorf("%s(err2): out=%+v want %+v", tt.Name, err2, tt.Error2)
			}

			pt.Close()
			pos, _ := loadPos(posfile)
			fi, _ := os.Stat(fname)
			expect := &position{
				Inode: detectInode(fi),
				Pos:   tt.NextPos,
			}
			if !reflect.DeepEqual(pos, expect) {
				t.Errorf("%s(nextPos): out=%+v want %+v", tt.Name, pos, expect)
			}
		}()
	}
}

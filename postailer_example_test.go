// +build !windows

package postailer_test

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Songmu/postailer"
)

func ExamplePostailer_Read() {
	appendFile := func(fname, content string) error {
		f, _ := os.OpenFile(fname, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		f.WriteString(content)
		return f.Close()
	}

	tmpd, _ := ioutil.TempDir("", "example")
	if tmpd != "" {
		defer os.RemoveAll(tmpd)
	}
	logfile := filepath.Join(tmpd, "log.log")
	posfile := logfile + ".json"

	appendFile(logfile, "1\n22\n")
	{
		pt, _ := postailer.Open(logfile, posfile)
		s := bufio.NewScanner(pt)
		for s.Scan() {
			fmt.Println(s.Text())
		}
		pt.Close()
	}

	appendFile(logfile, "333\n")
	{
		pt, _ := postailer.Open(logfile, posfile)
		s := bufio.NewScanner(pt)
		for s.Scan() {
			fmt.Println(s.Text())
		}
		pt.Close()
	}

	appendFile(logfile, "4444\n")
	os.Rename(logfile, logfile+".bak")
	appendFile(logfile, "newfile:1\nnewfile:22")
	{
		pt, _ := postailer.Open(logfile, posfile)
		s := bufio.NewScanner(pt)
		for s.Scan() {
			fmt.Println(s.Text())
		}
		pt.Close()
	}
	// Output:
	// 1
	// 22
	// 333
	// 4444
	// newfile:1
	// newfile:22
}

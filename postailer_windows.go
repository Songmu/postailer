package postailer

import "os"

func detectInode(_ os.FileInfo) uint {
	return 0
}

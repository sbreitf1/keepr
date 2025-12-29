package destination

type Interface interface {
	ReadDir(relPath string) ([]FileInfo, error)
	FileExists(relPath string) (bool, error)
	ReadFile(relPath string) ([]byte, error)
	WriteFile(relPath string, data []byte) error
	//TODO open file for writing data and set length
	DeleteDir(relPath string) error
	CreateDir(relPath string) error

	IsNotExists(err error) bool
}

type FileInfo struct {
	Name  string
	IsDir bool
}

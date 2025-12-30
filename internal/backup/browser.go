package backup

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sbreitf1/keepr/internal/backup/destination"
)

type Browser struct {
	backupSet   *BackupSet
	snapshot    *Snapshot
	dest        destination.Interface
	blobIndex   map[BlobID]blobLen
	prefixIndex map[string]prefix
}

func NewBrowser(backupSet *BackupSet, snapshot *Snapshot) (*Browser, error) {
	dest, err := backupSet.OpenDestination()
	if err != nil {
		return nil, fmt.Errorf("init destination: %w", err)
	}

	blobIndex, err := backupSet.ReadBlobIndex(dest)
	if err != nil {
		return nil, fmt.Errorf("read blob index: %w", err)
	}

	return &Browser{
		backupSet:   backupSet,
		snapshot:    snapshot,
		dest:        dest,
		blobIndex:   blobIndex,
		prefixIndex: buildPrefixIndex(backupSet),
	}, nil
}

type prefix struct {
	prefix              string
	directChilds        []string
	directChildPrefixes []string
}

func buildPrefixIndex(backupSet *BackupSet) map[string]prefix {
	//TODO build tree
	return nil
}

func (browser *Browser) IsDir(path string) (bool, error) {
	//TODO use tree

	if len(path) == 0 {
		return true, nil
	}

	path = strings.Trim(path, "/") + "/"
	for _, f := range browser.snapshot.Files {
		if strings.HasPrefix(f.Path, path) {
			return true, nil
		}
	}
	return false, nil
}

func (browser *Browser) GetFile(path string) (FileSnapshot, bool, error) {
	path = strings.Trim(path, "/")
	for _, f := range browser.snapshot.Files {
		if f.Path == path {
			return f, true, nil
		}
	}
	return FileSnapshot{}, false, nil
}

func (browser *Browser) FileName(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func (browser *Browser) ListDirs(path string) ([]string, error) {
	subDirs := make(map[string]struct{}, 0)
	path = strings.Trim(path, "/")
	if len(path) > 0 {
		path += "/"
	}
	for _, f := range browser.snapshot.Files {
		if strings.HasPrefix(f.Path, path) {
			parts := strings.Split(f.Path[len(path):], "/")
			if len(parts) > 1 {
				subDirs[parts[0]] = struct{}{}
			}
		}
	}
	dirs := make([]string, 0, len(subDirs))
	for dir := range subDirs {
		dirs = append(dirs, dir)
	}
	return dirs, nil
}

func (browser *Browser) ListFiles(path string) ([]FileSnapshot, error) {
	path = strings.Trim(path, "/")
	files := make([]FileSnapshot, 0)
	for _, f := range browser.snapshot.Files {
		parts := strings.Split(f.Path, "/")
		dir := strings.Join(parts[:len(parts)-1], "/")
		if path == dir {
			files = append(files, f)
		}
	}
	return files, nil
}

func (browser *Browser) OpenFile(path string) (io.ReadSeekCloser, error) {
	file, exists, err := browser.GetFile(path)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, os.ErrNotExist
	}
	return &backupFileReader{browser: browser, file: file}, nil
}

type backupFileReader struct {
	browser    *Browser
	file       FileSnapshot
	currentPos int64
}

func (r *backupFileReader) Read(p []byte) (int, error) {
	blobID, blobOffset, ok := r.findBlobIDAndOffset(r.currentPos)
	if !ok {
		return 0, io.EOF
	}

	blobPath := r.browser.snapshot.GetBlobPath(blobID)
	data, err := r.browser.dest.ReadFile(blobPath)
	if err != nil {
		return 0, err
	}
	n := min(len(p), len(data)-int(blobOffset))
	copy(p[:n], data[blobOffset:int(blobOffset)+n])
	r.currentPos += int64(n)
	return n, nil
}

func (r *backupFileReader) findBlobIDAndOffset(pos int64) (BlobID, int64, bool) {
	var blobsPos int64
	for _, blobID := range r.file.Blobs {
		blobLen, ok := r.browser.blobIndex[blobID]
		if !ok {
			fmt.Println("WARN: blob", blobID.String(), "is missing in index")
			return BlobID{}, 0, false
		}
		if pos >= blobsPos && pos < (blobsPos+int64(blobLen)) {
			return blobID, pos - blobsPos, true
		}
		blobsPos += int64(blobLen)
	}
	return BlobID{}, 0, false
}

func (r *backupFileReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekCurrent:
		r.currentPos += offset
	case io.SeekStart:
		r.currentPos = offset
	case io.SeekEnd:
		r.currentPos = int64(r.file.Size) + offset
	default:
		return r.currentPos, fmt.Errorf("unsupported whence %v", whence)
	}
	return r.currentPos, nil
}

func (r *backupFileReader) Close() error {
	return nil
}

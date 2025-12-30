package serve

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sbreitf1/keepr/internal/backup"

	"golang.org/x/net/webdav"
)

func ServeWebDAV(browser *backup.Browser) error {
	var handler webdav.Handler
	handler.FileSystem = &webDAVFS{browser: browser}
	handler.LockSystem = webdav.NewMemLS()
	handler.Logger = func(r *http.Request, err error) {
		//fmt.Println("DAV:", r.Method, r.URL, "->", err)
	}
	return http.ListenAndServe("127.0.0.1:8080", &handler)
}

type webDAVFS struct {
	browser *backup.Browser
}

func (wfs *webDAVFS) GetDestPath(name string) string {
	return strings.Trim(strings.ReplaceAll(name, "\\", "/"), "/")
}

func (wfs *webDAVFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return fmt.Errorf("fs is read-only")
}

func (wfs *webDAVFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (result webdav.File, errResult error) {
	//TODO check flags and perm

	isDir, err := wfs.browser.IsDir(wfs.GetDestPath(name))
	if err != nil {
		return nil, err
	}
	if isDir {
		return &davDir{wfs: wfs, path: wfs.GetDestPath(name)}, nil
	}

	file, fileExists, err := wfs.browser.GetFile(wfs.GetDestPath(name))
	if err != nil {
		return nil, err
	}
	if fileExists {
		r, err := wfs.browser.OpenFile(wfs.GetDestPath(name))
		if err != nil {
			return nil, err
		}
		return &davFile{
			wfs:    wfs,
			file:   file,
			reader: r,
		}, nil
	}
	return nil, os.ErrNotExist
}

func (wfs *webDAVFS) RemoveAll(ctx context.Context, name string) error {
	return fmt.Errorf("fs is read-only")
}

func (wfs *webDAVFS) Rename(ctx context.Context, oldName, newName string) error {
	return fmt.Errorf("fs is read-only")
}

func (wfs *webDAVFS) Stat(ctx context.Context, name string) (result os.FileInfo, errResult error) {
	isDir, err := wfs.browser.IsDir(wfs.GetDestPath(name))
	if err != nil {
		return nil, err
	}
	if isDir {
		return wfs.newDAVFileInfoForDir(wfs.GetDestPath(name)), nil
	}

	file, fileExists, err := wfs.browser.GetFile(wfs.GetDestPath(name))
	if err != nil {
		return nil, err
	}
	if fileExists {
		return wfs.newDAVFileInfoForFile(file), nil
	}
	return nil, os.ErrNotExist
}

type davFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

func (wfs *webDAVFS) newDAVFileInfoForDir(path string) *davFileInfo {
	return &davFileInfo{
		name:    wfs.browser.FileName(path),
		size:    0,
		mode:    os.ModeDir,
		modTime: time.Date(2025, time.December, 30, 16, 9, 0, 0, time.Local),
		isDir:   true,
	}
}

func (wfs *webDAVFS) newDAVFileInfoForFile(file backup.FileSnapshot) *davFileInfo {
	return &davFileInfo{
		name:    wfs.browser.FileName(file.Path),
		size:    int64(file.Size),
		mode:    0644,
		modTime: file.LastModified,
		isDir:   false,
	}
}

func (fi *davFileInfo) Name() string {
	return fi.name
}

func (fi *davFileInfo) Size() int64 {
	return fi.size
}

func (fi *davFileInfo) Mode() fs.FileMode {
	return fi.mode
}

func (fi *davFileInfo) ModTime() time.Time {
	return fi.modTime
}

func (fi *davFileInfo) IsDir() bool {
	return fi.isDir
}

func (fi *davFileInfo) Sys() any {
	return nil
}

type davDir struct {
	wfs  *webDAVFS
	path string
}

func (d *davDir) Readdir(count int) ([]fs.FileInfo, error) {
	content := make([]fs.FileInfo, 0)

	dirs, err := d.wfs.browser.ListDirs(d.path)
	if err != nil {
		return nil, err
	}
	for _, subDir := range dirs {
		content = append(content, d.wfs.newDAVFileInfoForDir(subDir))
	}

	files, err := d.wfs.browser.ListFiles(d.path)
	if err != nil {
		return nil, err
	}
	for _, subFile := range files {
		content = append(content, d.wfs.newDAVFileInfoForFile(subFile))
	}

	return content, nil
}

func (d *davDir) Stat() (fs.FileInfo, error) {
	return d.wfs.newDAVFileInfoForDir(d.path), nil
}

func (d *davDir) Read(p []byte) (n int, err error) {
	fmt.Println("illegal call to davDir.Read")
	return 0, fmt.Errorf("davDir.Read not allowed")
}

func (d *davDir) Write(p []byte) (n int, err error) {
	fmt.Println("illegal call to davDir.Write")
	return 0, fmt.Errorf("davDir.Write not allowed")
}

func (d *davDir) Seek(offset int64, whence int) (int64, error) {
	fmt.Println("illegal call to davDir.Seek")
	return 0, fmt.Errorf("davDir.Seek not allowed")
}

func (d *davDir) Close() error {
	return nil
}

type davFile struct {
	wfs    *webDAVFS
	file   backup.FileSnapshot
	reader io.ReadSeekCloser
}

func (f *davFile) String() string {
	return fmt.Sprintf("davFile[%q,%d,%v,%v]", f.file.Path, f.file.Size, f.file.LastModified, f.file.Blobs)
}

func (f *davFile) Readdir(count int) ([]fs.FileInfo, error) {
	fmt.Println("illegal call to davFile.Readdir")
	return nil, fmt.Errorf("davFile.Readdir not allowed")
}

func (f *davFile) Stat() (fs.FileInfo, error) {
	return f.wfs.newDAVFileInfoForFile(f.file), nil
}

func (f *davFile) Read(p []byte) (n int, err error) {
	return f.reader.Read(p)
}

func (f *davFile) Write(p []byte) (n int, err error) {
	fmt.Println("illegal call to davFile.Write")
	return 0, fmt.Errorf("davFile.Write not allowed")
}

func (f *davFile) Seek(offset int64, whence int) (int64, error) {
	return f.reader.Seek(offset, whence)
}

func (f *davFile) Close() error {
	return f.reader.Close()
}

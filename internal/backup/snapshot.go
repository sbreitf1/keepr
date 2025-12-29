package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sbreitf1/keepr/internal/backup/destination"
)

const (
	blobSize = uint64(50 * 1024 * 1024)
)

type Snapshotter interface {
	TakeSnapshot() error
}

type snapshotter struct {
	backupSet *BackupSet
}

type snapshotContext struct {
	relPath           string
	dest              destination.Interface
	snapshot          *snapshot
	previousSnapshot  *snapshot
	existingBlobIDs   map[blobID]blobLen
	referencedBlobIDs map[blobID]blobLen
	uploadedBlobIDs   map[blobID]blobLen
}

type snapshot struct {
	CreatedAt time.Time
	Files     map[string]fileSnapshot
	TotalSize uint64
}

type fileSnapshot struct {
	Path         string
	LastModified time.Time
	Size         uint64
	Blobs        []blobID
}

type blob struct {
	ID      blobID
	Content []byte
}

func NewSnapshotter(backupSet *BackupSet) (Snapshotter, error) {
	if !filepath.IsAbs(backupSet.conf.Source.Path) {
		return nil, fmt.Errorf("source path must be absolute")
	}
	//TODO check source exists

	if len(backupSet.conf.Destinations) == 0 {
		return nil, fmt.Errorf("missing destination")
	}
	if len(backupSet.conf.Destinations) > 1 {
		return nil, fmt.Errorf("multiple destinations not yet supported")
	}

	return &snapshotter{
		backupSet: backupSet,
	}, nil
}

func (snapshotter *snapshotter) TakeSnapshot() error {
	dest, err := destination.NewLocalDir(snapshotter.backupSet.conf.Destinations[0].LocalFileSystem)
	if err != nil {
		return fmt.Errorf("init local dir destination: %w", err)
	}

	snapshot := &snapshot{
		CreatedAt: time.Now(),
	}

	existingBlobs, err := snapshotter.backupSet.ReadBlobIndex(dest)
	if err != nil {
		return fmt.Errorf("read blob index: %w", err)
	}

	ctx := &snapshotContext{
		relPath:           snapshot.CreatedAt.UTC().Format("20060102T150405Z"),
		dest:              dest,
		snapshot:          snapshot,
		existingBlobIDs:   existingBlobs,
		referencedBlobIDs: make(map[blobID]blobLen),
		uploadedBlobIDs:   make(map[blobID]blobLen),
	}

	previousSnapshot, err := GetLatestSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("get previous snapshot: %w", err)
	}
	ctx.previousSnapshot = previousSnapshot
	if ctx.previousSnapshot != nil {
		fmt.Println("previous snapshot was", ctx.previousSnapshot.CreatedAt)
	} else {
		fmt.Println("no previous snapshot found")
	}

	if err := snapshotter.gatherFiles(ctx); err != nil {
		return fmt.Errorf("gather files for backup: %w", err)
	}
	fmt.Println("found", len(ctx.snapshot.Files), "files for backup with a total size of", ctx.snapshot.TotalSize)

	if err := snapshotter.uploadBlobs(ctx); err != nil {
		return fmt.Errorf("upload blobs: %w", err)
	}
	fmt.Println("uploaded", len(ctx.uploadedBlobIDs), "blobs of total", len(ctx.referencedBlobIDs), "referenced")

	if err := snapshot.WriteIndex(ctx); err != nil {
		return fmt.Errorf("write snapshot index: %w", err)
	}

	/*
		1. download latest backup index
		2. download blob indices from all backups
		3. detected changed or added files (last-modified + file-len compared to latest backup)
		4. iterate over all changed files
			a) unchanged
				add same blob to new backup index
			b) added or changed
				iterate over file blobs (16mb blocks). only upload non-existing blobs (not referenced in any of the blob indices)

				sha1 for blob-ids and file checksums?


				same key for all files and blocks. random IV (16 bytes) at beginning of each block
				-> on blob = 16 bytes IV + 16mb encrypted data (+ aes padding)
	*/

	if err := snapshotter.UpdateBlobIndex(ctx); err != nil {
		return fmt.Errorf("update blob index: %w", err)
	}

	return nil
}

func (snapshotter *snapshotter) gatherFiles(ctx *snapshotContext) error {
	ctx.snapshot.Files = make(map[string]fileSnapshot)
	return filepath.Walk(snapshotter.backupSet.conf.Source.Path, func(path string, fi fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		//TODO skip dir with return ErrSkipDir

		if !fi.IsDir() {
			//TODO skip files
			relPath := strings.ReplaceAll(strings.TrimLeft(path[len(snapshotter.backupSet.conf.Source.Path):], "/\\"), "\\", "/")
			ctx.snapshot.Files[relPath] = fileSnapshot{
				Path:         relPath,
				LastModified: fi.ModTime(),
				Size:         uint64(fi.Size()),
			}
			ctx.snapshot.TotalSize += uint64(fi.Size())
		}
		return nil
	})
}

func (snapshotter *snapshotter) uploadBlobs(ctx *snapshotContext) error {
	for relPath := range ctx.snapshot.Files {
		if err := snapshotter.uploadBlobsOfFile(ctx, relPath); err != nil {
			return fmt.Errorf("upload file blobs of %q: %w", relPath, err)
		}
	}
	return nil
}

func (snapshotter *snapshotter) uploadBlobsOfFile(ctx *snapshotContext, relPath string) error {
	file := ctx.snapshot.Files[relPath]

	if ctx.previousSnapshot != nil {
		if previousFile, ok := ctx.previousSnapshot.Files[relPath]; ok {
			if previousFile.LastModified.UnixMilli() == file.LastModified.UnixMilli() {
				file.Blobs = make([]blobID, 0, len(previousFile.Blobs))
				for _, blobID := range previousFile.Blobs {
					file.Blobs = append(file.Blobs, blobID)
					if blobLen, ok := ctx.existingBlobIDs[blobID]; ok {
						ctx.referencedBlobIDs[blobID] = blobLen
					} else {
						//TODO upload missing blobs
						return fmt.Errorf("file %q is unchanged, but blobs in dest are missing", relPath)
					}
				}
				ctx.snapshot.Files[relPath] = file
				return nil
			}
		}
	}

	buf := make([]byte, blobSize)

	path := filepath.Join(snapshotter.backupSet.conf.Source.Path, strings.ReplaceAll(relPath, "/", string(filepath.Separator)))
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	file.Blobs = make([]blobID, 0, file.Size/blobSize+1)
	for i := uint64(0); i < file.Size; i += blobSize {
		remainingSize := file.Size - i
		readLen := min(blobSize, remainingSize)
		if _, err := io.ReadFull(f, buf[:readLen]); err != nil {
			return err
		}

		blob, err := snapshotter.prepareBlob(ctx, buf[:readLen])
		if err != nil {
			return err
		}
		file.Blobs = append(file.Blobs, blob.ID)
		ctx.referencedBlobIDs[blob.ID] = blobLen(readLen)

		if _, ok := ctx.existingBlobIDs[blob.ID]; !ok {
			if err := snapshotter.WriteBlob(ctx, blob); err != nil {
				return err
			}
			ctx.uploadedBlobIDs[blob.ID] = blobLen(readLen)
		}
	}
	ctx.snapshot.Files[relPath] = file
	return nil
}

func (snapshotter *snapshotter) prepareBlob(_ *snapshotContext, content []byte) (*blob, error) {
	hash := sha256.Sum256(content)
	return &blob{
		ID:      hash,
		Content: content,
	}, nil
}

func (snapshotter *snapshotter) WriteBlob(ctx *snapshotContext, blob *blob) error {
	blobIDStr := blob.ID.String()
	blobDir := ".blobs/" + blobIDStr[0:2] + "/" + blobIDStr[2:4] + "/" + blobIDStr[4:6] + "/" + blobIDStr[6:8]
	blobPath := blobDir + "/" + blobIDStr[8:]
	if err := ctx.dest.CreateDir(blobDir); err != nil {
		return err
	}
	//TODO encrypt
	return ctx.dest.WriteFile(blobPath, blob.Content)
}

func (snapshotter *snapshotter) UpdateBlobIndex(ctx *snapshotContext) error {
	blobs, err := snapshotter.backupSet.ReadBlobIndex(ctx.dest)
	if err != nil {
		return err
	}

	for id, blobLen := range ctx.referencedBlobIDs {
		blobs[id] = blobLen
	}

	return snapshotter.backupSet.WriteBlobIndex(ctx.dest, blobs)
}

func (snapshot *snapshot) WriteIndex(ctx *snapshotContext) error {
	w := bytes.NewBuffer(nil)

	// version
	if err := w.WriteByte(0); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, uint64(snapshot.CreatedAt.Unix())); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, snapshot.TotalSize); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(snapshot.Files))); err != nil {
		return err
	}
	for _, f := range snapshot.Files {
		if err := writeStr(w, f.Path); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, uint64(f.LastModified.UnixMilli())); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, f.Size); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, uint32(len(f.Blobs))); err != nil {
			return err
		}
		for _, blobID := range f.Blobs {
			if _, err := w.Write(blobID[:]); err != nil {
				return err
			}
		}
	}

	return ctx.dest.WriteFile(snapshot.CreatedAt.UTC().Format("20060102T150405Z/.snapshot"), w.Bytes())
}

func ReadSnapshotIndex(ctx *snapshotContext, path string) (*snapshot, error) {
	data, err := ctx.dest.ReadFile(path)
	if err != nil {
		return nil, err
	}

	r := bytes.NewReader(data)

	version, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if version != 0 {
		return nil, fmt.Errorf("unsupported snapshot version %d", version)
	}

	snapshot := &snapshot{}

	var createdAt uint64
	if err := binary.Read(r, binary.LittleEndian, &createdAt); err != nil {
		return nil, err
	}
	snapshot.CreatedAt = time.Unix(int64(createdAt), 0)

	if err := binary.Read(r, binary.LittleEndian, &snapshot.TotalSize); err != nil {
		return nil, err
	}

	var fileCount uint32
	if err := binary.Read(r, binary.LittleEndian, &fileCount); err != nil {
		return nil, err
	}
	snapshot.Files = make(map[string]fileSnapshot, fileCount)
	for range fileCount {
		var file fileSnapshot

		path, err := readStr(r)
		if err != nil {
			return nil, err
		}
		file.Path = path

		var lastModifiedMillis uint64
		if err := binary.Read(r, binary.LittleEndian, &lastModifiedMillis); err != nil {
			return nil, err
		}
		file.LastModified = time.UnixMilli(int64(lastModifiedMillis))

		if err := binary.Read(r, binary.LittleEndian, &file.Size); err != nil {
			return nil, err
		}

		var blobCount uint32
		if err := binary.Read(r, binary.LittleEndian, &blobCount); err != nil {
			return nil, err
		}
		file.Blobs = make([]blobID, 0, blobCount)
		for range blobCount {
			var blobID blobID
			if err := binary.Read(r, binary.LittleEndian, &blobID); err != nil {
				return nil, err
			}
			file.Blobs = append(file.Blobs, blobID)
		}

		snapshot.Files[path] = file
	}

	return snapshot, nil
}

func ListSnapshots(ctx *snapshotContext) ([]*snapshot, error) {
	files, err := ctx.dest.ReadDir("")
	if err != nil {
		return nil, err
	}
	snapshots := make([]*snapshot, 0)
	for _, fi := range files {
		if fi.IsDir {
			_, err := time.Parse("20060102T150405Z", fi.Name)
			if err != nil {
				continue
			}
			if exists, err := ctx.dest.FileExists(fi.Name + "/.snapshot"); err != nil || !exists {
				continue
			}

			snapshot, err := ReadSnapshotIndex(ctx, fi.Name+"/.snapshot")
			if err != nil {
				return nil, err
			}
			snapshots = append(snapshots, snapshot)
		}
	}
	return snapshots, nil
}

func GetLatestSnapshot(ctx *snapshotContext) (*snapshot, error) {
	snapshots, err := ListSnapshots(ctx)
	if err != nil {
		return nil, err
	}

	var latestSnapshot *snapshot
	for _, snapshot := range snapshots {
		if latestSnapshot == nil || snapshot.CreatedAt.After(latestSnapshot.CreatedAt) {
			latestSnapshot = snapshot
		}
	}

	return latestSnapshot, nil
}

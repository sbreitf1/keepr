package backup

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/sbreitf1/keepr/internal/backup/destination"
)

type BackupSetConfig struct {
	Name         string
	Encryption   BackupSetEncryptionConfig
	Source       BackupSourceLocalDirConfig
	Destinations []destination.Config
}

type BackupSetEncryptionConfig struct {
	Enabled      bool
	PasswordHash [32]byte
	PasswordSalt []byte
}

type BackupSourceConfig struct {
	Name     string
	LocalDir BackupSourceLocalDirConfig
}

type BackupSourceLocalDirConfig struct {
	Path         string
	ExcludePaths []string
}

type BackupSet struct {
	conf BackupSetConfig
}

func NewBackupSetFromConfig(conf BackupSetConfig) (*BackupSet, error) {
	//TODO validate config

	return &BackupSet{conf: conf}, nil
}

func (backupSet *BackupSet) OpenDestination() (destination.Interface, error) {
	dest, err := destination.NewLocalDir(backupSet.conf.Destinations[0].LocalFileSystem)
	if err != nil {
		return nil, fmt.Errorf("init local dir destination: %w", err)
	}
	return dest, nil
}

/*func (backupSet *BackupSet) SetPassword(password string) {
	backupSet.conf.PasswordSalt = make([]byte, 64)
	if _, err := rand.Read(backupSet.conf.PasswordSalt); err != nil {
		panic(err)
	}
	backupSet.conf.PasswordHash = backupSet.computeHash(password)
}

func (backupSet *BackupSet) CheckPassword(password string) bool {
	return backupSet.computeHash(password) == backupSet.conf.PasswordHash
}

func (backupSet *BackupSet) computeHash(password string) [32]byte {
	hasher := sha256.New()
	hasher.Write(backupSet.conf.PasswordSalt)
	hasher.Write([]byte(password))
	return [32]byte(hasher.Sum(nil))
}*/

type BlobID [32]byte

func (id BlobID) String() string {
	return fmt.Sprintf("%x", [32]byte(id))
}

type blobLen uint32

func (backupSet *BackupSet) ReadBlobIndex(dest destination.Interface) (map[BlobID]blobLen, error) {
	data, err := dest.ReadFile(".blob-index")
	if err != nil {
		if dest.IsNotExists(err) {
			return make(map[BlobID]blobLen), nil
		}
		return nil, err
	}

	r := bytes.NewReader(data)
	version, err := r.ReadByte()
	if version != 0 {
		return nil, fmt.Errorf("unsupported blob index version %d", version)
	}

	var blobCount uint32
	if err := binary.Read(r, binary.LittleEndian, &blobCount); err != nil {
		return nil, err
	}

	blobs := make(map[BlobID]blobLen, blobCount)
	for range blobCount {
		var blobID BlobID
		if err := binary.Read(r, binary.LittleEndian, &blobID); err != nil {
			return nil, err
		}
		var blobLen blobLen
		if err := binary.Read(r, binary.LittleEndian, &blobLen); err != nil {
			return nil, err
		}
		blobs[blobID] = blobLen
	}

	return blobs, nil
}

func (backupSet *BackupSet) WriteBlobIndex(dest destination.Interface, blobs map[BlobID]blobLen) error {
	w := bytes.NewBuffer(nil)

	// version
	if err := w.WriteByte(0); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, uint32(len(blobs))); err != nil {
		return err
	}
	for id, blobLen := range blobs {
		if _, err := w.Write(id[:]); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, blobLen); err != nil {
			return err
		}
	}

	return dest.WriteFile(".blob-index", w.Bytes())
}

func (backupSet *BackupSet) ListSnapshots() ([]*Snapshot, error) {
	dest, err := backupSet.OpenDestination()
	if err != nil {
		return nil, err
	}
	return ListSnapshots(&snapshotContext{dest: dest})
}

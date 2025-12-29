package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sbreitf1/keepr/internal/backup"

	"github.com/adrg/xdg"
)

func getConfigDir() string {
	return filepath.Join(xdg.ConfigHome, "keepr")
}

func LoadBackupSets() ([]*backup.BackupSet, error) {
	configDir := getConfigDir()

	//TODO print error
	os.MkdirAll(configDir, os.ModePerm)

	data, err := os.ReadFile(filepath.Join(configDir, "backupsets.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return []*backup.BackupSet{}, nil
		}
		return nil, err
	}

	var conf struct {
		BackupSets []backup.BackupSetConfig
	}
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, err
	}

	sets := make([]*backup.BackupSet, 0)
	for _, bc := range conf.BackupSets {
		set, err := backup.NewBackupSetFromConfig(bc)
		if err != nil {
			return nil, err
		}
		sets = append(sets, set)
	}
	return sets, nil
}

func WriteBackupSets(sets []*backup.BackupSet) error {
	configDir := getConfigDir()

	if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(sets, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(configDir, "backupsets.json"), data, os.ModePerm)
}

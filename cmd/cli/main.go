package main

import (
	"fmt"
	"os"

	"github.com/sbreitf1/keepr/internal/backup"
	"github.com/sbreitf1/keepr/internal/config"
)

func main() {
	backupSets, err := config.LoadBackupSets()
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	snapshotter, err := backup.NewSnapshotter(backupSets[0])
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	fmt.Println(snapshotter.TakeSnapshot())
}

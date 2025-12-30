package main

import (
	"fmt"
	"os"

	"github.com/sbreitf1/keepr/internal/backup"
	"github.com/sbreitf1/keepr/internal/config"
	"github.com/sbreitf1/keepr/internal/serve"
)

func main() {
	backupSets, err := config.LoadBackupSets()
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	snapshots, err := backupSets[0].ListSnapshots()
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	fmt.Println("serve", snapshots[len(snapshots)-1].CreatedAt)
	browser, err := backup.NewBrowser(backupSets[0], snapshots[len(snapshots)-1])
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}

	/*r, err := browser.OpenFile("cnc2.png")
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	buf := make([]byte, 10*1024*1024)
	n, err := r.Read(buf)
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	fmt.Println(n)
	fmt.Println(os.WriteFile("G:\\Backup\\keepr-local-test\\cnc2.png", buf[:n], os.ModePerm))*/

	//fmt.Println(browser.GetFile("cnc2.png"))

	fmt.Println(serve.ServeWebDAV(browser))
	//fmt.Println(mirror.ServeWebDAV("G:\\Eigene Dateien\\Eigene Bilder\\Spiele"))

	/*snapshotter, err := backup.NewSnapshotter(backupSets[0])
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	fmt.Println(snapshotter.TakeSnapshot())*/
}

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hunterinvariants/HYPERION-DST/backup"
)

func main() {
	mode := flag.String("mode", "create", "create or restore")
	data := flag.String("data-dir", "", "offline node data directory")
	image := flag.String("backup-dir", "", "new backup directory")
	flag.Parse()
	var err error
	switch *mode {
	case "create":
		err = backup.Create(*data, *image)
	case "restore":
		err = backup.Restore(*image, *data)
	default:
		err = fmt.Errorf("invalid mode %q", *mode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hyperion-backup:", err)
		os.Exit(1)
	}
}

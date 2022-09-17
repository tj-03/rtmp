package logger

import (
	"log"
	"os"
	"path"

	"github.com/tj03/rtmp/src/util"
)

var (
	WarningLog *log.Logger
	ErrorLog   *log.Logger
	InfoLog    *log.Logger
)

func init() {
	config, _ := util.ReadConfig()
	dir := config.LogDirectory

	file, err := os.OpenFile(path.Join(dir, "log.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	WarningLog = log.New(file, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLog = log.New(file, "ERR: ", log.Ldate|log.Ltime|log.Lshortfile)
	InfoLog = log.New(file, "LOG: ", log.Ldate|log.Ltime|log.Lshortfile)
}

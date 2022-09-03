package logger

import (
	"log"
	"os"
)

var (
	WarningLog *log.Logger
	ErrorLog   *log.Logger
	InfoLog    *log.Logger
)

func init() {
	file, err := os.OpenFile("logs/log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	WarningLog = log.New(file, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLog = log.New(file, "ERR: ", log.Ldate|log.Ltime|log.Lshortfile)
	InfoLog = log.New(file, "LOG: ", log.Ldate|log.Ltime|log.Lshortfile)
}

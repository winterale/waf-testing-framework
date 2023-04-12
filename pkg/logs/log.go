package logs

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

//NewLogger creates a new log instance
func NewLogger(logfile string) *logrus.Logger {
	log := logrus.New()
	dirName := filepath.Dir(logfile)
	//check the directory exists
	if _, serr := os.Stat(dirName); serr != nil {
		merr := os.MkdirAll(dirName, os.ModePerm)
		if merr != nil {
			log.Fatalf("Error creating log file: %v", merr)
		}
	}
	f, err := os.Create(logfile)
	if err != nil {
		log.Fatalf("Error opening log file: %v", err)
	}
	log.SetFormatter(&Formatter{
		TimestampFormat: "01/02/2006 15:04:05",
	})
	log.SetOutput(f)
	return log
}

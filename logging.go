package main

import (
	"os"

	"github.com/sirupsen/logrus"
)

func setupLogging(logfile string) error {
	if logfile != "" {
		logrus.SetLevel(logrus.DebugLevel)
		f, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		logrus.SetOutput(f)
	} else {
		logrus.SetLevel(logrus.FatalLevel)
	}
	return nil
}

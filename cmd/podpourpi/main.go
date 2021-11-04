package main

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

func main() {
	logger := logrus.NewEntry(logrus.StandardLogger())
	err := Execute(logger)
	if err != nil {
		logger.Fatal(fmt.Errorf("podpourpi: %w", err))
	}
}

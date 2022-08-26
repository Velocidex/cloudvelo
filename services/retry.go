package services

import (
	"regexp"
	"time"
)

// Retry calls to the backend

var (
	retriableErrors = regexp.MustCompile("version conflict")
)

func retry(cb func() error) (err error) {
	for i := 0; i < 10; i++ {
		err = cb()
		if err == nil {
			return err
		}

		if !retriableErrors.MatchString(err.Error()) {
			return err
		}

		time.Sleep(time.Second)
	}

	return err
}

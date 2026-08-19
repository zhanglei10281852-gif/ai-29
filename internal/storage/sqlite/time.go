package sqlite

import "time"

func durationSeconds(seconds int64) time.Duration {
	return time.Duration(seconds) * time.Second
}

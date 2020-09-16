package ftpd

import (
	"fmt"
	"time"
)

func ftpTime(t time.Time) string {
	utc := t.UTC()
	return fmt.Sprintf("%04d%02d%02d%02d%02d%02f", utc.Year(), utc.Month(), utc.Day(), utc.Hour(), utc.Minute(), ((float64)(utc.Second())*1000000000+(float64)(utc.Nanosecond()))/1000000000)
}

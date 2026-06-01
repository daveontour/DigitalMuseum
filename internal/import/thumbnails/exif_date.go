package thumbnails

import (
	"fmt"
	"strings"
	"time"
)

// ParseExifDateTaken parses ImageMagick EXIF DateTimeOriginal-style values
// (e.g. "2024:01:15 12:30:45") into calendar parts and a timestamp.
func ParseExifDateTaken(dateTaken string) (year, month, day *int, takenAt *time.Time) {
	s := strings.TrimSpace(dateTaken)
	if s == "" {
		return nil, nil, nil, nil
	}

	layouts := []string{
		"2006:01:02 15:04:05",
		"2006:01:02 15:04",
		"2006:01:02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			y, m, d := t.Date()
			yy, mm, dd := y, int(m), d
			return &yy, &mm, &dd, &t
		}
	}

	parts := strings.Fields(s)
	if len(parts) == 0 {
		return nil, nil, nil, nil
	}
	dateParts := strings.Split(parts[0], ":")
	if len(dateParts) < 2 {
		return nil, nil, nil, nil
	}

	var yVal, mVal, dVal int
	if _, err := fmt.Sscanf(dateParts[0], "%d", &yVal); err != nil {
		return nil, nil, nil, nil
	}
	if _, err := fmt.Sscanf(dateParts[1], "%d", &mVal); err != nil {
		return nil, nil, nil, nil
	}
	year = &yVal
	month = &mVal
	if len(dateParts) >= 3 {
		if _, err := fmt.Sscanf(dateParts[2], "%d", &dVal); err == nil {
			day = &dVal
		}
	}
	return year, month, day, nil
}

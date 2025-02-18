package utils

import (
	"strings"
	"time"
)

func FormatTime(timestamp int64) string {
	t := time.Unix(timestamp/1000, 0).Add(2 * time.Hour)
	now := time.Now()

	if now.Sub(t) < 24*time.Hour {
		return t.Format("Today at 3:04 PM")
	} else if now.Sub(t) < 48*time.Hour {
		return t.Format("Yesterday at 3:04 PM")
	}

	return t.Format("Jan 2 at 3:04 PM")
}

func LowercaseTime(timestamp int64) string {
	formattedTime := FormatTime(timestamp)
	formattedTime = strings.Replace(formattedTime, "Today", "today", 1)
	formattedTime = strings.Replace(formattedTime, "Yesterday", "yesterday", 1)
	return formattedTime
}

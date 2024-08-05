package util

import "fmt"

func FormatBytesToHumanReadable(bytes uint64) string {
	const (
		_  = iota
		KB = 1 << (10 * iota)
		MB
		GB
		TB
	)
	unit := "Bytes"
	value := float64(bytes)

	switch {
	case bytes >= TB:
		unit = "TB"
		value /= TB
	case bytes >= GB:
		unit = "GB"
		value /= GB
	case bytes >= MB:
		unit = "MB"
		value /= MB
	case bytes >= KB:
		unit = "KB"
		value /= KB
	}

	return fmt.Sprintf("%.2f %s", value, unit)
}

package main

import "strconv"

func formatStatus(status int) string {
	if status == 0 {
		status = 200
	}
	return strconv.Itoa(status)
}

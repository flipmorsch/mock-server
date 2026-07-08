package main

import "strconv"

func formatStatus(status int) string {
	if status == 0 {
		status = 200
	}
	return strconv.Itoa(status)
}

func testInit(path string) string {
	return `{ method: 'GET', path: '` + path + `', headers: '', body: '' }`
}

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

func dragStart(idx int) string { return "onDragStart(" + strconv.Itoa(idx) + ", $event)" }
func dragOver(idx int) string { return "onDragOver($event, " + strconv.Itoa(idx) + ")" }

package ui

import (
	"encoding/json"
	"strconv"
)

func FormatStatus(status int) string {
	if status == 0 {
		status = 200
	}
	return strconv.Itoa(status)
}

func TestInit(path string) string {
	b, _ := json.Marshal(map[string]string{"method": "GET", "path": path, "headers": "", "body": ""})
	return string(b)
}

func DragStart(idx int) string { return "onDragStart(" + strconv.Itoa(idx) + ", $event)" }
func DragOver(idx int) string  { return "onDragOver($event, " + strconv.Itoa(idx) + ")" }

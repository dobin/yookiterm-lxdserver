package main

import (
	"net/http"
)

func auditLog(userId string, r *http.Request, message string) {
	address, _, _ := restClientIP(r)

	dbInsertLog(userId, address, message)
}

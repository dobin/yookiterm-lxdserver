package main

import (
	"encoding/json"
	//"fmt"
	"net/http"
	"os/exec"

	"github.com/gorilla/mux"

)


func execCommand(bashCommand string ) (error, string) {
	out, err := exec.Command(bashCommand).Output()
	return err, string(out)
}

// REST
// Authenticated
// Admin only
var restAdminExecHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	command := vars["command"]
	userId := getUserId(r)

	logger.Infof("Exec: ", command)

	if ! userIsAdmin(r) {
		logger.Infof("User %s which is not admin tried to exec %s", userId, command)

		http.Error(w, "Internal server error", 500)
		return
	}

	var output string
	var err error
	switch command {
	case "checkout": err, output = execCommand("./adminscripts/updatecontainer.sh");
	case "allowfw": err, output = execCommand("./adminscripts/fw-allow.sh");
	case "blockfw": err, output = execCommand("./adminscripts/fw-block.sh");
	}

	body := make(map[string]interface{})
	body["output"] = output
	body["error"] = err
	body["host"] = config.ServerHostnameAlias

	err = json.NewEncoder(w).Encode(body)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
})



// REST
// Authenticated, Admin only
// URL: /1.0/admin/logs
var restAdminLogsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	userId := getUserId(r)

	if ! userIsAdmin(r) {
		logger.Infof("User %s which is not admin tried to access logs", userId)
		http.Error(w, "Internal server error", 500)
		return
	}

	body := make(map[string]interface{})

	body["logs"], _ = dbGetLogs()
	err := json.NewEncoder(w).Encode(body)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
})


// REST
// Authenticated, Admin only
// URL: /1.0/admin/stats
var restAdminStatsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	userId := getUserId(r)

	if ! userIsAdmin(r) {
		logger.Infof("User %s which is not admin tried to access logs", userId)
		http.Error(w, "Internal server error", 500)
		return
	}

	body := make(map[string]interface{})

	logs, err := dbGetLogs()

	m := make(map[string]int)
	for _, log := range logs {
		m[log.UserId] += 1
	}


	type userLog struct {
		Username string
		Logincount int
	}
	var userLogs []userLog

	for username, logincount := range m {
		userLogs = append(userLogs, userLog{Username: username, Logincount: logincount})
	}

	body["stats"] = userLogs
	body["server"] = config.ServerHostnameAlias + config.ServerHttpPort

	err = json.NewEncoder(w).Encode(body)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
})

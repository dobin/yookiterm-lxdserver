package main

import (
	"encoding/json"
	"fmt"
//	"io"
//	"net"
	"net/http"
	"strconv"

//	"github.com/gorilla/mux"
//	"github.com/gorilla/websocket"
//	"github.com/lxc/lxd"
//	"github.com/lxc/lxd/shared"
)


// Console by contianer id
func restConsoleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get the id argument
	id := r.FormValue("id")

	// Get the container
	containerName, _, _, _, _, err := dbGetContainer(id)
	if err != nil || containerName == "" {
		http.Error(w, "Container not found", 404)
		return
	}

	// Get console width and height
	width := r.FormValue("width")
	height := r.FormValue("height")

	if width == "" {
		width = "150"
	}

	if height == "" {
		height = "20"
	}

	widthInt, err := strconv.Atoi(width)
	if err != nil {
		http.Error(w, "Invalid width value", 400)
	}

	heightInt, err := strconv.Atoi(height)
	if err != nil {
		http.Error(w, "Invalid width value", 400)
	}

	restMakeMeConsole(w, r, widthInt, heightInt, containerName)
}



func restStartHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	// Extract IP
	requestIP, _, err := restClientIP(r)
	if err != nil {
		restStartError(w, err, containerUnknownError)
		return
	}

	userId := r.FormValue("userId")
	//if userId == nil {
	//	fmt.Println("No Username")
	//}

	containerBaseName := r.FormValue("containerBaseName")
	//if containerBaseName == nil {
	//	fmt.Println("No containerBaseName")
	//}


	// If we already created the vm, return it already here
	doesExist, existuuid, containerName := dbContainerExists(userId, containerBaseName)
	if doesExist {
		fmt.Println("Exists: ", existuuid, "  name: ", containerName)
		restReturnExistingContainer(existuuid, userId, containerBaseName, w)
		return
	} else {
		fmt.Println("Does not exist")
		restCreateContainer(userId, containerBaseName, w, requestIP)
	}

/*
	// Check for banned users
	if shared.StringInSlice(requestIP, config.ServerBannedIPs) {
		fmt.Println("Banned user");
		//restStartError(w, nil, containerUserBanned)
		//return
	}

	// Count running containers
	containersCount, err := dbActiveCount()
	if err != nil {
		containersCount = config.ServerContainersMax
	}

	// Server is full
	if containersCount >= config.ServerContainersMax {
		fmt.Println("Server full");
		//restStartError(w, nil, containerServerFull)
		//return
	}

	// Count container for requestor IP
	containersCount, err = dbActiveCountForIP(requestIP)
	if err != nil {
		containersCount = config.QuotaSessions
	}

	if config.QuotaSessions != 0 && containersCount >= config.QuotaSessions {
		fmt.Println("Too many container for ip");
		//restStartError(w, nil, containerQuotaReached)
		//return
	}
*/
}



func restInfoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	// Get the id
	id := r.FormValue("id")

	// Get the container
	containerName, containerIP, containerUsername, containerPassword, containerExpiry, err := dbGetContainer(id)
	if err != nil || containerName == "" {
		http.Error(w, "Container not found", 404)
		return
	}

	body := make(map[string]interface{})

	if !config.ServerConsoleOnly {
		body["ip"] = containerIP
		body["username"] = containerUsername
		body["password"] = containerPassword
		body["fqdn"] = fmt.Sprintf("%s.lxd", containerName)
	}
	body["id"] = id
	body["expiry"] = containerExpiry

	// Return to the client
	body["status"] = containerStarted
	err = json.NewEncoder(w).Encode(body)
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		http.Error(w, "Internal server error", 500)
		return
	}
}

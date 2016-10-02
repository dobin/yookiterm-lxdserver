package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)
import b64 "encoding/base64"


// REST
// Authenticated
var restContainerListHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	userId := getUserId(r)

	err, containerList := dbGetContainerListForUser(userId)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}

	// add my hostname to every container


	err = json.NewEncoder(w).Encode(containerList)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
})


// REST
// Public
func restStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	var failure bool

	// Parse the remote client information
	address, protocol, err := restClientIP(r)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}

	// Get some container data
	var containersCount int
	var containersNext int

	containersCount, err = dbActiveContainerCount()
	if err != nil {
		failure = true
	}

	if containersCount >= config.ServerContainersMax {
		containersNext, err = dbNextExpire()
		if err != nil {
			failure = true
		}
	}

	// Generate the response
	body := make(map[string]interface{})
	body["client_address"] = address
	body["client_protocol"] = protocol
	body["server_console_only"] = config.ServerConsoleOnly
	body["server_ipv6_only"] = config.ServerIPv6Only
	if !config.ServerMaintenance && !failure {
		body["server_status"] = serverOperational
	} else {
		body["server_status"] = serverMaintenance
	}
	body["containers_count"] = containersCount
	body["containers_max"] = config.ServerContainersMax
	body["containers_next"] = containersNext

	err = json.NewEncoder(w).Encode(body)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
}


// REST
// Authenticated
var restContainerHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	containerBaseName := vars["containerBaseName"]

	userId := getUserId(r)

	logger.Infof("restContainerHandler: start container %s from user %s", containerBaseName, userId)

	body := make(map[string]interface{})

	var containerName string
	var containerIP string
	var containerUsername string
	var containerPassword string
	var containerExpiry int64

	containerName, containerIP, containerUsername, containerPassword, containerExpiry, err := dbGetContainerForUser(userId, containerBaseName)
	if err != nil || containerName == "" {
		body["isStarted"] = false
	} else {
		body["isStarted"] = true
	}

	body["containerName"] = containerName
	body["containerBaseName"] = containerBaseName

	body["ip"] = containerIP
	body["username"] = containerUsername
	body["password"] = containerPassword
	body["fqdn"] = fmt.Sprintf("%s.lxd", containerName)
//	body["id"] = id
	body["expiry"] = containerExpiry
	body["status"] = containerStarted

	err = json.NewEncoder(w).Encode(body)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
})


// REST
// Authenticated
var restContainerStartHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	containerBaseName := vars["containerBaseName"]

	userId := getUserId(r)

	logger.Infof("Starting containerBase %s for user %s", containerBaseName, userId)

	doesExist, _, _ := dbContainerExists(userId, containerBaseName)
	if doesExist {
		logger.Infof("Container already exists, returning data")
		body := make(map[string]interface{})
		var containerName string
		var containerIP string
		var containerUsername string
		var containerPassword string
		var containerExpiry int64

		containerName, containerIP, containerUsername, containerPassword, containerExpiry, err := dbGetContainerForUser(userId, containerBaseName)

		// Not necessary
		if err != nil || containerName == "" {
			http.Error(w, "Container not found", 404)
			return
		}

		body["isStarted"] = true
		body["ip"] = containerIP
		body["username"] = containerUsername
		body["password"] = containerPassword
		body["fqdn"] = fmt.Sprintf("%s.lxd", containerName)
	//	body["id"] = id
		body["expiry"] = containerExpiry
		body["status"] = containerStarted

		err = json.NewEncoder(w).Encode(body)
		if err != nil {
			http.Error(w, "Internal server error", 500)
			return
		}
	} else {
		restCreateContainer(userId, containerBaseName, w, "1.1.1.1")
	}
})


// REST
// Authenticated
// Admin only
var restContainerStopHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	containerBaseName := vars["containerBaseName"]
	userId := getUserId(r)

	if ! userIsAdmin(r) {
		logger.Infof("User %s which is not admin tried to stop a container %s", userId, containerBaseName)

		http.Error(w, "Internal server error", 500)
		return
	}

	body := make(map[string]interface{})

	logger.Infof("Stopping containerBase %s for user %s", containerBaseName, userId)
	var err error

	doesExist, uuid, containerName := dbContainerExists(userId, containerBaseName)
	if doesExist {
		lxdForceDelete(lxdDaemon, containerName)
		err = dbExpireUuid(uuid)
	} else {
		http.Error(w, "Container not found", 404)
		return
	}

	body["operationSuccess"] = true

	err = json.NewEncoder(w).Encode(body)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
})


// REST
// Authenticated
var restContainerConsoleHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerBaseName := vars["containerBaseName"]

	token := r.FormValue("token");
	isAuth, userId := jwtValidate(token)
	if isAuth == false {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	logger.Infof("restContainerConsoleHandler: start console for container %s for user %s", containerBaseName, userId)

	// TODO replace with db call
	containerName := fmt.Sprintf("%s%s", containerBaseName, userId)

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

	// TODO check if VM is already started

	restMakeMeConsole(w, r, widthInt, heightInt, containerName)
})


/********************/

func restClientIP(r *http.Request) (string, string, error) {
	var address string
	var protocol string

	viaProxy := r.Header.Get("X-Forwarded-For")

	if viaProxy != "" {
		address = viaProxy
	} else {
		host, _, err := net.SplitHostPort(r.RemoteAddr)

		if err == nil {
			address = host
		} else {
			address = r.RemoteAddr
		}
	}

	ip := net.ParseIP(address)
	if ip == nil {
		return "", "", fmt.Errorf("Invalid address: %s", address)
	}

	if ip.To4() == nil {
		protocol = "IPv6"
	} else {
		protocol = "IPv4"
	}

	return address, protocol, nil
}


func restMakeMeConsole(w http.ResponseWriter, r *http.Request, widthInt int, heightInt int, containerName string) {
	// Setup websocket with the client
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
	defer conn.Close()

	// Connect to the container
	env := make(map[string]string)
	env["USER"] = "root"
	env["HOME"] = "/root"
	env["TERM"] = "xterm"

	inRead, inWrite := io.Pipe()
	outRead, outWrite := io.Pipe()

	// read handler
	go func(conn *websocket.Conn, r io.Reader) {
		in := shared.ReaderToChannel(r, -1)

		for {
			buf, ok := <-in
			if !ok {
				break
			}

			data_b64 := b64.StdEncoding.EncodeToString([]byte(buf))
			err = conn.WriteMessage(websocket.TextMessage, []byte(data_b64))
			if err != nil {
				break
			}
		}
	}(conn, outRead)

	// writer handler
	go func(conn *websocket.Conn, w io.Writer) {
		for {
			mt, payload, err := conn.ReadMessage()
			if err != nil {
				if err != io.EOF {
					break
				}
			}

			switch mt {
			case websocket.BinaryMessage:
				continue
			case websocket.TextMessage:
				data_decoded, _ := b64.StdEncoding.DecodeString(string(payload))
				w.Write(data_decoded);
				//w.Write(payload);
			default:
				break
			}
		}
	}(conn, inWrite)

	// control socket handler
	handler := func(c *lxd.Client, conn *websocket.Conn) {
		for {
			_, _, err = conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}

	_, err = lxdDaemon.Exec(containerName, []string{"bash"}, env, inRead, outWrite, outWrite, handler, widthInt, heightInt)

	inWrite.Close()
	outRead.Close()

	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
}

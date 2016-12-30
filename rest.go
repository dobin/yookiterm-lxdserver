package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)
import b64 "encoding/base64"


// REST
// Public / Not-Authenticated
// URL: /1.0
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
// URL: /1.0/container
var restContainerListHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	userId := getUserId(r)

	err, containerList := dbGetContainerListForUser(userId)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}

	err = json.NewEncoder(w).Encode(containerList)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
})


// REST
// Authenticated
// URL: /1.0/container/{containerBaseName}
var restContainerHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	containerBaseName := vars["containerBaseName"]
	userId := getUserId(r)

	container, doesExist := dbGetContainerForUser(userId, containerBaseName)

	if doesExist == false {
		body := make(map[string]interface{})
		body["isStarted"] = false
		err := json.NewEncoder(w).Encode(body)
		if err != nil {
			http.Error(w, "Internal server error", 500)
			return
		}
	} else {
		restWriteContainerInfo(w, container);
		return
	}
})


// REST
// Authenticated
// URL: /1.0/container/{containerBaseName}/start
var restContainerStartHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	containerBaseName := vars["containerBaseName"]
	userId := getUserId(r)

	container, doesExist := dbGetContainerForUser(userId, containerBaseName)
	if doesExist {
		logger.Infof("Container %s for user %s already exists, get data", containerBaseName, userId)
		restWriteContainerInfo(w, container);
		return;
	} else {
		logger.Infof("Starting containerBase %s for user %s", containerBaseName, userId)
		restCreateContainer(userId, containerBaseName, w, "1.1.1.1")
		return;
	}
})


// REST
// Authenticated, Admin only
// URL: /1.0/container/{containerBaseName}/stop
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
// URL: /1.0/container/{containerBaseName}/console
var restContainerConsoleHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerBaseName := vars["containerBaseName"]

	// manual validation of auth token because its websocket
	token := r.FormValue("token");
	isAuth, userId := jwtValidate(token)
	if isAuth == false {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	logger.Infof("try creating a websocket console for container %s for user %s", containerBaseName, userId)

	doesExist, uuid, containerName := dbContainerExists(userId, containerBaseName)
	if doesExist == false {
		logger.Errorf("try creating a websocket console for container %s for user %s: container does not exist", containerBaseName, userId)
		http.Error(w, "Container not found", 404)
		return
	}

	containerExpiry := time.Now().Unix() + int64(config.QuotaTime)
	err := dbUpdateContainerExpire(uuid, containerExpiry)

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
		return
	}
	heightInt, err := strconv.Atoi(height)
	if err != nil {
		http.Error(w, "Invalid width value", 400)
		return
	}

	restMakeMeConsole(w, r, widthInt, heightInt, containerName)
})


/********************/

func restWriteContainerInfo(w http.ResponseWriter, container containerDbInfo) {
	body := make(map[string]interface{})

	body["containerName"] = container.ContainerName
	body["containerBaseName"] = container.ContainerBaseName

	body["isStarted"] = true
	body["username"] = container.ContainerUsername
	body["password"] = container.ContainerPassword
	body["expiry"] = container.ContainerExpiry
	body["status"] = container.ContainerStatus
	if container.ContainerIP != "" {
		body["sshPort"] = strings.Split(container.ContainerIP, ".")[3]
	} else {

	}

	err := json.NewEncoder(w).Encode(body)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
}


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

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dustinkirkland/golang-petname"
	"github.com/pborman/uuid"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)


func restCreateContainer(userId string, containerBaseName string, w http.ResponseWriter, requestIP string) {
	body := make(map[string]interface{})

	logger.Infof("Starting container %s for %s", containerBaseName, userId)

	requestDate := time.Now().Unix()

	// Create the container
	containerName := fmt.Sprintf("%s%s", containerBaseName, userId)
	containerUsername := petname.Adjective()
	containerPassword := petname.Adjective()
	id := uuid.NewRandom().String()

	// Config
	ctConfig := map[string]string{}

	ctConfig["security.nesting"] = "false"
	if config.QuotaCPU > 0 {
		ctConfig["limits.cpu"] = fmt.Sprintf("%d", config.QuotaCPU)
	}

	if config.QuotaRAM > 0 {
		ctConfig["limits.memory"] = fmt.Sprintf("%dMB", config.QuotaRAM)
	}

	if config.QuotaProcesses > 0 {
		ctConfig["limits.processes"] = fmt.Sprintf("%d", config.QuotaProcesses)
	}

	if !config.ServerConsoleOnly {
		ctConfig["user.user-data"] = fmt.Sprintf(`#cloud-config
ssh_pwauth: True
manage_etc_hosts: True
users:
 - name: %s
	 groups: sudo
	 plain_text_passwd: %s
	 lock_passwd: False
	 shell: /bin/bash
`, containerUsername, containerPassword)
	}

	var resp *lxd.Response

	// Copy the base image
	logger.Debugf("restCreateContainer: Pre-copy")
	logger.Infof("Creating container from image %s with name %s", containerBaseName, containerName)
		resp, err := lxdDaemon.LocalCopy(containerBaseName, containerName, ctConfig, nil, false)

	if err != nil {
		restStartContainerError(w, err, containerUnknownError)
		return
	}
	err = lxdDaemon.WaitForSuccess(resp.Operation)
	if err != nil {
		restStartContainerError(w, err, containerUnknownError)
		return
	}
	logger.Debugf("restCreateContainer: Post-copy")

	// Configure the container devices
	ct, err := lxdDaemon.ContainerInfo(containerName)
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		restStartContainerError(w, err, containerUnknownError)
		return
	}
	if config.QuotaDisk > 0 {
		ct.Devices["root"] = shared.Device{"type": "disk", "path": "/", "size": fmt.Sprintf("%dGB", config.QuotaDisk)}
	}
	err = lxdDaemon.UpdateContainerConfig(containerName, ct.Brief())
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		restStartContainerError(w, err, containerUnknownError)
		return
	}

	// Start the container
	logger.Debugf("restCreateContainer: Pre-start")
	resp, err = lxdDaemon.Action(containerName, "start", -1, false, false)
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		restStartContainerError(w, err, containerUnknownError)
		return
	}
	err = lxdDaemon.WaitForSuccess(resp.Operation)
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		restStartContainerError(w, err, containerUnknownError)
		return
	}
	logger.Debugf("restCreateContainer: Post-start")

	_, containerIP := containerGetIp(containerName)

	containerExpiry := time.Now().Unix() + int64(config.QuotaTime)

	// Prepare return data
	if !config.ServerConsoleOnly {
		body["ip"] = containerIP
		body["username"] = containerUsername
		body["password"] = containerPassword
		body["fqdn"] = fmt.Sprintf("%s.lxd", containerName)
	}
	body["id"] = id
	body["expiry"] = containerExpiry

	// Setup cleanup code
	duration, err := time.ParseDuration(fmt.Sprintf("%ds", config.QuotaTime))
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		restStartContainerError(w, err, containerUnknownError)
		return
	}

	// Create container in db
	containerID, err := dbNewContainer(id, userId, containerBaseName, containerName, containerIP, containerUsername, containerPassword, containerExpiry, requestDate, requestIP)
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		restStartContainerError(w, err, containerUnknownError)
		return
	}

	// Create timer to destroy that container after configured timeframe
	time.AfterFunc(duration, func() {
		lxdForceDelete(lxdDaemon, containerName)
		dbExpire(containerID)
	})
	// Create thread which gets the IP
	//go storeContainerIp(containerID, containerName)

	// Return data to the client
	body["status"] = containerStarted
	err = json.NewEncoder(w).Encode(body)
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		http.Error(w, "Internal server error", 500)
		return
	}
}


func storeContainerIp(containerID int64, containerName string) {
	logger.Infof("Trying to get ip... ")

	err, ip := containerGetIp(containerName)
	if err != nil {
		logger.Errorf("Could not get IP :(")
		return
	}

	logger.Infof("Found ip: %s", ip)
	//dbWriteContainerIp(containerID, ip)

}


func containerGetIp(containerName string) (error, string) {
	var containerIP string

	time.Sleep(1 * time.Second)

	timeout := 16
	for timeout != 0 {
		timeout--
		ct, err := lxdDaemon.ContainerState(containerName)
		if err != nil {
			lxdForceDelete(lxdDaemon, containerName)
			//restStartContainerError(w, err, containerUnknownError)
			return err, ""
		}

		for netName, net := range ct.Network {
			if !shared.StringInSlice(netName, []string{"eth0", "lxcbr0"}) {
				continue
			}

			for _, addr := range net.Addresses {
				if addr.Address == "" {
					continue
				}

				if addr.Scope != "global" {
					continue
				}

				//if config.ServerIPv6Only && addr.Family != "inet6" {
				//	continue
				//}

				containerIP = addr.Address
				break
			}

			if containerIP != "" {
				break
			}
		}

		if containerIP != "" {
			break
		}

		time.Sleep(500 * time.Millisecond)
	}

	return nil, containerIP
}


func initialContainerCleanupHandler() error {
	// Restore cleanup handler for existing containers
	containers, err := dbActiveContainer()
	if err != nil {
		return err
	}

	for _, entry := range containers {
		containerID := int64(entry[0].(int))
		containerName := entry[1].(string)
		containerExpiry := int64(entry[2].(int))

		duration := containerExpiry - time.Now().Unix()
		timeDuration, err := time.ParseDuration(fmt.Sprintf("%ds", duration))
		if err != nil || duration <= 0 {
			lxdForceDelete(lxdDaemon, containerName)
			dbExpire(containerID)
			continue
		}

		time.AfterFunc(timeDuration, func() {
			lxdForceDelete(lxdDaemon, containerName)
			dbExpire(containerID)
		})
	}

	return nil
}


func restStartContainerError(w http.ResponseWriter, err error, code statusCode) {
	body := make(map[string]interface{})
	body["status"] = code

	logger.Errorf("restStartContainerError: %s", err)

	if err != nil {
		fmt.Printf("error: %s\n", err)
	}

	err = json.NewEncoder(w).Encode(body)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
}

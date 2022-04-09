package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"
	"github.com/pborman/uuid"
)

func containerStartIfStopped(userId string, containerBaseName string) error {
	containerName := fmt.Sprintf("%s%s", containerBaseName, userId)

	logger.Infof("(try) Starting container %s for %s", containerBaseName, userId)

	req := api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	}

	op, err := lxdDaemon.UpdateContainerState(containerName, req, "")
	if err != nil {
		return err
	}

	err = op.Wait()
	if err != nil {
		return err
	}

	return nil
}

func restCreateContainer(userId string, containerBaseName string, w http.ResponseWriter, requestIP string) {
	body := make(map[string]interface{})
	requestDate := time.Now().Unix()

	// Container Data
	containerName := fmt.Sprintf("%s%s", containerBaseName, userId)
	containerUsername := petname.Adjective()
	containerPassword := petname.Adjective()
	id := uuid.NewRandom().String()

	// Config
	ctConfig := makeConfig(containerUsername, containerPassword)

	logger.Infof("Starting container %s for %s", containerBaseName, userId)

	err := containerCopy(ctConfig, containerBaseName, containerName)
	if err != nil {
		restStartContainerError(w, err, containerUnknownError)
		return
	}

	err = containerConfig(containerName)
	if err != nil {
		restStartContainerError(w, err, containerUnknownError)
		return
	}

	err = containerStart(containerName)
	if err != nil {
		restStartContainerError(w, err, containerUnknownError)
		return
	}

	err, containerIP := containerGetIp(containerName)
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		restStartContainerError(w, err, containerUnknownError)
		return
	}

	containerExpiry := time.Now().Unix() + int64(config.QuotaTime)
	containerExpiryHard := time.Now().Unix() + int64(config.QuotaTimeMax)

	// Prepare return data
	if !config.ServerConsoleOnly {
		body["ip"] = containerIP
		body["username"] = containerUsername
		body["password"] = containerPassword
		body["fqdn"] = fmt.Sprintf("%s.lxd", containerName)
	}
	body["id"] = id
	body["expiry"] = containerExpiry
	body["expiryHard"] = containerExpiryHard

	// Setup cleanup code
	duration, err := time.ParseDuration(fmt.Sprintf("%ds", config.QuotaTime))
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		restStartContainerError(w, err, containerUnknownError)
		return
	}

	// Create container in db
	containerID, err := dbNewContainer(
		id, userId, containerBaseName, containerName, containerIP, containerUsername,
		containerPassword, containerExpiry, containerExpiryHard, requestDate, requestIP)
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		restStartContainerError(w, err, containerUnknownError)
		return
	}

	// Create timer to destroy that container after configured timeframe
	time.AfterFunc(duration, func() {
		logger.Infof("Deleting container AfterFunc (%s)", duration)
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

func makeConfig(containerUsername, containerPassword string) map[string]string {
	ctConfig := map[string]string{}

	ctConfig["security.nesting"] = "false"
	if config.QuotaCPU > 0 {
		ctConfig["limits.cpu.allowance"] = fmt.Sprintf("%d%%", config.QuotaCPU)
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

	return ctConfig
}

func containerCopy(ctConfig map[string]string, containerBaseName, containerName string) error {
	var rop lxd.RemoteOperation
	logger.Debugf("ContainerCopy")

	logger.Infof("Creating container from image %s with name %s", containerBaseName, containerName)
	args := lxd.ContainerCopyArgs{
		Name:          containerName,
		ContainerOnly: true,
	}
	source, _, err := lxdDaemon.GetContainer(containerBaseName)
	if err != nil {
		return fmt.Errorf("GetContainer for %s error: %s", containerBaseName, err.Error())
	}
	source.Config = ctConfig
	source.Profiles = config.Profiles
	rop, err = lxdDaemon.CopyContainer(lxdDaemon, *source, &args)
	if err != nil {
		return fmt.Errorf("CopyContainer error: %s", err.Error())
	}
	err = rop.Wait()
	if err != nil {
		return fmt.Errorf("CopyContainer error: %s", err.Error())
	}

	return nil
}

func containerConfig(containerName string) error {
	logger.Debugf("ContainerConfig")

	ct, etag, err := lxdDaemon.GetContainer(containerName)
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		return err
	}

	if config.QuotaDisk > 0 {
		_, ok := ct.ExpandedDevices["root"]
		if ok {
			ct.Devices["root"] = ct.ExpandedDevices["root"]
			ct.Devices["root"]["size"] = fmt.Sprintf("%dGB", config.QuotaDisk)
		} else {
			ct.Devices["root"] = map[string]string{"type": "disk", "path": "/", "size": fmt.Sprintf("%dGB", config.QuotaDisk)}
		}
	}
	op, err := lxdDaemon.UpdateContainer(containerName, ct.Writable(), etag)
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		return err
	}
	err = op.Wait()
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		return err
	}

	return nil
}

func containerStart(containerName string) error {
	logger.Debugf("containerStart")

	req := api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	}
	op, err := lxdDaemon.UpdateContainerState(containerName, req, "")
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		return err
	}
	err = op.Wait()
	if err != nil {
		lxdForceDelete(lxdDaemon, containerName)
		return err
	}

	return nil
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
	time.Sleep(2 * time.Second)
	timeout := 30
	for timeout != 0 {
		timeout--
		ct, _, err := lxdDaemon.GetContainerState(containerName)
		if err != nil {
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
	body["container_status"] = code

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

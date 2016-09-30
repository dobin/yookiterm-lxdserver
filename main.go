package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/lxc/lxd"
	"golang.org/x/exp/inotify"
	"gopkg.in/yaml.v2"
	"github.com/rs/cors"
  "github.com/howbazaar/loggo"
)

// Global variables
var lxdDaemon *lxd.Client
var config serverConfig

var logger = loggo.GetLogger("project.main")


func initLogger() {
	logger.SetLogLevel(loggo.DEBUG)
}


type serverConfig struct {
	QuotaCPU            int      `yaml:"quota_cpu"`
	QuotaRAM            int      `yaml:"quota_ram"`
	QuotaDisk           int      `yaml:"quota_disk"`
	QuotaProcesses      int      `yaml:"quota_processes"`
	QuotaSessions       int      `yaml:"quota_sessions"`
	QuotaTime           int      `yaml:"quota_time"`
	Container           string   `yaml:"container"`
	Image               string   `yaml:"image"`
	ServerBannedIPs     []string `yaml:"server_banned_ips"`
	ServerConsoleOnly   bool     `yaml:"server_console_only"`
	ServerIPv6Only      bool     `yaml:"server_ipv6_only"`
	ServerCPUCount      int      `yaml:"server_cpu_count"`
	ServerContainersMax int      `yaml:"server_containers_max"`
	ServerMaintenance   bool     `yaml:"server_maintenance"`
	ServerTerms         string   `yaml:"server_terms"`
	Jwtsecret           string   `yaml:"jwtsecret"`
	ServerHttp          bool     `yaml:"server_http"`
	ServerHttpPort      string   `yaml:"server_http_port"`
	ServerHttps         bool     `yaml:"server_https"`
	ServerHttpsPort     string   `yaml:"server_https_port"`
	ServerHttpsCertFile string   `yaml:"server_https_cert_file"`
	ServerHttpsKeyFile  string   `yaml:"server_https_key_file"`
	ServerHostnameAlias string   `yaml:"server_hostname_alias"`
	ContainerSshBasePort int     `yaml:"container_sshbaseport"`
}


type statusCode int
const (
	serverOperational statusCode = 0
	serverMaintenance statusCode = 1

	containerStarted      statusCode = 0
//	containerInvalidTerms statusCode = 1
	containerServerFull   statusCode = 2
	containerQuotaReached statusCode = 3
	containerUserBanned   statusCode = 4
	containerUnknownError statusCode = 5
)


func main() {
	//rand.Seed(time.Now().UTC().UnixNano())
	err := run()
	if err != nil {
		fmt.Printf("error: %s\n", err)
		os.Exit(1)
	}
}


func parseConfig() error {
	data, err := ioutil.ReadFile("yookitermlxd-config.yml")
	if os.IsNotExist(err) {
		return fmt.Errorf("The configuration file (yookitermlxd-config.yml) doesn't exist.")
	} else if err != nil {
		return fmt.Errorf("Unable to read the configuration: %s", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return fmt.Errorf("Unable to parse the configuration: %s", err)
	}

	return nil
}


func configWatcher() {
	// Watch for configuration changes
	watcher, err := inotify.NewWatcher()
	if err != nil {
		fmt.Errorf("Unable to setup inotify: %s", err)
	}

	err = watcher.Watch(".")
	if err != nil {
		fmt.Errorf("Unable to setup inotify watch: %s", err)
	}

	go func() {
		for {
			select {
			case ev := <-watcher.Event:
				if ev.Name != "./lxd-demo.yml" {
					continue
				}

				if ev.Mask&inotify.IN_MODIFY != inotify.IN_MODIFY {
					continue
				}

				fmt.Printf("Reloading configuration\n")
				err := parseConfig()
				if err != nil {
					fmt.Printf("Failed to parse configuration: %s\n", err)
				}
			case err := <-watcher.Error:
				fmt.Printf("Inotify error: %s\n", err)
			}
		}
	}()
}


func run() error {
	var err error

	initLogger()

	// Setup configuration
	err = parseConfig()
	if err != nil {
		return err
	}

	// Start the configuration file watcher
	configWatcher()

	// Connect to the LXD daemon
	lxdDaemon, err = lxd.NewClient(&lxd.DefaultConfig, "local")
	if err != nil {
		return fmt.Errorf("Unable to connect to LXD: %s", err)
	}

	// Setup the database
	err = dbSetup()
	if err != nil {
		return fmt.Errorf("Failed to setup the database: %s", err)
	}

	// Restore cleanup handler for existing containers
	err = initialContainerCleanupHandler()
	if err != nil {
		return fmt.Errorf("Unable to read current containers: %s", err)
	}

	// Setup the HTTP server
	r := mux.NewRouter()

	// API
	// Public
	r.HandleFunc("/1.0", restStatusHandler)

	// Container related
	// Authenticated
	r.Handle("/1.0/container", jwtMiddleware.Handler(restContainerListHandler))
	r.Handle("/1.0/container/{containerBaseName}", jwtMiddleware.Handler(restContainerHandler))
	r.Handle("/1.0/container/{containerBaseName}/start", jwtMiddleware.Handler(restContainerStartHandler))
	r.Handle("/1.0/container/{containerBaseName}/stop", jwtMiddleware.Handler(restContainerStopHandler))
	// Websockets cannot contain additional header, so authentication is
	// performed via token in GET parameter
	r.Handle("/1.0/container/{containerBaseName}/console", restContainerConsoleHandler)

	// Admin related
	// Authenticated
	r.Handle("/1.0/admin/exec/{command}", jwtMiddleware.Handler(restAdminExecHandler))

	// Set CORS
	c := cors.New(cors.Options{
	    AllowedOrigins: []string{"*"},
	    AllowCredentials: true,
			AllowedHeaders: []string{"Origin", "X-Requested-With", "Content-Type", "Accept", "Authorization"},
	})
	handler := c.Handler(r)

	fmt.Println("Yookiterm LXD server 0.3")
	if config.ServerHttps {
		go func() {
			fmt.Println("Listening HTTPS on: ", config.ServerHttpsPort)
			//err = http.ListenAndServeTLS(":443", "/etc/letsencrypt/live/container.exploit.courses/fullchain.pem", "/etc/letsencrypt/live/container.exploit.courses/privkey.pem", handler)
			err = http.ListenAndServeTLS(config.ServerHttpsPort, config.ServerHttpsCertFile, config.ServerHttpsKeyFile, handler)
	   	 	if err != nil {
	       	 		fmt.Println("ListenAndServe: ", handler)
    		}
		}()
	}

	if config.ServerHttp {
		fmt.Println("Listening HTTP on: ", config.ServerHttpPort)
		err = http.ListenAndServe(config.ServerHttpPort, handler)
		if err != nil {
			//return err
			fmt.Println("HTTP error:", err)
		}
	}

	return nil
}

# yookiterm lxd server

The server used to deploy and access linux containers.


## What is yookiterm

Yookiterm provides per-user Linux root containers via JavasScript
terminal, and accompagning tutorials and writeups of
certain topics. It is currently used as a plattform
teaching exploit development at an university.

## Building it
```
 go get
 go build
```


## Config

```yml
jwtsecret: "<secret>"
quota_cpu: 1
quota_ram: 128
quota_disk: 5
quota_sessions: 2
quota_time: 21600
quota_time_max: 43200
quota_processes: 200

server_banned_ips:
server_console_only: false
server_containers_max: 50
server_cpu_count: 2
server_ipv6_only: true
server_maintenance: false

server_http: true
server_http_port: ":80"

server_https: true
server_https_port: ":443"
server_https_cert_file: "/etc/letsencrypt/live/container.exploit.courses/fullchain.pem"
server_https_key_file: "/etc/letsencrypt/live/container.exploit.courses/privkey.pem"
```

## Credits

Based on https://github.com/lxc/lxd-demo-server/

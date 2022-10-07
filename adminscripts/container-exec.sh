#!/bin/bash

command=$1
echo "Execute command: $1"

lxd.lxc start Debian32
lxd.lxc start Debian64

sleep 3

echo "Debian32: "
lxd.lxc exec Debian32 -- /bin/bash -c "$command"

echo "Dabian64: "
lxd.lxc exec Debian64 -- /bin/bash -c "$command"

sleep 1

lxd.lxc stop Debian32
lxd.lxc stop Debian64

echo "Execute command: done"


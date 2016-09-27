#!/bin/bash

./adminscripts/fw-allow.sh

echo "Update: hlUbuntu32 and hlUbuntu64"

lxc start hlUbuntu32
lxc start hlUbuntu64

sleep 3

echo "Ubuntu32: "
lxc exec hlUbuntu32 -- /bin/bash -c 'cd /root/challenges; git pull;'
echo "Ubuntu64: "
lxc exec hlUbuntu64 -- /bin/bash -c 'cd /root/challenges; git pull;'

sleep 1

lxc stop hlUbuntu32
lxc stop hlUbuntu64

echo "Update: done"

./adminscripts/fw-block.sh

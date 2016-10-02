#!/bin/bash

$command = ""

echo "Execute command: hlUbuntu32 and hlUbuntu64"

lxc start hlUbuntu32
lxc start hlUbuntu64

sleep 3

echo "Ubuntu32: "
lxc exec hlUbuntu32 -- /bin/bash -c "$command"
echo "Ubuntu64: "
lxc exec hlUbuntu64 -- /bin/bash -c "$command"

sleep 1

lxc stop hlUbuntu32
lxc stop hlUbuntu64

echo "Execute command: done"

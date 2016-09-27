#!/bin/bash

echo "iptables: Restore iptables-blockcontainer.rules"
iptables-restore ./adminscripts/iptables-blockcontainer.rules

#!/bin/bash

echo "iptables: Restore iptables-allowcontainer.rules"
iptables-restore ./adminscripts/iptables-allowcontainer.rules

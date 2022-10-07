#!/bin/bash

if [ "$1" == "enable" ]; then
        echo "Enable Container Firewall"
        sudo /sbin/iptables -A FORWARD -j DROP
fi

if [ "$1" = "disable" ]; then
        echo "Disable Firewall"
        sudo /sbin/iptables -D FORWARD -j DROP
fi


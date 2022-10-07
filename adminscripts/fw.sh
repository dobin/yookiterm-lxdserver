#!/bin/bash

if [ "$1" == "enable" ]; then
        echo "Enable Container Firewall"
        iptables -A FORWARD -j DROP
fi

if [ "$1" = "disable" ]; then
        echo "Disable Firewall"
        iptables -D FORWARD -j DROP
fi


#!/bin/bash

execInContainers() {
        arr=("$@")
        vms=("vmaslr" "vmnoaslr")

        for vm in "${vms[@]}"; do
                echo "Update: ${vm}"

                ssh yookiterm@${vm} /bin/bash << EOF
                        lxd.lxc start Debian32
                        lxd.lxc start Debian64
EOF
                sleep 1

                for i in "${arr[@]}"; do
                        ssh yookiterm@${vm} /bin/bash << EOF
                                echo "Update Debian32 with: $i"
                                lxd.lxc exec Debian32 -- /bin/bash -c "$i"
EOF

                        ssh yookiterm@${vm} /bin/bash << EOF
                                echo "Update Debian64 with: $i"
                                lxd.lxc exec Debian64 -- /bin/bash -c "$i"
EOF
                done

                ssh yookiterm@${vm} /bin/bash << EOF
                        lxd.lxc stop Debian32
                        lxd.lxc stop Debian64
EOF

        done
}


commands=("cd /root/challenges; git pull")
execInContainers "${commands[@]}"
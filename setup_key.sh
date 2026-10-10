#!/bin/bash
mkdir -p /root/.ssh
chmod 700 /root/.ssh
echo "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILccdflM4tMqNiJKC6+u+h4A2pDwpcHQE2D04czRh3qS mac-ulanish" >> /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys
echo "SSH_KEY_INSTALLED_OK"

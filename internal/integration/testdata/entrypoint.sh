#!/bin/sh
set -e

if [ -n "$PUBLIC_KEY" ]; then
    printf '%s\n' "$PUBLIC_KEY" > /home/testuser/.ssh/authorized_keys
    chmod 600 /home/testuser/.ssh/authorized_keys
    chown testuser:testuser /home/testuser/.ssh/authorized_keys
fi

exec /usr/sbin/sshd -D -e

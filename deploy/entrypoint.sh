#!/bin/sh
set -e

# Fix ownership of the mounted volume while still root, then drop privileges and
# exec the app so it runs as PID 1 and receives SIGTERM directly. (Running it
# under su left su as PID 1, which SIGKILLed the app on docker stop and skipped
# the graceful shutdown.)
chown -R servicarr:servicarr /data
exec setpriv --reuid=servicarr --regid=servicarr --init-groups --inh-caps=-all --no-new-privs /usr/local/bin/status

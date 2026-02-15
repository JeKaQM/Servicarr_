#!/bin/sh
# Fix ownership of mounted volume (runs as root briefly, then drops to servicarr)
chown -R servicarr:servicarr /data
exec su -s /bin/sh servicarr -c '/usr/local/bin/status'

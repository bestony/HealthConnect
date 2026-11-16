#!/bin/sh
# When the container starts as root, give the data directory to healthd and drop privileges.
set -eu

data_dir="${DATA_DIR:-/var/lib/healthconnect}"

if [ "$(id -u)" -eq 0 ]; then
	mkdir -p "$data_dir"
	chown healthd:healthd "$data_dir"
	exec setpriv --reuid=healthd --regid=healthd --clear-groups --inh-caps=-all /usr/local/bin/healthd
fi

exec /usr/local/bin/healthd

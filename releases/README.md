# Published feeder releases

Each `vVERSION` directory contains the four platform packages, a SHA-256
manifest and its ECDSA signature. The public installer has the matching public
key embedded and refuses unsigned or modified packages.

Raw standalone binaries are build outputs, but public installation uses the
signed `.tar.gz` packages because they also contain the systemd unit, migration
installer and rollback command.

# ADS-B.Pro feeder

The supported feeder is `adsbpro-feeder`, a static Go client using feeder
protocol v2. It pairs once over TLS on `feed.ads-b.pro:48582` and then forwards
only the raw Beast or SBS stream. Tokens and signatures are not appended to
aircraft frames.

The legacy `radarview.py` client and port `48581` remain available for rollback
and backward compatibility.

An ADS-B decoder such as readsb or dump1090 must already expose Beast on port
`30005` or SBS on port `30003`. The installer does not replace or reconfigure
the receiver's decoder.

## Install or migrate

Download the installer first and run it as root:

```bash
curl -fsSLo /tmp/adsbpro-setup.sh \
  https://raw.githubusercontent.com/br3jski/radarview/main/radarview_setup.sh
sudo bash /tmp/adsbpro-setup.sh
```

The installer:

1. detects Linux `amd64`, `arm64`, `armv6` or `armv7`;
2. downloads the matching release package;
3. verifies the signed manifest and SHA-256 checksum;
4. reads an existing legacy token or prompts for one without displaying it;
5. installs the hardened `adsbpro-feeder.service` under a dedicated user;
6. waits for the server to accept the first ADS-B frame;
7. disables `radarview.service` only after v2 reaches `ACTIVE`.

Existing legacy files are never deleted. If pairing or the data source fails,
legacy remains unchanged. A first v2 installation on an account can pair with
the current feeder token. Further installations require a 10-minute pairing
window opened in the ADS-B.Pro account panel.

### Non-interactive token input

Store the token in a root-readable temporary file and pass its path:

```bash
sudo bash /tmp/adsbpro-setup.sh --token-file /path/to/pairing-token
```

The token file copied into the feeder data directory is mode `0600` and is
deleted by the client only after `ACTIVE` and the first accepted frame. The
source file supplied with `--token-file` is not deleted by the installer.

### Source overrides

By default the client waits up to five seconds for a valid Beast frame from
`127.0.0.1:30005`, then falls back to SBS on `127.0.0.1:30003`.

```bash
sudo bash /tmp/adsbpro-setup.sh \
  --source-mode beast \
  --source-host 127.0.0.1 \
  --beast-port 30005
```

Supported options are listed by:

```bash
bash /tmp/adsbpro-setup.sh --help
```

## Operations

```bash
sudo systemctl status adsbpro-feeder.service
sudo /usr/local/bin/adsbpro-feeder status
sudo journalctl -u adsbpro-feeder.service
```

Rollback keeps the v2 identity for a later retry and re-enables legacy:

```bash
sudo /usr/local/sbin/adsbpro-feeder-rollback
```

There is no automatic updater. Installing a newer signed release is an explicit
administrator action.

## Build and test

```bash
go test ./...
VERSION=2.0.0 ./scripts/build-release.sh
```

For a signed release, set `RELEASE=true` and point `RELEASE_SIGNING_KEY` at the
ECDSA P-256 release signing key. The public verification key is committed in
`packaging/release-signing-public.pem` and embedded in the public bootstrap
installer. Release signatures use ECDSA with SHA-256 and are verified by
OpenSSL. Public packages are stored under `releases/vVERSION`; a GitHub Release
can be added as an optional mirror without changing the trust model.

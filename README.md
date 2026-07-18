# ADS-B.Pro feeder

`radarview.py` remains the supported legacy client for port 48581. The new
`adsbpro-feeder` Go client pairs once over TLS on port 48582 and then sends only
raw Beast or SBS data. It does not append an account token to aircraft frames.

Build and test v2 locally:

```bash
go test ./...
go build ./cmd/adsbpro-feeder
```

The migration installer does not stop `radarview.service` until the server has
acknowledged the first valid v2 frame. Legacy files remain available for rollback.
Run `sudo ./rollback-v2.sh` to stop v2 and re-enable the preserved legacy unit.

## Legacy client

# About RadarView and what the heck happened to RawFlight?!

RawFlight was a revived from 2020 project of community flight tracking system. Now, one developer, Bruno, revived this project and renamed it to RadarView
For now, RadarView is in closed beta stage and access to RawFlight is granted to feeders only. 
We have written all necessary data collection scripts on our own, so you can simply download our script and start being a RawFlight Feeder!

## Installation

Simply download installation script and let it do all work for you, 

```bash
curl https://github.com/br3jski/radarview/blob/main/radarview_setup.sh | bash
```
Or, if you prefer to use wget, execute: 
```bash
wget -O https://github.com/br3jski/radarview/blob/main/radarview_setup.sh install_rawflight.sh | bash install_rawflight.sh
```

## Roadmap?
Once I will make a Wiki, I will prepare everything. Remember, I am the only one person working on it :)

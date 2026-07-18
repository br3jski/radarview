# ADS-B.Pro Feeder

This installer connects your ADS-B receiver to ADS-B.Pro.

For Linux you need:

- a Linux receiver with readsb or dump1090 already working;
- Beast data on port `30005` or SBS data on port `30003`;
- your ADS-B.Pro feeder token.

## New installation

Run this command on your receiver:

```bash
curl -fsSL https://raw.githubusercontent.com/br3jski/radarview/main/radarview_setup.sh | sudo bash
```

When asked, paste your feeder token and press Enter. The token is not displayed
while you type.

The installation is complete only when you see:

```text
Feeder v2 is ACTIVE.
```

Check the feeder at any time with:

```bash
sudo /usr/local/bin/adsbpro-feeder status
```

The result should contain `"state":"active"`.

## Windows installation

Open **Windows PowerShell as Administrator** and run:

```powershell
irm https://raw.githubusercontent.com/br3jski/radarview/main/radarview_setup.ps1 | iex
```

Paste your feeder token when asked. The installer supports 64-bit Intel/AMD and
ARM64 Windows. It automatically uses Beast data from `127.0.0.1:30005`, or SBS
from `127.0.0.1:30003` when Beast data is unavailable.

The installation is complete only when you see `Feeder v2 is ACTIVE.`. The
feeder then runs automatically as the `ADSBProFeeder` Windows service.

Check its status with:

```powershell
& "$env:ProgramFiles\ADSBPro\Feeder\adsbpro-feeder.exe" status
```

## Migrating an old RadarView feeder

If your receiver already uses `/opt/radarview.py`, run exactly the same command:

```bash
curl -fsSL https://raw.githubusercontent.com/br3jski/radarview/main/radarview_setup.sh | sudo bash
```

The installer reads the existing token automatically. The old feeder keeps
working until the new feeder connects and ADS-B.Pro accepts its first frame.
Only then is the old `radarview.service` disabled.

The old script is not deleted. If migration fails, the old feeder remains
active and nothing needs to be restored.

## Migrating another receiver on the same account

Open a 10-minute feeder pairing window in your ADS-B.Pro account panel, then run
the installation command. If this is the account's first v2 receiver, no
pairing window is needed.

## Roll back to the old feeder

This command is available only on receivers migrated from the old script:

```bash
sudo /usr/local/sbin/adsbpro-feeder-rollback
```

It stops feeder v2 and starts the preserved `radarview.service` again.

## If installation fails

Do not remove the old script. Check the error with:

```bash
sudo journalctl -u adsbpro-feeder.service -n 50 --no-pager
```

The most common cause is that readsb or dump1090 is not providing data on port
`30005` or `30003`. After fixing the ADS-B source, run the installation command
again.

#!/usr/bin/env bash
set -euo pipefail

CASE="${1:?case required}"
install -d -m 0755 /fakebin /etc/systemd/system /opt /run/fake-systemctl
install -m 0755 /repo/scripts/testdata/fake-systemctl /fakebin/systemctl
printf '%s\n' '[Service]' > /etc/systemd/system/radarview.service
printf '%s\n' "USER_TOKEN = 'ADS-test-token-value'" > /opt/radarview.py
touch /run/fake-systemctl/radarview.service.active /run/fake-systemctl/radarview.service.enabled

case "$CASE" in
  active)
    OUTPUT=$(PATH="/fakebin:$PATH" ADSB_TEST_RESULT=active /repo/install-v2.sh --binary /bin/true --wait-seconds 5 2>&1)
    printf '%s\n' "$OUTPUT"
    ! printf '%s\n' "$OUTPUT" | grep -q 'ADS-test-token-value'
    test -f /run/fake-systemctl/adsbpro-feeder.service.active
    test -f /run/fake-systemctl/adsbpro-feeder.service.enabled
    test ! -f /run/fake-systemctl/radarview.service.active
    test ! -f /run/fake-systemctl/radarview.service.enabled
    test ! -e /var/lib/adsbpro-feeder/pairing-token
    test "$(stat -c %a /var/lib/adsbpro-feeder)" = 700
    test "$(stat -c %a /usr/local/sbin/adsbpro-feeder-rollback)" = 750
    find /var/backups/adsbpro-feeder -name radarview.py -print -quit | grep -q .
    ;;
  invalid|pairing-window)
    install -d -m 0700 /var/lib/adsbpro-feeder
    printf '%s' '{"state":"active"}' > /var/lib/adsbpro-feeder/status.json
    printf '%s' 'ADS-explicit-test-token' > /run/pairing-token
    chmod 0600 /run/pairing-token
    set +e
    OUTPUT=$(PATH="/fakebin:$PATH" ADSB_TEST_RESULT="$CASE" /repo/install-v2.sh --binary /bin/true --token-file /run/pairing-token --wait-seconds 5 2>&1)
    RESULT=$?
    set -e
    printf '%s\n' "$OUTPUT"
    test "$RESULT" -ne 0
    ! printf '%s\n' "$OUTPUT" | grep -q 'ADS-explicit-test-token'
    test ! -f /run/fake-systemctl/adsbpro-feeder.service.active
    test ! -f /run/fake-systemctl/adsbpro-feeder.service.enabled
    test -f /run/fake-systemctl/radarview.service.active
    test -f /run/fake-systemctl/radarview.service.enabled
    if [ "$CASE" = invalid ]; then
      printf '%s\n' "$OUTPUT" | grep -q 'token was rejected'
      grep -q INVALID_TOKEN /var/lib/adsbpro-feeder/status.json
    else
      printf '%s\n' "$OUTPUT" | grep -q 'Open a pairing window'
      grep -q PAIRING_WINDOW_REQUIRED /var/lib/adsbpro-feeder/status.json
    fi
    ;;
  legacy-disable)
    set +e
    OUTPUT=$(PATH="/fakebin:$PATH" ADSB_TEST_RESULT=active ADSB_TEST_LEGACY_DISABLE_FAIL=true /repo/install-v2.sh --binary /bin/true --wait-seconds 5 2>&1)
    RESULT=$?
    set -e
    printf '%s\n' "$OUTPUT"
    test "$RESULT" -ne 0
    printf '%s\n' "$OUTPUT" | grep -q 'avoid duplicate upload'
    test ! -f /run/fake-systemctl/adsbpro-feeder.service.active
    test ! -f /run/fake-systemctl/adsbpro-feeder.service.enabled
    test -f /run/fake-systemctl/radarview.service.active
    test -f /run/fake-systemctl/radarview.service.enabled
    ;;
  *) exit 2 ;;
esac

#!/bin/bash

rawflight_create_config() {
  echo "Creating rawflight config file..."
  wget -O https://raw.githubusercontent.com/br3jski/rawflight_eu/main/rawflight.py /opt/rawflight.py
  chmod +x /opt/rawflight.py

rawflight_create_service() {
  sleep 3
  echo "Creating RawFlight Service."
  touch rawflight.service
  echo "[Unit]
  Description=RawFlight Python service
  After=network-online.target

  [Timer]
  OnBootSec=60

  [Service]
  Type=simple
  ExecStart=opt/rawflight.py
  user=root
  Restart=on-failure
  StartLimitBurst=2

  [Install]
  WantedBy=multi-user.target
" > rawflight.service
  echo "Service created. Enabling it."
  mv rawflight.service /etc/systemd/system
  systemctl daemon-reload
  service rawflight start
  systemctl enable rawflight
}

exiter() {
  echo "User refused installing Dump1090. Exiting now"
  exit 1
}

wrongkey() {
  echo "You clicked wrong button. Exiting now. "
  exit 1
}

install_readsb() { 
  # Do nothing, function to be implemented
  #echo "Installing reADSB..."
  #echo "WARNING! After readsb installation, your feeder will be rebooted. After reboot, please run this script again!"
  #bash -c "$(wget -O - https://github.com/wiedehopf/adsb-scripts/raw/master/readsb-install.sh)"

}

if [ "$(whoami)" != "root" ]
  then echo "Please run this script as admin (use sudo or sudo su)"

  else
    if [ -x /usr/bin/dump1090-fa ] || [ -x /usr/bin/dump1090-mutability ] || [ -x /usr/bin/dump1090 ]; then {
      rawflight_create_config
      rawflight_create_service
    } else {
      echo "Dump1090 / reADSB are not installed. Please install it first. Do you want to install it now? (y/n)"
      read key
      case "$key" in
          "y") install_readsb
                rawflight_create_config
                rawflight_create_service;;
          "n") exiter;;
          *) wrongkey;;
      esac
    }
    fi
fi
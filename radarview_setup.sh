#!/bin/bash

radarview_create_config() {
  echo "Creating radarview config file..."
  wget -O /opt/radarview.py https://raw.githubusercontent.com/br3jski/radarview/main/radarview.py
  chmod +x /opt/radarview.py
}

radarview_create_service() {
  sleep 3
  echo "Creating RadarView Service."
  touch radarview.service
  echo "[Unit]
  Description=RadarView Python service
  After=network-online.target

  [Timer]
  OnBootSec=60

  [Service]
  Type=simple
  ExecStart=/opt/radarview.py
  user=root
  Restart=on-failure
  StartLimitBurst=2

  [Install]
  WantedBy=multi-user.target
" > radarview.service
  echo "Service created. Enabling it."
  mv radarview.service /etc/systemd/system
  systemctl daemon-reload
  service radarview start
  systemctl enable radarview
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
  :
}

if [ "$(whoami)" != "root" ]
  then 
    echo "Please run this script as admin (use sudo or sudo su)"
  else
    if [ -x /usr/bin/dump1090-fa ] || [ -x /usr/bin/dump1090-mutability ] || [ -x /usr/bin/dump1090 ]; then
      radarview_create_config
      radarview_create_service
    else
      echo "Dump1090 / reADSB are not installed. Please install it first. Do you want to install it now? (y/n)"
      read key
      case "$key" in
          "y") 
            install_readsb
            radarview_create_config
            radarview_create_service
            ;;
          "n") 
            exiter
            ;;
          *) 
            wrongkey
            ;;
      esac
    fi
fi

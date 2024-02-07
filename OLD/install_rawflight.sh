#!/bin/bash

install_dump()
{
  echo "Downloading Dump1090. It'll few seconds"
  wget "http://rawflight.eu/dump1090.deb"
  sleep 3
  echo "Installing Dump1090. It'll take some time"
  dpkg -i dump1090.deb
  sleep 3
  echo "Installing missing dependencies. It'll take some time. "
  apt-get --fix-broken install
  sleep 3
  echo "Starting Dump1090. "
  service dump1090-fa start
  sleep 3
  echo "Adding Dump1090 to autostart."
  systemctl enable dump1090-fa
}

rawflight_create_config() {
  echo "Dump1090 is installed. Script will now configure your Feeder. . "
  sleep 3
  echo "Configuring script........"
  echo " "
  sleep 3
  touch rawflight.sh
  echo "#!/bin/bash
  while [ 0 ]; do
    socat TCP:127.0.0.1:30005 TCP:138.201.244.70:8787
    sleep 3;
  done" > rawflight.sh

  mkdir /root/rawflight
  mv rawflight.sh /root/rawflight
  chmod +x /root/rawflight/rawflight.sh
}

rawflight_create_service() {
  sleep 3
  echo "Creating RawFlight Service."
  touch rawflight.service
  echo "[Unit]
  Description=Socat service to push receiver via socat to VRS
  After=network-online.target

  [Timer]
  OnBootSec=60

  [Service]
  Type=simple
  ExecStart=/root/rawflight/rawflight.sh
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
  echo "You clicked wrong key. Exiting now. "
  exit 1
}


if [ "$(whoami)" != "root" ]
  then echo "Please run this script as admin (use sudo or sudo su)"

  else
    if [ -x /usr/bin/dump1090-fa ] || [ -x /usr/bin/dump1090-mutability ] || [ -x /usr/bin/dump1090 ]; then {
      rawflight_create_config
      rawflight_create_service
    } else {
      echo "Dump1090 is not installed. Do you want to install it? (y/n)"
      read key
      case "$key" in
          "y") install_dump
                rawflight_create_config
                rawflight_create_service;;
          "n") exiter;;
          *) wrongkey;;
      esac
    }
    fi


fi

#!/bin/bash
validate_token() {
  local token="$1"
  # Sprawdzamy, czy token zaczyna się od "ADS-" i ma łącznie 36 znaków
  # (4 znaki "ADS-" + 32 znaki heksadecymalne)
  if [[ $token =~ ^ADS-[a-f0-9]{32}$ ]]; then
    return 0
  else
    return 1
  fi
}

get_user_token() {
  while true; do
    echo "Please enter your RadarView token (format: ADS-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX):"
    read token
    if validate_token "$token"; then
      echo "$token"
      return 0
    else
      echo "Invalid token format. Please try again."
    fi
  done
}

radarview_create_config() {
  echo "Starting radarview_create_config function..."
  echo "Creating radarview config file..."
  wget -O /opt/radarview.py https://raw.githubusercontent.com/br3jski/radarview/main/radarview.py
  if [ $? -ne 0 ]; then
    echo "Failed to download radarview.py"
    exit 1
  fi
  echo "radarview.py downloaded successfully."
  chmod +x /opt/radarview.py
  echo "Permissions set for radarview.py"
  
  echo "About to call get_user_token function..."
  user_token=$(get_user_token)
  echo "Received token: $user_token"
  
  echo "Updating radarview.py with the token..."
  sed -i "s/USER_TOKEN = ''/USER_TOKEN = '$user_token'/" /opt/radarview.py
  echo "radarview.py updated with token."
}



radarview_create_service() {
  sleep 3
  echo "Creating RadarView Service."
  SERVICE_FILE="/etc/systemd/system/radarview.service"
  if [ -f "$SERVICE_FILE" ]; then
    echo "radarview.service already exists. Overwriting..."
  fi
  echo "[Unit]
  Description=RadarView Python service
  After=network-online.target

  [Service]
  Type=simple
  ExecStartPre=/bin/sleep 60
  ExecStart=/usr/bin/python3 /opt/radarview.py
  User=root
  Restart=on-failure
  StartLimitBurst=2

  [Install]
  WantedBy=multi-user.target
" > radarview.service
  mv radarview.service "$SERVICE_FILE"
  systemctl daemon-reload
  if [ $? -ne 0 ]; then
    echo "Failed to reload systemd daemon"
    exit 1
  fi
  systemctl start radarview
  if [ $? -ne 0 ]; then
    echo "Failed to start radarview service"
    exit 1
  fi
  systemctl enable radarview
  if [ $? -ne 0 ]; then
    echo "Failed to enable radarview service"
    exit 1
  fi
}

exiter() {
  echo "User refused installing Dump1090. Exiting now"
  exit 1
}

wrongkey() {
  echo "You clicked wrong button. Exiting now."
  exit 1
}

install_dump() { 
  echo "Installing dump1090..."
  wget https://www.flightaware.com/adsb/piaware/files/packages/pool/piaware/f/flightaware-apt-repository/flightaware-apt-repository_1.1_all.deb -O /opt/farepo.deb
  if [ $? -ne 0 ]; then
    echo "Failed to download flightaware-apt-repository"
    exit 1
  fi
  dpkg -i /opt/farepo.deb
  if [ $? -ne 0 ]; then
    echo "Failed to install flightaware-apt-repository"
    exit 1
  fi
  apt update
  apt install -y dump1090-fa
  if [ $? -ne 0 ]; then
    echo "Failed to install dump1090-fa"
    exit 1
  fi
  rm /opt/farepo.deb
}

clean_up() {
  echo "Cleaning up..."
  [ -f /opt/farepo.deb ] && rm /opt/farepo.deb
}

if [ "$(whoami)" != "root" ]; then
  echo "Please run this script as admin (use sudo or sudo su)"
  exit 1
fi

if [ -x /usr/bin/dump1090-fa ] || [ -x /usr/bin/dump1090-mutability ] || [ -x /usr/bin/dump1090 ] || [ -x /usr/bin/readsb ]; then
  radarview_create_config
  radarview_create_service
else
  echo "Dump1090 / reADSB are not installed. Please install it first. Do you want to install it now? (y/n)"
  read key
  case "$key" in
    "y") 
      install_dump
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

clean_up

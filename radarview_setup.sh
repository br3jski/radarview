#!/bin/bash

validate_token() {
  local token="$1"
  if [[ $token =~ ^ADS-[a-f0-9]{32}$ ]]; then
    return 0
  else
    return 1
  fi
}

get_user_token() {
  while true; do
    read -p "Please enter your RadarView token (format: ADS-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX): " token
    if validate_token "$token"; then
      echo "$token"
      return 0
    else
      echo "Invalid token format. Please try again."
    fi
  done
}

radarview_create_config() {
  local user_token="$1"
  cp radarview/radarview.py /opt/radarview.py
  if [ $? -ne 0 ]; then
    echo "Failed to copy radarview.py"
    exit 1
  fi
  chmod +x /opt/radarview.py
  
  python3 << EOF
import re

user_token = """$user_token"""
with open('/opt/radarview.py', 'r') as file:
    content = file.read()
content = re.sub(r"USER_TOKEN = '.*'", f"USER_TOKEN = '{user_token}'", content)
with open('/opt/radarview.py', 'w') as file:
    file.write(content)
EOF
  
  if ! grep -q "USER_TOKEN = '$user_token'" /opt/radarview.py; then
    echo "Failed to update radarview.py with token. Please check the file manually."
    exit 1
  fi
}

radarview_create_service() {
  SERVICE_FILE="/etc/systemd/system/radarview.service"
  cat > "$SERVICE_FILE" << EOF
[Unit]
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
EOF

  systemctl daemon-reload
  systemctl start radarview
  systemctl enable radarview
}

install_dump() { 
  wget https://www.flightaware.com/adsb/piaware/files/packages/pool/piaware/f/flightaware-apt-repository/flightaware-apt-repository_1.1_all.deb -O /opt/farepo.deb
  dpkg -i /opt/farepo.deb
  apt update
  apt install -y dump1090-fa
  rm /opt/farepo.deb
}

clean_up() {
  [ -f /opt/farepo.deb ] && rm /opt/farepo.deb
}

# Main script
echo "Welcome to the RadarView setup script!"

if [ "$(whoami)" != "root" ]; then
  echo "Please run this script as root (use sudo or sudo su)"
  exit 1
fi

user_token=$(get_user_token)

if [ -x /usr/bin/dump1090-fa ] || [ -x /usr/bin/dump1090-mutability ] || [ -x /usr/bin/dump1090 ] || [ -x /usr/bin/readsb ]; then
  radarview_create_config "$user_token"
  radarview_create_service
else
  echo "Dump1090 / reADSB are not installed. Do you want to install it now? (y/n)"
  read -r key
  case "$key" in
    y|Y) 
      install_dump
      radarview_create_config "$user_token"
      radarview_create_service
      ;;
    n|N) 
      echo "User refused installing Dump1090. Exiting now"
      exit 1
      ;;
    *) 
      echo "Invalid input. Exiting now."
      exit 1
      ;;
  esac
fi

clean_up

echo "RadarView setup completed successfully."
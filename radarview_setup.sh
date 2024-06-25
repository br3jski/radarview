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
  echo "Creating radarview config file..."
  cp /opt/radarview/radarview.py /opt/radarview.py
  if [ $? -ne 0 ]; then
    echo "Failed to copy radarview.py"
    exit 1
  fi
  chmod +x /opt/radarview.py
  
  echo "Content of /opt/radarview.py before modification:"
  grep USER_TOKEN /opt/radarview.py
  
  echo "Updating radarview.py with the token..."
  python3 << EOF
import re

user_token = """$user_token"""
with open('/opt/radarview.py', 'r') as file:
    content = file.read()
content = re.sub(r"USER_TOKEN = '.*'", f"USER_TOKEN = '{user_token}'", content)
with open('/opt/radarview.py', 'w') as file:
    file.write(content)
print('Python script executed successfully')
EOF
  
  # Weryfikacja
  if grep -q "USER_TOKEN = '$user_token'" /opt/radarview.py; then
    echo "radarview.py successfully updated with token."
  else
    echo "Failed to update radarview.py with token. Please check the file manually."
    exit 1
  fi
  
  echo "Content of /opt/radarview.py after modification:"
  grep USER_TOKEN /opt/radarview.py
}

radarview_create_service() {
  echo "Creating RadarView Service."
  SERVICE_FILE="/etc/systemd/system/radarview.service"
  if [ -f "$SERVICE_FILE" ]; then
    echo "radarview.service already exists. Overwriting..."
  fi
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

# Główna część skryptu
echo "Welcome to the RadarView setup script!"

if [ "$(whoami)" != "root" ]; then
  echo "Please run this script as root (use sudo or sudo su)"
  exit 1
fi

user_token=$(get_user_token)
echo "Token received: $user_token"

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

echo "Final check:"
grep USER_TOKEN /opt/radarview.py
echo "RadarView setup completed successfully."
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
    echo "Please enter your RadarView token (format: ADS-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX):"
    read -p "Token: " token
    echo "Validating token..."
    if validate_token "$token"; then
      echo "Token validated successfully."
      echo "$token"
      return 0
    else
      echo "Invalid token format. Please try again."
    fi
  done
}

# Główna część skryptu
echo "Welcome to the RadarView setup script!"

if [ "$(whoami)" != "root" ]; then
  echo "Please run this script as root (use sudo or sudo su)"
  exit 1
fi

# Pytanie o token na samym początku
user_token=$(get_user_token)

radarview_create_config() {
  echo "Creating radarview config file..."
  cp /opt/radarview/radarview.py /opt/radarview.py
  if [ $? -ne 0 ]; then
    echo "Failed to copy radarview.py"
    exit 1
  fi
  chmod +x /opt/radarview.py
  
  echo "Updating radarview.py with the token..."
  python3 -c "
import re
import os

user_token = os.environ['USER_TOKEN']
with open('/opt/radarview.py', 'r') as file:
    content = file.read()
content = re.sub(r\"USER_TOKEN = '.*'\", f\"USER_TOKEN = '{user_token}'\", content)
with open('/opt/radarview.py', 'w') as file:
    file.write(content)
" USER_TOKEN="$user_token"
  
  # Weryfikacja
  if grep -q "USER_TOKEN = '$user_token'" /opt/radarview.py; then
    echo "radarview.py successfully updated with token."
  else
    echo "Failed to update radarview.py with token. Please check the file manually."
    exit 1
  fi
  
  echo "Content of /opt/radarview.py after modification:"
  cat /opt/radarview.py
}

radarview_create_service() {
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
" > "$SERVICE_FILE"
  systemctl daemon-reload
  systemctl start radarview
  systemctl enable radarview
}

# Reszta skryptu
if [ -x /usr/bin/dump1090-fa ] || [ -x /usr/bin/dump1090-mutability ] || [ -x /usr/bin/dump1090 ] || [ -x /usr/bin/readsb ]; then
  radarview_create_config
  radarview_create_service
else
  echo "Dump1090 / reADSB are not installed. Please install it first."
  exit 1
fi

echo "RadarView setup completed successfully."
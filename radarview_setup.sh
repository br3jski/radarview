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

find_adsb_decoder() {
  local candidate
  local path

  for candidate in dump1090-fa dump1090-mutability dump1090 readsb; do
    path=$(command -v "$candidate" 2>/dev/null)
    if [ -n "$path" ] && [ -x "$path" ]; then
      echo "$path"
      return 0
    fi
  done

  for path in /usr/bin/dump1090-fa /usr/bin/dump1090-mutability /usr/bin/dump1090 /usr/bin/readsb /usr/local/bin/readsb; do
    if [ -x "$path" ]; then
      echo "$path"
      return 0
    fi
  done

  return 1
}

radarview_create_config() {
  local user_token="$1"
  
  # Pobierz plik radarview.py, jeśli nie istnieje
  if [ ! -f radarview.py ]; then
    wget https://raw.githubusercontent.com/br3jski/radarview/main/radarview.py
  fi
  
  cp radarview.py /opt/radarview.py
  if [ $? -ne 0 ]; then
    echo "Failed to copy radarview.py"
    exit 1
  fi
  chmod +x /opt/radarview.py

  # Sprawdzenie wersji Pythona
  if python_command=$(command -v python3 2>/dev/null); then
    :
  elif python_command=$(command -v python 2>/dev/null); then
    :
  else
    echo "Python not found. Please install Python and try again."
    exit 1
  fi

  echo "Updating radarview.py with user token..."

  # Uaktualnienie pliku radarview.py
  "$python_command" - "$user_token" << 'EOF'
import re
import sys

user_token = sys.argv[1]
with open('/opt/radarview.py', 'r') as file:
    content = file.read()
new_token_line = "USER_TOKEN = '" + user_token + "'"
content = re.sub(r"USER_TOKEN = .*", new_token_line, content)
with open('/opt/radarview.py', 'w') as file:
    file.write(content)
EOF

  if grep -q "USER_TOKEN = '$user_token'" /opt/radarview.py; then
    echo "Successfully updated radarview.py with user token."
  else
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
ExecStart=$python_command /opt/radarview.py
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

if [ "$(id -u)" -ne 0 ]; then
  echo "Please run this script as root (use sudo or sudo su)"
  exit 1
fi

user_token=$(get_user_token)

adsb_decoder=$(find_adsb_decoder)
if [ -n "$adsb_decoder" ]; then
  echo "Found ADS-B decoder: $adsb_decoder"
  radarview_create_config "$user_token"
  radarview_create_service
else
  echo "Dump1090 / readsb are not installed. Do you want to install it now? (y/n)"
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

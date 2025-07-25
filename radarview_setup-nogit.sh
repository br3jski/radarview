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
  echo "Configuring RadarView..."
  # Download radarview.py from GitHub
  wget https://raw.githubusercontent.com/br3jski/radarview/main/radarview.py -O /opt/radarview.py
  if [ $? -ne 0 ]; then
    echo "Failed to copy radarview.py"
    exit 1
  fi
  chmod +x /opt/radarview.py

  # Sprawdzenie wersji Pythona
  if command -v python3 &>/dev/null; then
    python_command="python3"
  elif command -v python &>/dev/null; then
    python_command="python"
  else
    echo "Python not found. Please install Python and try again."
    exit 1
  fi

  # Uaktualnienie pliku radarview.py - using simpler approach for Python 2 compatibility
  # Create a temporary Python script to avoid shell injection issues
  cat > /tmp/update_token.py << 'PYTHON_SCRIPT'
import re
import sys

if len(sys.argv) != 2:
    print("Usage: update_token.py <token>")
    sys.exit(1)

user_token = sys.argv[1]

with open('/opt/radarview.py', 'r') as file:
    content = file.read()

# Replace the USER_TOKEN line - this works with both Python 2 and 3
content = re.sub(r"USER_TOKEN = '.*'", "USER_TOKEN = '" + user_token + "'", content)

with open('/opt/radarview.py', 'w') as file:
    file.write(content)
PYTHON_SCRIPT

  # Run the Python script with the token as an argument
  $python_command /tmp/update_token.py "$user_token"
  rm /tmp/update_token.py

  if ! grep -q "USER_TOKEN = '$user_token'" /opt/radarview.py; then
    echo "Failed to update radarview.py with token. Please check the file manually."
    exit 1
  fi
  echo "RadarView configuration completed."
}

radarview_create_service() {
  echo "Creating RadarView service..."
  SERVICE_FILE="/etc/systemd/system/radarview.service"
  cat > "$SERVICE_FILE" << EOF
[Unit]
Description=RadarView Python service
After=network-online.target

[Service]
Type=simple
ExecStartPre=/bin/sleep 60
ExecStart=/usr/bin/$python_command /opt/radarview.py
User=root
Restart=on-failure
StartLimitBurst=2

[Install]
WantedBy=multi-user.target
EOF

  echo "Enabling and starting RadarView service..."
  systemctl daemon-reload
  systemctl start radarview
  systemctl enable radarview
  echo "RadarView service enabled and started."
}

install_dump() { 
  echo "Installing Dump1090..."
  wget https://www.flightaware.com/adsb/piaware/files/packages/pool/piaware/f/flightaware-apt-repository/flightaware-apt-repository_1.1_all.deb -O /opt/farepo.deb
  dpkg -i /opt/farepo.deb
  apt update
  apt install -y dump1090-fa
  rm /opt/farepo.deb
  echo "Dump1090 installation completed."
}

clean_up() {
  echo "Cleaning up..."
  [ -f /opt/farepo.deb ] && rm /opt/farepo.deb
}

# Main script
echo "Welcome to the RadarView setup script!"

if [ "$(id -u)" -ne 0 ]; then
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
      echo "Skipping Dump1090 installation. Setting up RadarView only..."
      radarview_create_config "$user_token"
      radarview_create_service
      ;;
    *) 
      echo "Invalid input. Exiting now."
      exit 1
      ;;
  esac
fi

clean_up

echo "RadarView setup completed successfully."
import socket
import time

# Create a TCP socket
sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)

# Connect to the server
while True:
    try:
        sock.connect(('localhost', 30005))
        break
    except socket.error:
        time.sleep(3)

# Create a second TCP socket to connect to rawflight.eu
sock_rawflight = socket.socket(socket.AF_INET, socket.SOCK_STREAM)

# Connect to rawflight.eu
while True:
    try:
        sock_rawflight.connect(('rawflight.eu', 8787))
        break
    except socket.error:
        time.sleep(3)

# Forward data between the two sockets
while True:
    try:
        data = sock.recv(1024)
        if not data:
            break
        sock_rawflight.sendall(data)
    except socket.error:
        time.sleep(3)

# Close the sockets
sock.close()
sock_rawflight.close()
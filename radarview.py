#!/usr/bin/python3
import socket
import time
import sys

# Token użytkownika będzie wstawiany tutaj przez skrypt instalacyjny
USER_TOKEN = ''

def forward_data(source_host, source_port, dest_host, dest_port):
    connection_established = False
    while True:
        try:
            # Create the source socket
            source_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            source_socket.connect((source_host, source_port))
            print(f"Connected to source {source_host}:{source_port}")

            # Create the destination socket
            dest_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            dest_socket.connect((dest_host, dest_port))
            print("Connection to Radarview on port 48581 established")
            
            # Send the user token
            if USER_TOKEN:
                dest_socket.sendall(f"TOKEN:{USER_TOKEN}\n".encode('utf-8'))
                print("User token sent")
            else:
                print("Error: User token not set")
                sys.exit(1)
            
            connection_established = True

            # Forward data
            while True:
                data = source_socket.recv(1024)
                if not data:
                    print("Did not receive more data from source.") 
                    break  # Break the loop if no more data
                dest_socket.sendall(data)

        except socket.timeout:
            if not connection_established:
                print("Cannot connect to RadarView - got timeout")
                connection_established = True  # Protects from displaying the message again
        except socket.error as e:
            if e.errno == socket.errno.ECONNREFUSED:
                print("Cannot connect to RadarView - connection refused")
            elif e.errno == socket.errno.EPIPE:
                print("Cannot connect to RadarView - broken pipe")
            else:
                print(f"Unexpected error: {e}")
            connection_established = False  # Allows to retry the connection in the next iteration 
        finally:
            # Close the sockets
            if 'source_socket' in locals():
                source_socket.close()
            if 'dest_socket' in locals():
                dest_socket.close()

        time.sleep(3)  # Wait 3 seconds before the next attempt

if __name__ == "__main__":
    if not USER_TOKEN:
        print("Error: User token not set. Please run the installation script again.")
        sys.exit(1)
    forward_data("127.0.0.1", 30003, "feed.ads-b.pro", 48581)
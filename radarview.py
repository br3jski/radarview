#!/usr/bin/python3
import socket
import time
import sys

# Token użytkownika będzie wstawiany tutaj przez skrypt instalacyjny
USER_TOKEN = ''

def forward_data(source_host, source_port, dest_host, dest_port):
    while True:
        try:
            # Create the source socket
            source_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            source_socket.connect((source_host, source_port))
            print(f"Connected to source {source_host}:{source_port}")

            # Create the destination socket
            dest_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            dest_socket.connect((dest_host, dest_port))
            print(f"Connected to destination {dest_host}:{dest_port}")
            
            if not USER_TOKEN:
                print("Error: User token not set")
                sys.exit(1)

            # Prepare the token message
            token_message = f"TOKEN:{USER_TOKEN}\n".encode('utf-8')

            # Forward data
            while True:
                data = source_socket.recv(1024)
                if not data:
                    print("Did not receive more data from source.") 
                    break  # Break the loop if no more data
                
                # Split the data into lines
                lines = data.split(b'\n')
                
                # Process each line
                for line in lines:
                    if line:  # Skip empty lines
                        # Send the token before each line of data
                        dest_socket.sendall(token_message)
                        # Send the original line of data
                        dest_socket.sendall(line + b'\n')
                
                print("Data sent with token")

        except socket.timeout:
            print("Connection timeout")
        except socket.error as e:
            print(f"Socket error: {e}")
        finally:
            # Close the sockets
            if 'source_socket' in locals():
                source_socket.close()
            if 'dest_socket' in locals():
                dest_socket.close()

        print("Waiting before reconnecting...")
        time.sleep(3)  # Wait 3 seconds before the next attempt

if __name__ == "__main__":
    if not USER_TOKEN:
        print("Error: User token not set. Please run the installation script again.")
        sys.exit(1)
    forward_data("127.0.0.1", 30003, "feed.ads-b.pro", 48581)
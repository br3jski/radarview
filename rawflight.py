#!/usr/bin/python3
import socket
import time

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
            print("Connection to RawFlight on port 8787 established")
            connection_established = True

            # Forward data
            while True:
                data = source_socket.recv(1024)
                if not data:
                    print("Did not receive more data from source.") 
                    break  # Break the loop if no more data
                #print(f"Received data: {data[:100]}...")  # Print data for debugging
                dest_socket.sendall(data)
                #print("Data sent to the target.")

        except socket.timeout:
            if not connection_established:
                print("Cannot connect to RawFlight - got timeout")
                connection_established = True  # Protects from displaying the message again
        except socket.error as e:
            if e.errno == socket.errno.ECONNREFUSED:
                print("Cannot connect to RawFlight - connection refused")
            elif e.errno == socket.errno.EPIPE:
                print("Cannot connect to RawFlight - broken pipe")
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
    forward_data("127.0.0.1", 30005, "rawflight.eu", 8787)
#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import socket
import time
import sys
import threading
from typing import Tuple
from flask import Flask, render_template
from datetime import datetime
import logging

# Konfiguracja logowania
logging.basicConfig(
    format='%(asctime)s [%(levelname)s] %(message)s',
    level=logging.INFO,
    handlers=[
        logging.StreamHandler(),
        logging.FileHandler('radar_feeder.log')
    ]
)

class StatsCollector:
    def __init__(self):
        self.packets = 0
        self.bytes_sent = 0
        self.start_time = datetime.now()
        self.lock = threading.Lock()

    def add_packet(self, bytes_count):
        with self.lock:
            self.packets += 1
            self.bytes_sent += bytes_count

    def get_stats(self):
        with self.lock:
            return {
                'packets': self.packets,
                'bytes': self.bytes_sent,
                'runtime': datetime.now() - self.start_time
            }

app = Flask(__name__)
stats = StatsCollector()

class RadarFeeder:
    def __init__(self, token: str):
        if not token:
            logging.critical("No user token provided")
            sys.exit(1)
        self.token = f"TOKEN:{token}\n".encode('utf-8')
        self.reconnect_delay = 3
        self.buffer_size = 4096

    def _connect(self, host: str, port: int) -> socket.socket:
        """Establish TCP connection with retry logic"""
        while True:
            try:
                sock = socket.create_connection((host, port), timeout=10)
                logging.info(f"Connected to {host}:{port}")
                return sock
            except (socket.error, socket.timeout) as e:
                logging.error(f"Connection failed: {e}. Retrying in {self.reconnect_delay}s...")
                time.sleep(self.reconnect_delay)

    def _forward_data(self, source: Tuple[str, int], destination: Tuple[str, int]):
        """Main data forwarding loop"""
        while True:
            src_sock = self._connect(*source)
            dest_sock = self._connect(*destination)
            
            try:
                while True:
                    data = src_sock.recv(self.buffer_size)
                    if not data:
                        logging.warning("Source connection closed")
                        break

                    for line in data.split(b'\n'):
                        if line.strip():
                            dest_sock.sendall(self.token)
                            dest_sock.sendall(line + b'\n')
                            stats.add_packet(len(line) + len(self.token))
                            logging.debug(f"Sent data line: {line[:50]}...")

            except (socket.error, socket.timeout) as e:
                logging.error(f"Connection error: {e}")
            finally:
                src_sock.close()
                dest_sock.close()
                logging.info("Connections closed, reconnecting...")
                time.sleep(self.reconnect_delay)

    def run(self, source: Tuple[str, int], destination: Tuple[str, int]):
        try:
            self._forward_data(source, destination)
        except KeyboardInterrupt:
            logging.info("Shutting down gracefully")

@app.route('/')
def dashboard():
    return render_template('dashboard.html')

@app.route('/stats')
def get_stats():
    data = stats.get_stats()
    return {
        'packets': data['packets'],
        'gb_sent': round(data['bytes'] / (1024**3), 3),
        'runtime': str(data['runtime']).split('.')[0]
    }

def run_flask():
    app.run(port=5000, threaded=True)

if __name__ == "__main__":
    USER_TOKEN = 'afsdfsfsas'  # Wypełniane przez skrypt instalacyjny
    
    if not USER_TOKEN:
        logging.critical("Missing user token. Run installation script first.")
        sys.exit(1)

    # Uruchom serwer Flask w osobnym wątku
    flask_thread = threading.Thread(target=run_flask, daemon=True)
    flask_thread.start()

    # Uruchom główną aplikację
    feeder = RadarFeeder(USER_TOKEN)
    feeder.run(
        source=("127.0.0.1", 30003),
        destination=("feed.ads-b.pro", 48581)
    )

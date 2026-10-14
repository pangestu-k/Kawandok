from __future__ import annotations

import socket
import struct
from typing import Protocol


class Scanner(Protocol):
    def scan(self, data: bytes) -> None: ...


class MalwareDetected(Exception):
    pass


class ScannerUnavailable(Exception):
    pass


class DisabledScanner:
    def scan(self, data: bytes) -> None:
        return None


class ClamAVScanner:
    """Minimal ClamAV INSTREAM client; scanned bytes never touch disk."""

    def __init__(self, host: str, port: int, timeout_seconds: float) -> None:
        self._address = (host, port)
        self._timeout_seconds = timeout_seconds

    def scan(self, data: bytes) -> None:
        try:
            with socket.create_connection(self._address, self._timeout_seconds) as conn:
                conn.settimeout(self._timeout_seconds)
                conn.sendall(b"zINSTREAM\x00")
                for offset in range(0, len(data), 64 * 1024):
                    chunk = data[offset : offset + 64 * 1024]
                    conn.sendall(struct.pack("!I", len(chunk)) + chunk)
                conn.sendall(struct.pack("!I", 0))
                response = conn.recv(4096).decode("utf-8", "replace").rstrip("\x00")
        except OSError as exc:
            raise ScannerUnavailable("ClamAV is unavailable") from exc

        if response.endswith(" OK"):
            return
        if response.endswith(" FOUND"):
            raise MalwareDetected("document failed malware scan")
        raise ScannerUnavailable("ClamAV returned an invalid response")

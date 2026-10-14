from __future__ import annotations

import unittest
from unittest.mock import patch

from worker.scanner import ClamAVScanner, MalwareDetected, ScannerUnavailable


class FakeConnection:
    def __init__(self, response: bytes) -> None:
        self.response = response
        self.sent = b""

    def __enter__(self) -> "FakeConnection":
        return self

    def __exit__(self, *args: object) -> None:
        return None

    def settimeout(self, timeout: float) -> None:
        return None

    def sendall(self, data: bytes) -> None:
        self.sent += data

    def recv(self, size: int) -> bytes:
        return self.response


class ClamAVScannerTests(unittest.TestCase):
    def test_clean_document_uses_instream_protocol(self) -> None:
        conn = FakeConnection(b"stream: OK\x00")
        with patch("worker.scanner.socket.create_connection", return_value=conn):
            ClamAVScanner("scanner", 3310, 2).scan(b"synthetic document")
        self.assertTrue(conn.sent.startswith(b"zINSTREAM\x00"))
        self.assertTrue(conn.sent.endswith(b"\x00\x00\x00\x00"))

    def test_malware_is_rejected(self) -> None:
        conn = FakeConnection(b"stream: Eicar-Test-Signature FOUND\x00")
        with patch("worker.scanner.socket.create_connection", return_value=conn):
            with self.assertRaises(MalwareDetected):
                ClamAVScanner("scanner", 3310, 2).scan(b"synthetic malware fixture")

    def test_invalid_response_fails_closed(self) -> None:
        conn = FakeConnection(b"stream: ERROR\x00")
        with patch("worker.scanner.socket.create_connection", return_value=conn):
            with self.assertRaises(ScannerUnavailable):
                ClamAVScanner("scanner", 3310, 2).scan(b"synthetic document")

from __future__ import annotations

import contextlib

import psycopg


@contextlib.contextmanager
def connect(dsn: str, connect_timeout: int = 10):
    conn = psycopg.connect(dsn, connect_timeout=connect_timeout)
    try:
        yield conn
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()

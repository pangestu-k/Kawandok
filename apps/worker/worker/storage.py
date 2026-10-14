from __future__ import annotations

from typing import Protocol


class ObjectNotFound(Exception):
    pass


class StorageError(Exception):
    pass


class StorageFetcher(Protocol):
    def fetch(self, bucket: str, key: str) -> bytes:
        ...


class MemoryFetcher:
    def __init__(self, objects: dict[tuple[str, str], bytes] | None = None) -> None:
        self._objects = dict(objects or {})

    def put(self, bucket: str, key: str, data: bytes) -> None:
        self._objects[(bucket, key)] = data

    def fetch(self, bucket: str, key: str) -> bytes:
        try:
            return self._objects[(bucket, key)]
        except KeyError as exc:
            raise ObjectNotFound(f"{bucket}/{key}") from exc


class S3Fetcher:
    def __init__(self, endpoint: str, access_key: str, secret_key: str) -> None:
        import boto3
        from botocore.config import Config

        self._client = boto3.client(
            "s3",
            endpoint_url=endpoint,
            aws_access_key_id=access_key,
            aws_secret_access_key=secret_key,
            region_name="auto",
            config=Config(
                retries={"max_attempts": 2},
                connect_timeout=5,
                read_timeout=60,
            ),
        )

    def fetch(self, bucket: str, key: str) -> bytes:
        from botocore.exceptions import ClientError

        try:
            response = self._client.get_object(Bucket=bucket, Key=key)
            return response["Body"].read()
        except ClientError as exc:
            code = exc.response.get("Error", {}).get("Code", "")
            if code in ("NoSuchKey", "404", "NotFound"):
                raise ObjectNotFound(f"{bucket}/{key}") from exc
            raise StorageError(f"s3 get_object failed: {exc}") from exc
        except Exception as exc:
            raise StorageError(f"s3 get_object failed: {exc}") from exc


def build_fetcher(cfg) -> StorageFetcher:
    if cfg.storage_backend == "s3":
        return S3Fetcher(
            endpoint=cfg.storage_endpoint,
            access_key=cfg.storage_access_key,
            secret_key=cfg.storage_secret_key,
        )
    return MemoryFetcher()

from __future__ import annotations

import logging
import sys

from worker.config import WorkerConfig


def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    log = logging.getLogger("worker")
    try:
        WorkerConfig.from_env()
    except ValueError as exc:
        log.error("invalid configuration: %s", exc)
        return 1
    log.info("worker stub ready (M0); queue consumption arrives in M2")
    return 0


if __name__ == "__main__":
    sys.exit(main())

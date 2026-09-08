from __future__ import annotations

import sys
from dataclasses import replace

import remote_deploy


def main() -> int:
    try:
        config = remote_deploy.load_remote_config()
        config = replace(config, deploy_target="plugin_service")
        remote_deploy.deploy_to_remote(config)
        return 0
    except Exception as exc:  # pragma: no cover - CLI fallback
        print(f"Plugin service remote deploy failed: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())

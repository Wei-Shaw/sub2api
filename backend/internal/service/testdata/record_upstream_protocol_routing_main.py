#!/usr/bin/env python3
"""Record the routing golden from the pinned main source, never the checkout."""

import io
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile


def format_golden(fixture):
    """Keep observations readable and wrap case indexes in compact rows."""
    metadata = {key: value for key, value in fixture.items() if key != "results"}
    prefix = json.dumps(metadata, ensure_ascii=False, indent=2)[:-2]
    indexes = fixture["results"]
    rows = ["    " + ", ".join(map(str, indexes[i:i + 32])) for i in range(0, len(indexes), 32)]
    return (prefix + ',\n  "results": [\n' + ",\n".join(rows) + "\n  ]\n}\n").encode("utf-8")


def main():
    repo = Path(__file__).resolve().parents[4]
    driver = repo / "backend/internal/service/upstream_protocol_routing_matrix_test.go"
    source = driver.read_text()
    revision = re.search(r'const routingMatrixMainRevision = "([0-9a-f]{40})"', source).group(1)
    subprocess.run(["git", "cat-file", "-e", revision + "^{commit}"], cwd=repo, check=True)
    archive = subprocess.check_output(["git", "archive", revision, "backend"], cwd=repo)

    with tempfile.TemporaryDirectory(prefix="sub2api-routing-main-") as scratch:
        root = Path(scratch)
        with tarfile.open(fileobj=io.BytesIO(archive)) as files:
            files.extractall(root, filter="data")
        recording_driver = root / "backend/internal/service/upstream_protocol_routing_matrix_test.go"
        recording_driver.write_text(source + r'''

func TestRecordUpstreamProtocolRoutingMainGolden(t *testing.T) {
    gin.SetMode(gin.TestMode)
    data, err := json.MarshalIndent(captureRoutingMatrixGolden(), "", "  ")
    require.NoError(t, err)
    require.NoError(t, os.WriteFile(os.Getenv("ROUTING_MATRIX_RECORD_OUTPUT"), append(data, '\n'), 0o644))
}
''')
        output = root / "routing_main_golden.json"
        environment = dict(os.environ, ROUTING_MATRIX_RECORD_OUTPUT=str(output))
        subprocess.run(
            ["go", "test", "-tags", "unit", "./internal/service/", "-run", "^TestRecordUpstreamProtocolRoutingMainGolden$", "-count=1"],
            cwd=root / "backend", env=environment, check=True,
        )
        data = output.read_bytes()
        fixture = json.loads(data)
        if fixture["source_revision"] != revision:
            raise RuntimeError("Recorded source revision differs from the pinned revision")
        destination = driver.parent / "testdata/upstream_protocol_routing_main_golden.json"
        destination.write_bytes(format_golden(fixture))
        print(f"Recorded {fixture['case_count']} cases from {revision} into {destination}")


if __name__ == "__main__":
    main()

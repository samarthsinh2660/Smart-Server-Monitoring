"""Check process isolation in the real Pi launcher with stub service executables."""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import unittest


class PiLauncherTests(unittest.TestCase):
    def test_prediction_failure_keeps_controller_running_and_cleanup_stops_it(self):
        launcher = Path(__file__).resolve().parents[1] / "start-pi.sh"
        for missing_dependencies in (False, True):
            with self.subTest(missing_dependencies=missing_dependencies), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                shutil.copy2(launcher, root / "start-pi.sh")
                (root / "stubs").mkdir()
                (root / "model/artifacts").mkdir(parents=True)
                (root / "model/artifacts/serverless_model.joblib").touch()
                controller = root / "fake-controller"
                controller.write_text("""#!/usr/bin/env python3
import signal
import time
from pathlib import Path
def stop(*args):
    Path('controller_stopped').touch()
    raise SystemExit(0)
signal.signal(signal.SIGTERM, stop)
Path('controller_started').touch()
while True:
    time.sleep(0.02)
""")
                go = root / "stubs/go"
                go.write_text("#!/bin/sh\ncp fake-controller bin/controller\nchmod +x bin/controller\n")
                python = root / "fake-python"
                python.write_text("#!/bin/sh\nif [ \"$1\" = '-c' ]; then exit " + ("1" if missing_dependencies else "0") + "; fi\ntouch model_exited\nexit 7\n")
                for path in (controller, go, python):
                    path.chmod(0o755)
                env = {**os.environ, "PATH": str(root / "stubs") + os.pathsep + os.environ["PATH"], "APP_URL": "http://primary:8000", "AGENT_URL": "http://primary:8081", "MODEL_PYTHON": str(python), "AI_SCALING_VALIDATED": "false"}
                process = subprocess.Popen(["bash", "start-pi.sh"], cwd=root, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                try:
                    deadline = time.monotonic() + 5
                    while time.monotonic() < deadline and not (root / "controller_started").exists():
                        time.sleep(0.02)
                    self.assertTrue((root / "controller_started").exists(), "controller did not start")
                    if not missing_dependencies:
                        while time.monotonic() < deadline and not (root / "model_exited").exists():
                            time.sleep(0.02)
                        self.assertTrue((root / "model_exited").exists(), "model failure was not exercised")
                    time.sleep(0.1)
                    self.assertIsNone(process.poll(), "prediction failure stopped the controller launcher")
                finally:
                    process.terminate()
                    try:
                        process.communicate(timeout=5)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.communicate()
                self.assertTrue((root / "controller_stopped").exists(), "launcher did not clean up its controller")


if __name__ == "__main__":
    unittest.main()

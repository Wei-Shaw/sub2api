import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('verify_customizations.py')
spec = importlib.util.spec_from_file_location('guard', SCRIPT)
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)


class GuardTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.registry = self.root / 'CUSTOMIZATIONS.md'
        self.registry.write_text('### C-001 Notice\n- 文件：app.txt\n- 断言："long_notice_content"\n')
        self.registry.with_name('custom-files.json').write_text(json.dumps({'files': ['app.txt']}))
        (self.root / 'app.txt').write_text('app\nlong_notice_content\n')

    def test_valid(self):
        self.assertEqual(guard.verify(self.root, self.registry), [])

    def test_in_file_customization_removal_is_blocked(self):
        (self.root / 'app.txt').write_text('app\n')
        self.assertTrue(guard.verify(self.root, self.registry))

    def test_missing_file_is_blocked(self):
        (self.root / 'app.txt').unlink()
        self.assertTrue(guard.verify(self.root, self.registry))

    def test_empty_registry_is_blocked(self):
        self.registry.write_text('')
        self.assertTrue(guard.verify(self.root, self.registry))

    def test_malformed_assertion_is_blocked(self):
        self.registry.write_text('### C-001 Notice\n- 文件：app.txt\n- 断言：not-json\n')
        self.assertTrue(guard.verify(self.root, self.registry))

    def test_failure_reaches_shell_exit_code(self):
        (self.root / 'app.txt').write_text('upstream replacement')
        result = subprocess.run(['python3', str(SCRIPT), str(self.root), '--registry', str(self.registry)], capture_output=True)
        self.assertNotEqual(result.returncode, 0)


if __name__ == '__main__':
    unittest.main()

import contextlib
import importlib.util
import io
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('gate', ROOT / 'tools/security/check-go-findings.py')
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class GoFindingsTests(unittest.TestCase):
    def setUp(self):
        self.source = (ROOT / gate.NAME_SOURCE).read_bytes()
        self.finding = {'osv': 'GO-2026-4887', 'trace': [{
            'module': 'github.com/docker/docker', 'version': 'v28.5.2+incompatible',
            'package': 'github.com/docker/docker/pkg/namesgenerator', 'function': 'GetRandomName'}]}

    def report(self, finding):
        return json.dumps({'config': {'scan_level': 'symbol', 'scan_mode': 'source'}}) + json.dumps({'finding': finding})

    def test_exact_review_only(self):
        self.assertTrue(gate.reviewed(self.finding, self.source))
        for key, value in [('module', 'other'), ('version', 'v28.5.3'), ('package', 'github.com/docker/docker/plugin'), ('function', 'ValidatePrivileges')]:
            changed = json.loads(json.dumps(self.finding))
            changed['trace'][0][key] = value
            self.assertFalse(gate.reviewed(changed, self.source))
        self.assertFalse(gate.reviewed(self.finding, self.source + b'// change'))
        self.finding['osv'] = 'GO-2099-0001'
        self.assertFalse(gate.reviewed(self.finding, self.source))

    def test_other_findings_fail(self):
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertTrue(gate.check(self.report(self.finding), self.source))
            self.finding['trace'][0]['package'] = 'github.com/docker/docker/plugin'
            self.assertFalse(gate.check(self.report(self.finding), self.source))

    def test_malformed_reports_fail(self):
        for text in ['', '{}', '{', '{"config":{"scan_level":"package"}}']:
            with self.assertRaises(ValueError):
                gate.check(text, self.source)


if __name__ == '__main__':
    unittest.main()

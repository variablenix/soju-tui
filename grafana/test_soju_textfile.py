import importlib.util
from pathlib import Path
import sys
import tempfile
import time
import unittest

spec = importlib.util.spec_from_file_location("collector", Path(__file__).with_name("soju-textfile.py"))
collector = importlib.util.module_from_spec(spec)
spec.loader.exec_module(collector)


class CollectorTests(unittest.TestCase):
    def test_aggregates_connected_channels_and_separates_disabled(self):
        responses = {
            ("server", "status"): "2/2 users, 3 downstreams, 2 upstreams, 4 networks, 12 channels\n",
            ("user", "status"): "example: user (admin): 3 networks\nother: 1 networks (5 max)\n",
            ("user", "run", "example: user", "network", "status"):
                "one (ircs://irc.example:6697) [connected]: 7 channels\n"
                "paused [disabled]\nfailed [disconnected]: certificate expired\n",
            ("user", "run", "other", "network", "status"): "two [connected]: 3 channels\n",
        }
        counts = collector.collect(lambda args: responses[tuple(args)])
        self.assertEqual(counts, {"networks_configured": 4, "networks_connected": 2,
                                 "networks_disabled": 1, "networks_disconnected": 1,
                                 "channels_configured": 12, "channels_joined": 10})
        rendered = collector.render("example-bouncer:18081", counts, 123)
        self.assertNotIn("example: user", rendered)
        self.assertNotIn("irc.example", rendered)
        self.assertNotIn("certificate", rendered)

    def test_rejects_truncated_unknown_and_ambiguous_listings(self):
        for text, total in [("alice: 1 networks\n(1 more users omitted)", 2),
                            ("alice: 1 networks", 2),
                            ("alice (admin): 1 networks\nalice: 1 networks", 2),
                            ("unrecognized output", 1)]:
            with self.subTest(text=text), self.assertRaises(collector.CollectionError):
                collector.users_status(text, total)
        for text, total in [("one [connected]: 3 channels", 2), ("unknown", 1),
                            ("one [connected]: not a number", 1),
                            ("one [disabled]: injected detail", 1)]:
            with self.subTest(text=text), self.assertRaises(collector.CollectionError):
                collector.network_status(text, total)

    def test_disconnected_error_cannot_spoof_a_connected_suffix(self):
        self.assertEqual(collector.network_status(
            "one [disconnected]: server says fake [connected]: 999 channels", 1), (0, 0, 0))

    def test_empty_instance_and_no_networks(self):
        self.assertEqual(collector.users_status("", 0), [])
        self.assertEqual(collector.network_status(
            'No network configured, add one with "network create".', 0), (0, 0, 0))
        with self.assertRaises(collector.CollectionError):
            collector.render("bad\nlabel", {}, 1)

    def test_configuration_race_fails_instead_of_publishing_partial_counts(self):
        replies = iter(["1/1 users, 0 downstreams, 0 upstreams, 0 networks, 0 channels",
                        "alice: 0 networks", 'No network configured, add one with "network create".',
                        "1/1 users, 0 downstreams, 0 upstreams, 1 networks, 0 channels"])
        with self.assertRaises(collector.CollectionError):
            collector.collect(lambda _: next(replies))

    def test_atomic_success_then_failure_removes_old_counts(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "soju.prom"
            collector.write_atomic(path, collector.render("example:18081", {"channels_joined": 4}, 1))
            self.assertIn("channels_joined", path.read_text())
            collector.write_atomic(path, collector.render("example:18081", None, 2))
            content = path.read_text()
            self.assertNotIn("channels_joined", content)
            self.assertIn('soju_admin_collection_success{soju_instance="example:18081"} 0', content)
            self.assertEqual(list(Path(directory).iterdir()), [path])
            self.assertEqual(path.stat().st_mode & 0o777, 0o644)

    def test_bounds_timeout_and_output_without_leaking_output(self):
        for code, duration in [("import time; time.sleep(10)", 0.1),
                               ("print('x' * 1100000)", 3),
                               ("import sys; print('private output'); sys.exit(1)", 3)]:
            with self.subTest(code=code), self.assertRaises(collector.CollectionError) as error:
                collector.command([sys.executable, "-c", code], time.monotonic() + duration)
            self.assertNotIn("private output", str(error.exception))


if __name__ == "__main__":
    unittest.main()

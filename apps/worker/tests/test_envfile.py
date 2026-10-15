from __future__ import annotations

import os
import unittest

from worker.envfile import find_dotenv, load_dotenv


class EnvFileTests(unittest.TestCase):
    def setUp(self) -> None:
        for key in ("DOTENV_TEST_A", "DOTENV_TEST_B", "DOTENV_TEST_C"):
            os.environ.pop(key, None)

    def test_load_sets_missing_keys_and_keeps_existing(self) -> None:
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, ".env")
            with open(path, "w", encoding="utf-8") as handle:
                handle.write(
                    "# comment\n"
                    "DOTENV_TEST_A=alpha\n"
                    "DOTENV_TEST_B='quoted'\n"
                    "export DOTENV_TEST_C=exported\n"
                )
            os.environ["DOTENV_TEST_A"] = "existing"

            loaded = load_dotenv(tmp)

            self.assertEqual(loaded, path)
            self.assertEqual(os.environ["DOTENV_TEST_A"], "existing")
            self.assertEqual(os.environ["DOTENV_TEST_B"], "quoted")
            self.assertEqual(os.environ["DOTENV_TEST_C"], "exported")

    def test_find_walks_up(self) -> None:
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            nested = os.path.join(tmp, "a", "b")
            os.makedirs(nested)
            with open(os.path.join(tmp, ".env"), "w", encoding="utf-8") as handle:
                handle.write("X=1\n")
            self.assertEqual(find_dotenv(nested), os.path.join(tmp, ".env"))

    def test_missing_file_returns_none(self) -> None:
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            self.assertIsNone(load_dotenv(tmp))


if __name__ == "__main__":
    unittest.main()

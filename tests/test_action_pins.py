"""Offline positive and negative supply-chain policy fixtures."""
import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("action_policy", ROOT / ".github/scripts/check_action_pins.py")
policy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy)


class ActionPinTests(unittest.TestCase):
    def test_accepts_reviewed_commit_and_local_action(self):
        for value in ("owner/action@" + "a" * 40 + " # v1.0.1", "./.github/actions/check"):
            self.assertEqual(policy.violations("      - uses: " + value), [])

    def test_rejects_mutable_and_malformed_action_fixtures(self):
        for value in ("actions/checkout@v7.0.1", "actions/checkout@main", "actions/checkout@1234", "actions/checkout@" + "g" * 40, "docker://alpine:latest", "${{ inputs.action }}", "|"):
            for template in ("- uses: {}", '- uses: "{}"', "- {{uses: {}}}"):
                self.assertTrue(policy.violations(template.format(value)), value)

    def test_structural_policy_cannot_be_bypassed_with_yaml_syntax(self):
        for fixture in ('[{"uses": "owner/action@v1"}]', '- "uses": owner/action@v1', '- uses: >-\n    owner/action@v1', '- &action {uses: owner/action@v1}\n- *action', r'- {"u\u0073es": "owner/action@v1"}'):
            self.assertTrue(policy.violations(fixture), fixture)

    def test_broad_credentials_and_persisted_checkout_fail(self):
        for fixture in ('env: {TOKEN: "${{ secrets.TOKEN }}"}', 'jobs: {publish: {env: {TOKEN: "${{ secrets.TOKEN }}"}}}', 'permissions: {contents: write}', '- uses: actions/checkout@' + 'a' * 40):
            self.assertTrue(policy.violations(fixture), fixture)

    def test_indexed_secrets_cannot_bypass_scope_policy(self):
        import yaml
        for reference in ("${{ secrets['TOKEN'] }}", "${{ secrets [ 'TOKEN' ] }}"):
            fixtures = (
                {'env': {'TOKEN': reference}},
                {'jobs': {'publish': {'env': {'TOKEN': reference}}}},
                [{'uses': 'actions/checkout@' + 'a' * 40,
                  'with': {'persist-credentials': False, 'token': reference}}],
            )
            for fixture, expected in zip(fixtures, (
                    'publication secrets must be step scoped',
                    'publication secrets must be step scoped',
                    'checkout must not receive publication secrets')):
                with self.subTest(reference=reference, fixture=fixture):
                    self.assertIn(expected, policy.violations(yaml.safe_dump(fixture)))

    def test_indexed_secrets_remain_allowed_at_publication_step(self):
        fixture = """jobs:
  publish:
    steps:
      - uses: docker/login-action@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
        with:
          password: ${{ secrets['TOKEN'] }}
"""
        self.assertEqual(policy.violations(fixture), [])

    def test_all_repository_workflows_are_immutable(self):
        paths = list((ROOT / ".github/workflows").glob("*.y*ml"))
        self.assertTrue(paths)
        for path in paths:
            with self.subTest(workflow=path.name):
                self.assertEqual(policy.violations(path.read_text()), [])

    def test_dependency_updates_retain_pins(self):
        import json
        config = json.loads((ROOT / "renovate.json").read_text())
        self.assertTrue(any("github-actions" in rule.get("matchManagers", []) and rule.get("pinDigests") is True for rule in config["packageRules"]))


if __name__ == "__main__":
    unittest.main()

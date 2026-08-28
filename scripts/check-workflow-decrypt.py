#!/usr/bin/env python3
"""Fail if a workflow job compiles or tests the source without decrypting it first.

Source encryption stores .go, .ts, .tsx, .sql and .sh files as ciphertext. A job
that checks the repository out and then runs a build tool reads that ciphertext,
and the error it produces points anywhere but at the cause:

    Makefile:1: *** missing separator.  Stop.
    core/internal/version/version_default.go says '', VERSION says '0.5.0'.
    read core/cmd/api/main.go: unexpected NUL in input

Three separate jobs shipped with this defect in a single day, each found only
after it broke on main. This check is cheaper than finding the fourth the same
way.

A job needs ./.github/actions/decrypt-source when it BOTH checks the repository
out AND runs one of the build tools below. A job that checks out only to read
VERSION or the changelog does not, because those files are never encrypted.
"""

import sys
import pathlib
import re

try:
    import yaml
except ImportError:
    sys.exit("PyYAML is required: pip install pyyaml")

# Tools that read source files. Matched as words so that, for example, a job
# echoing the word "going" is not mistaken for one that runs `go`.
BUILD_TOOLS = re.compile(
    r"(?<![\w./-])(go|gofmt|golangci-lint|npm|npx|node|make|tsc|shellcheck)(?![\w-])"
)

WORKFLOWS = pathlib.Path(".github/workflows")
DECRYPT_ACTION = "./.github/actions/decrypt-source"


def job_steps(job):
    steps = job.get("steps")
    return steps if isinstance(steps, list) else []


def main() -> int:
    problems = []

    for path in sorted(WORKFLOWS.glob("*.yml")) + sorted(WORKFLOWS.glob("*.yaml")):
        try:
            doc = yaml.safe_load(path.read_text(encoding="utf-8"))
        except yaml.YAMLError as exc:
            problems.append(f"{path}: is not valid YAML: {exc}")
            continue
        if not isinstance(doc, dict):
            continue

        for job_id, job in (doc.get("jobs") or {}).items():
            if not isinstance(job, dict):
                continue

            steps = job_steps(job)
            checks_out = any(
                isinstance(s, dict) and "actions/checkout" in str(s.get("uses", ""))
                for s in steps
            )
            if not checks_out:
                continue

            decrypts = any(
                isinstance(s, dict) and DECRYPT_ACTION in str(s.get("uses", ""))
                for s in steps
            )
            if decrypts:
                continue

            for step in steps:
                if not isinstance(step, dict):
                    continue
                run = step.get("run")
                if not isinstance(run, str) or not BUILD_TOOLS.search(run):
                    continue
                name = step.get("name") or "(unnamed step)"
                problems.append(
                    f"{path.name}: job '{job_id}', step '{name}' runs a build tool "
                    f"after checkout but the job never uses {DECRYPT_ACTION}.\n"
                    f"    It will read encrypted source and fail with something that "
                    f"does not name the real cause."
                )
                break

    if problems:
        print("Workflow jobs that would read encrypted source:\n")
        for p in problems:
            print(f"  - {p}\n")
        print(
            "Add this immediately after the checkout step in each job listed:\n\n"
            "      - name: Decrypt source\n"
            "        uses: ./.github/actions/decrypt-source\n"
            "        env:\n"
            "          VKAI_CRYPT_KEY: ${{ secrets.VKAI_CRYPT_KEY }}\n\n"
            "The action does nothing on a repository that has not enabled\n"
            "encryption, so it is safe to add in either state."
        )
        return 1

    print("Every workflow job that builds decrypts first.")
    return 0


if __name__ == "__main__":
    sys.exit(main())

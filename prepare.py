"""
prepare.py — fixed, one-time data prep. Agent NEVER edits this.
Calls SEAI's data_gen to generate training data from trajectories/feedback.
Runs before each experiment arm.

Usage:
    python prepare.py --exp-id <id> [--data-type sft|grpo]
    python prepare.py --exp-id <id> --data-type sft    # for SFT experiments
    python prepare.py --exp-id <id> --data-type grpo   # for GRPO experiments

Karpathy equivalent: autoresearch/prepare.py
"""
import sys
import json
from pathlib import Path

SEAI_ROOT = Path("/Users/Shared/SubjectiveEgoneticsAI")
sys.path.insert(0, str(SEAI_ROOT))

from models.training.data_gen import generate_sft_from_feedback, generate_grpo_from_trajectories

EXPERIMENTS_DIR = Path.home() / ".egonetics" / "experiments"


def main():
    exp_id = None
    data_type = "sft"

    i = 1
    while i < len(sys.argv):
        arg = sys.argv[i]
        if arg == "--exp-id" and i + 1 < len(sys.argv):
            exp_id = sys.argv[i + 1]
            i += 2
        elif arg == "--data-type" and i + 1 < len(sys.argv):
            data_type = sys.argv[i + 1]
            i += 2
        else:
            i += 1

    if not exp_id:
        print(json.dumps({"ok": False, "error": "--exp-id required"}))
        sys.exit(1)

    exp_dir = EXPERIMENTS_DIR / exp_id
    exp_dir.mkdir(parents=True, exist_ok=True)

    output_path = str(exp_dir / f"{data_type}_data.json")

    if data_type == "sft":
        samples = generate_sft_from_feedback(output_path)
    elif data_type == "grpo":
        samples = generate_grpo_from_trajectories(output_path)
    else:
        print(json.dumps({"ok": False, "error": f"unknown data_type: {data_type}"}))
        sys.exit(1)

    print(json.dumps({
        "ok": True,
        "data_type": data_type,
        "samples": len(samples),
        "output": output_path,
    }))


if __name__ == "__main__":
    main()

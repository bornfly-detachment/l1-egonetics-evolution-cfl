#!/usr/bin/env bash
# prepare.py 走 SEAI，使用 LLaMA-Factory venv（与 SubjectiveEgoneticsAI README 一致）
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LLAMA_VENV="${LLAMA_FACTORY_VENV:-$HOME/llama-factory/venv}"
PYTHON="${LLAMA_VENV}/bin/python"

if [[ ! -x "$PYTHON" ]]; then
  echo "error: LLaMA-Factory venv not found: $PYTHON" >&2
  echo "  set LLAMA_FACTORY_VENV to your venv root, or install ~/llama-factory/venv" >&2
  exit 1
fi

cd "$REPO_ROOT"
exec "$PYTHON" prepare.py "$@"

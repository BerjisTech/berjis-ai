#!/usr/bin/env sh
set -e

# Start Ollama server in background
ollama serve &
PID=$!

# Wait for API to be ready (avoid curl; use ollama CLI)
for i in $(seq 1 60); do
  if ollama list >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

# Helper: ensure model is present; pull if missing
ensure_model() {
  NAME="$1"
  [ -z "$NAME" ] && return 0
  # Use ollama CLI to check availability
  if ! ollama show "$NAME" >/dev/null 2>&1; then
    echo "[ollama-bootstrap] pulling $NAME"
    ollama pull "$NAME" || exit 1
  else
    echo "[ollama-bootstrap] model $NAME already present"
  fi
}

ensure_model "$DEFAULT_MODEL"
ensure_model "$EMBEDDINGS_MODEL"

# Bring server to foreground (so container stops when it exits)
wait "$PID"

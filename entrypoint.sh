#!/bin/sh

# Start Ollama in the background
ollama serve &

# Wait for Ollama to be ready
until curl -s http://localhost:11434/api/tags > /dev/null; do
  echo "Waiting for Ollama..."
  sleep 2
done

# Start the FlakyOllama Agent
exec ./flakyollama

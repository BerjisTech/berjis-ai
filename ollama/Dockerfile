FROM ollama/ollama:latest

# Optional defaults can be overridden at compose/runtime
ENV OLLAMA_HOST=0.0.0.0 \
    DEFAULT_MODEL=llama3.1:8b \
    EMBEDDINGS_MODEL=nomic-embed-text

COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 11434
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]


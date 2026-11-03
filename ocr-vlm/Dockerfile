# Pinned for the GB10 / CUDA 13.0 path. Adjust only with a recorded smoke test.
FROM python:3.12-slim@sha256:b18b261e1b4974b4f321451ad5251ded85abb22c2b8b262da527dda1d031ab30

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    HF_HOME=/models/huggingface \
    TRANSFORMERS_CACHE=/models/huggingface \
    LOAD_MODEL_ON_STARTUP=true

RUN apt-get update \
    && apt-get install -y --no-install-recommends libgl1 libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir \
    torch==2.12.1 torchvision==0.27.1 \
    --index-url https://download.pytorch.org/whl/cu130

RUN pip install --no-cache-dir \
    transformers==4.57.1 \
    fastapi \
    "uvicorn[standard]" \
    pydantic \
    pillow \
    pymupdf \
    matplotlib \
    einops \
    addict \
    easydict \
    psutil \
    tqdm \
    numpy \
    safetensors

RUN useradd --create-home --uid 10001 loklingo \
    && mkdir -p /models/huggingface \
    && chown -R loklingo:loklingo /models
USER loklingo
WORKDIR /app
COPY --chown=loklingo:loklingo server.py /app/server.py

EXPOSE 10000
HEALTHCHECK --interval=30s --timeout=10s --start-period=300s --retries=5 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:10000/health')"

CMD ["uvicorn", "server:app", "--host", "0.0.0.0", "--port", "10000"]

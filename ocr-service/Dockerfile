FROM python:3.11-slim

WORKDIR /app

# System libraries required by OpenCV (paddleocr), pdf2image (poppler), and Pillow
RUN apt-get update && apt-get install -y --no-install-recommends \
    libglib2.0-0 \
    libsm6 \
    libxrender1 \
    libxext6 \
    libgl1 \
    poppler-utils \
    && rm -rf /var/lib/apt/lists/* \
    && addgroup --system ocr && adduser --system --ingroup ocr ocr

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .

RUN chown -R ocr:ocr /app
USER ocr

EXPOSE 8000

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8000"]

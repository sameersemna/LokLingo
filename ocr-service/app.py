from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
import base64
import io

app = FastAPI(title="LokLingo OCR Service")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


class OCRRequest(BaseModel):
    # Base64-encoded image data
    image_b64: str
    mime_type: str = "image/png"


class OCRResponse(BaseModel):
    text: str
    confidence: float


@app.get("/health")
def health():
    return {"status": "ok", "service": "loklingo-ocr"}


@app.post("/ocr", response_model=OCRResponse)
def run_ocr(req: OCRRequest):
    try:
        image_bytes = base64.b64decode(req.image_b64)
    except Exception:
        raise HTTPException(status_code=400, detail="Invalid base64 image data")

    # PaddleOCR integration
    from paddleocr import PaddleOCR  # noqa: PLC0415

    ocr = PaddleOCR(use_angle_cls=True, lang="en", show_log=False)
    image_file = io.BytesIO(image_bytes)

    result = ocr.ocr(image_file.read(), cls=True)

    lines = []
    total_confidence = 0.0
    count = 0

    for page in result:
        if page is None:
            continue
        for line in page:
            text, confidence = line[1]
            lines.append(text)
            total_confidence += confidence
            count += 1

    extracted = "\n".join(lines)
    avg_confidence = total_confidence / count if count > 0 else 0.0

    return OCRResponse(text=extracted, confidence=round(avg_confidence, 4))

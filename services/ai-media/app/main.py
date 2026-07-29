"""Local media feature extraction service.

Runs on the user's machine so raw audio, images, and video never need to leave
the device. Only derived features (prosody numbers, OCR text) go to the backend.
"""

from fastapi import FastAPI
from pydantic import BaseModel

from .prosody import extract_prosody
from .ocr import extract_text

app = FastAPI(title="ditalk ai-media", version="0.1.0")


class ProsodyRequest(BaseModel):
    audio_path: str


class ProsodyResponse(BaseModel):
    duration_seconds: float
    tempo: float
    mean_energy: float
    pause_count: int
    pause_ratio: float


class OCRRequest(BaseModel):
    image_path: str


class OCRResponse(BaseModel):
    text: str
    confidence: float


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/prosody", response_model=ProsodyResponse)
def prosody(req: ProsodyRequest) -> ProsodyResponse:
    return ProsodyResponse(**extract_prosody(req.audio_path))


@app.post("/ocr", response_model=OCRResponse)
def ocr(req: OCRRequest) -> OCRResponse:
    return OCRResponse(**extract_text(req.image_path))

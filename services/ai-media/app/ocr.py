"""Local OCR for screenshots, memes, and visual documents (doc 9.1).

Install the optional `ocr` extra to enable real extraction.
"""


def extract_text(image_path: str) -> dict[str, float | str]:
    from paddleocr import PaddleOCR

    engine = PaddleOCR(use_angle_cls=True, lang="id", show_log=False)
    result = engine.ocr(image_path, cls=True)

    lines: list[str] = []
    scores: list[float] = []
    for page in result or []:
        for _box, (text, score) in page or []:
            lines.append(text)
            scores.append(float(score))

    return {
        "text": "\n".join(lines),
        "confidence": sum(scores) / len(scores) if scores else 0.0,
    }

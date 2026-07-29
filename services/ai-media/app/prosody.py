"""Acoustic feature extraction for voice notes (doc 10.2).

Install the optional `audio` extra to enable real extraction.
"""


def extract_prosody(audio_path: str) -> dict[str, float | int]:
    import librosa
    import numpy as np

    y, sr = librosa.load(audio_path, sr=16000, mono=True)
    duration = float(len(y) / sr)

    tempo, _ = librosa.beat.beat_track(y=y, sr=sr)
    rms = librosa.feature.rms(y=y)[0]

    intervals = librosa.effects.split(y, top_db=30)
    speech_samples = int(sum(end - start for start, end in intervals))
    pause_ratio = 1.0 - (speech_samples / len(y)) if len(y) else 0.0

    return {
        "duration_seconds": duration,
        "tempo": float(np.atleast_1d(tempo)[0]),
        "mean_energy": float(rms.mean()) if rms.size else 0.0,
        "pause_count": max(len(intervals) - 1, 0),
        "pause_ratio": float(pause_ratio),
    }

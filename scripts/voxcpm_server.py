#!/usr/bin/env python3
"""Minimal VoxCPM HTTP server for Reading Assistant.

POST /tts       JSON {text, control?, ...} -> audio/wav
POST /tts/book  JSON {sentences[], control?, ...} -> {segments: [base64 wav...]}
GET  /health    -> {ok: bool}
"""
from __future__ import annotations

import base64
import io
import json
import os
import re
import sys
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import numpy as np
import soundfile as sf

MODEL = None
MODEL_LOCK = threading.Lock()
SAMPLE_RATE = 48000


def styled_text(text: str, control: str) -> str:
    control = re.sub(r"[()（）]", "", control).strip()
    return f"({control}){text}" if control else text


def load_model() -> None:
    global MODEL, SAMPLE_RATE
    from voxcpm import VoxCPM

    print("Loading VoxCPM2…", flush=True)
    MODEL = VoxCPM.from_pretrained("openbmb/VoxCPM2", load_denoiser=False)
    SAMPLE_RATE = int(MODEL.tts_model.sample_rate)
    print(f"VoxCPM ready (sample_rate={SAMPLE_RATE})", flush=True)


def normalize_peak_rms(wav: np.ndarray) -> np.ndarray:
    wav = np.asarray(wav, dtype=np.float32).flatten()
    rms = float(np.sqrt(np.mean(wav**2))) if wav.size else 0.0
    target_rms = 0.08
    max_peak = 0.95
    if rms > 1e-6:
        wav = wav * (target_rms / rms)
        peak = float(np.max(np.abs(wav)))
        if peak > max_peak:
            wav = wav * (max_peak / peak)
    return wav


def generate_array(body: dict) -> np.ndarray:
    text = str(body.get("text") or "").strip()
    if not text:
        raise ValueError("text required")

    cfg = float(body.get("cfg_value") or 2.0)
    steps = int(body.get("inference_timesteps") or 10)
    ref = str(body.get("reference_wav_path") or "").strip().replace("/", os.sep)

    gen_kw: dict = {
        "cfg_value": cfg,
        "inference_timesteps": steps,
    }

    prompt_wav = str(body.get("prompt_wav_path") or "").strip()
    prompt_txt = str(body.get("prompt_text") or "").strip()
    if prompt_wav and os.path.isfile(prompt_wav) and prompt_txt:
        gen_kw["prompt_wav_path"] = prompt_wav
        gen_kw["prompt_text"] = prompt_txt

    if ref and os.path.isfile(ref):
        gen_kw["text"] = text
        gen_kw["reference_wav_path"] = ref
        gen_kw["normalize"] = bool(body.get("normalize", True))
    else:
        control = str(body.get("control") or "").strip()
        gen_kw["text"] = styled_text(text, control)
        gen_kw["normalize"] = bool(body.get("normalize", False))

    with MODEL_LOCK:
        wav = MODEL.generate(**gen_kw)

    wav = normalize_peak_rms(np.asarray(wav, dtype=np.float32))
    sys.stderr.write(f"tts chars={len(text)} samples={wav.size}\n")
    return wav


def wav_bytes(wav: np.ndarray) -> bytes:
    buf = io.BytesIO()
    sf.write(buf, wav, SAMPLE_RATE, format="WAV")
    return buf.getvalue()


def synthesize(body: dict) -> bytes:
    return wav_bytes(generate_array(body))


def synthesize_book(body: dict) -> list[str]:
    """One VoxCPM call per sentence — avoids chunk split bleed on sentences 2–3."""
    sentences = body.get("sentences")
    if not isinstance(sentences, list) or not sentences:
        raise ValueError("sentences required")

    control = str(body.get("control") or "").strip()
    cfg = float(body.get("cfg_value") or 2.0)
    steps = int(body.get("inference_timesteps") or 10)

    ref_path: str | None = None
    all_parts: list[np.ndarray] = []

    for i, raw in enumerate(sentences):
        text = str(raw).strip()
        if not text:
            raise ValueError(f"empty sentence at index {i}")
        if text[-1] not in ".!?":
            text += "."

        gen_body: dict = {
            "text": text,
            "control": control,
            "cfg_value": cfg,
            "inference_timesteps": steps,
            "normalize": False,
        }
        if ref_path is not None:
            gen_body["reference_wav_path"] = ref_path

        wav = generate_array(gen_body)
        all_parts.append(wav)

        sys.stderr.write(
            f"tts book sentence {i + 1}/{len(sentences)} "
            f"chars={len(text)} samples={wav.size}\n"
        )

        if i == 0:
            tmp = tempfile.NamedTemporaryFile(delete=False, suffix=".wav")
            sf.write(tmp, wav, SAMPLE_RATE, format="WAV")
            tmp.close()
            ref_path = tmp.name

    if ref_path:
        try:
            os.unlink(ref_path)
        except OSError:
            pass

    sys.stderr.write(f"tts book total sentences={len(sentences)}\n")
    return [base64.b64encode(wav_bytes(p)).decode("ascii") for p in all_parts]


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt: str, *args) -> None:
        sys.stderr.write("%s - %s\n" % (self.address_string(), fmt % args))

    def _json(self, code: int, payload: dict) -> None:
        raw = json.dumps(payload).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self) -> None:
        if self.path.rstrip("/") == "/health":
            ready = MODEL is not None
            self._json(200, {
                "ok": ready,
                "loading": not ready,
                "sample_rate": SAMPLE_RATE,
            })
            return
        self._json(404, {"error": "not found"})

    def do_POST(self) -> None:
        path = self.path.rstrip("/")
        if path not in ("/tts", "/tts/book"):
            self._json(404, {"error": "not found"})
            return
        if MODEL is None:
            self._json(503, {"error": "model loading"})
            return

        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length) if length else b"{}"
        try:
            body = json.loads(raw.decode("utf-8"))
        except json.JSONDecodeError:
            self._json(400, {"error": "invalid json"})
            return

        try:
            if path == "/tts/book":
                segments = synthesize_book(body)
                self._json(200, {"segments": segments, "sample_rate": SAMPLE_RATE})
                return
            wav = synthesize(body)
        except ValueError as e:
            self._json(400, {"error": str(e)})
            return
        except Exception as e:
            self._json(500, {"error": str(e)})
            return

        self.send_response(200)
        self.send_header("Content-Type", "audio/wav")
        self.send_header("Content-Length", str(len(wav)))
        self.end_headers()
        self.wfile.write(wav)


def main() -> None:
    import argparse

    parser = argparse.ArgumentParser(description="VoxCPM sidecar for Reading Assistant")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8808)
    args = parser.parse_args()

    threading.Thread(target=load_model, daemon=True).start()

    class ReuseAddrServer(ThreadingHTTPServer):
        allow_reuse_address = False

    server = ReuseAddrServer((args.host, args.port), Handler)
    print(f"VoxCPM sidecar on http://{args.host}:{args.port}", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nStopped.", flush=True)


if __name__ == "__main__":
    main()

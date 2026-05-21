"""
Equivalent of classifyImageKeywordsWithOllama + parseKeywordJSONArray
from internal/handler/image_handler.go.

Requires: stdlib only. Reads LOCALAI_BASE_URL from the environment (default
http://localhost:11434), same as the Go server.
"""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.request
from typing import Any


def _parse_keyword_json_array(raw: str) -> list[str]:
    s = raw.strip()
    i = s.find("[")
    if i >= 0:
        j = s.rfind("]")
        if j > i:
            s = s[i : j + 1]
    arr = json.loads(s)
    if not isinstance(arr, list):
        raise ValueError("JSON root is not an array")
    out: list[str] = []
    seen: set[str] = set()
    for kw in arr:
        if not isinstance(kw, str):
            raise TypeError("keyword array must contain only strings")
        k = kw.strip()
        if not k:
            continue
        key = k.lower()
        if key in seen:
            continue
        seen.add(key)
        out.append(k)
    if not out:
        raise ValueError("empty keyword array")
    return out


def classify_image_keywords_with_ollama(
    encoded_image_b64: str,
    *,
    base_url: str | None = None,
    timeout_sec: float | None = 120.0,
) -> str:
    """
    Call Ollama /api/chat with gemma4:latest and return a comma-separated keyword string.

    encoded_image_b64: raw base64 (no data: URL prefix), same as the Go handler.
    base_url: if None, uses env LOCALAI_BASE_URL or http://localhost:11434.
    """
    if base_url is None:
        base_url = os.environ.get("LOCALAI_BASE_URL", "").strip().rstrip("/")
    if not base_url:
        base_url = "http://localhost:11434"
    else:
        base_url = base_url.rstrip("/")

    req_body: dict[str, Any] = {
        "model": "gemma4:latest",
        "stream": False,
        "messages": [
            {
                "role": "user",
                "content": (
                    "Analyze this image and return only a JSON array of short keyword strings. "
                    "Capture content, atmosphere, vibe, and location. "
                    "Use 8 to 16 concise keywords. Do not include any text outside the JSON array."
                ),
                "images": [encoded_image_b64],
            }
        ],
        "options": {"temperature": 0.2},
    }
    payload = json.dumps(req_body).encode("utf-8")
    url = f"{base_url}/api/chat"
    req = urllib.request.Request(
        url,
        data=payload,
        method="POST",
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout_sec) as resp:
            if resp.status != 200:
                raise RuntimeError(f"ollama API status {resp.status}")
            body = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"ollama API status {e.code}") from e
    except urllib.error.URLError as e:
        raise RuntimeError(f"call ollama: {e}") from e

    try:
        content = body["message"]["content"]
    except (KeyError, TypeError) as e:
        raise RuntimeError("decode ollama response: missing message.content") from e
    if not isinstance(content, str):
        raise RuntimeError("decode ollama response: content is not a string")

    try:
        keywords = _parse_keyword_json_array(content)
    except (json.JSONDecodeError, ValueError) as e:
        raise RuntimeError(f"parse keyword array: {e}") from e

    return ", ".join(keywords)

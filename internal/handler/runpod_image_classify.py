#!/usr/bin/env python3
"""
Read a JPEG file, base64-encode it, and invoke a RunPod serverless endpoint
with input key imageClassifyRequest (see src/utils.py and src/engine.py).

Usage:
  set RUNPOD_API_KEY in the environment (or pass --api-key)
  python scripts/runpod_image_classify.py --endpoint-id YOUR_ID path/to/image.jpg

Optional:
  python ... --url https://api.runpod.ai/v2/YOUR_ID/runsync   # full run URL
  python ... --full-response ...   # print raw RunPod JSON (default: keyword JSON array only)
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import sys
import urllib.error
import urllib.request


def read_jpeg_b64(path: str) -> str:
    with open(path, "rb") as f:
        data = f.read()
    if len(data) >= 2 and data[:2] != b"\xff\xd8":
        print("Warning: file does not start with JPEG SOI marker; sending anyway.", file=sys.stderr)
    return base64.b64encode(data).decode("ascii")


def build_payload(b64: str) -> bytes:
    body = {"input": {"imageClassifyRequest": b64}}
    return json.dumps(body).encode("utf-8")


def post_json(url: str, api_key: str, payload: bytes) -> dict:
    req = urllib.request.Request(
        url,
        data=payload,
        method="POST",
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=None) as resp:
            raw = resp.read().decode("utf-8")
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", errors="replace")
        raise SystemExit(f"HTTP {e.code}: {err_body}") from e
    return json.loads(raw) if raw else {}


def extract_keyword_array(result: dict) -> list:
    """RunPod runsync returns output as a list of Ollama-style dicts; parse message.content JSON array."""
    if not isinstance(result, dict):
        raise ValueError("Response is not a JSON object")

    err = result.get("error")
    if err:
        raise ValueError(str(err))

    out = result.get("output")
    if out is None:
        raise ValueError("No 'output' in response")

    if isinstance(out, dict) and out.get("error"):
        raise ValueError(str(out["error"]))

    if not isinstance(out, list) or not out:
        raise ValueError("Unexpected or empty 'output'")

    content = None
    for item in reversed(out):
        if not isinstance(item, dict):
            continue
        if item.get("error"):
            raise ValueError(str(item["error"]))
        msg = item.get("message")
        if isinstance(msg, dict):
            c = msg.get("content")
            if isinstance(c, str) and c.strip():
                content = c.strip()
                break

    if not content:
        raise ValueError("No assistant message.content found in output")

    try:
        words = json.loads(content)
    except json.JSONDecodeError as e:
        raise ValueError(f"content is not valid JSON: {e}") from e

    if not isinstance(words, list):
        raise ValueError("message.content is not a JSON array")

    return words


def main() -> None:
    p = argparse.ArgumentParser(description="Send a JPEG to RunPod as imageClassifyRequest")
    p.add_argument("image", help="Path to a .jpg / .jpeg file")
    p.add_argument(
        "--endpoint-id",
        help="RunPod serverless endpoint ID (builds .../v2/{id}/runsync)",
    )
    p.add_argument(
        "--url",
        help="Full Run URL (overrides --endpoint-id), e.g. https://api.runpod.ai/v2/xxx/runsync",
    )
    p.add_argument(
        "--async-run",
        action="store_true",
        help="Use /run instead of /runsync (returns job id; poll status separately)",
    )
    p.add_argument(
        "--api-key",
        default=os.environ.get("RUNPOD_API_KEY", ""),
        help="RunPod API key (default: env RUNPOD_API_KEY)",
    )
    p.add_argument(
        "--full-response",
        action="store_true",
        help="Print the full RunPod JSON instead of only the keyword array",
    )
    args = p.parse_args()

    if not args.api_key.strip():
        sys.exit("Missing API key: set RUNPOD_API_KEY or pass --api-key")

    if args.url:
        url = args.url.rstrip("/")
        if not url.endswith("/run") and not url.endswith("/runsync"):
            url = url + ("/run" if args.async_run else "/runsync")
    elif args.endpoint_id:
        suffix = "run" if args.async_run else "runsync"
        url = f"https://api.runpod.ai/v2/{args.endpoint_id.strip()}/{suffix}"
    else:
        sys.exit("Provide --endpoint-id or --url")

    b64 = read_jpeg_b64(args.image)
    payload = build_payload(b64)

    result = post_json(url, args.api_key.strip(), payload)
    if args.full_response:
        print(json.dumps(result, indent=2))
        return
    try:
        words = extract_keyword_array(result)
    except ValueError as e:
        sys.exit(str(e))
    print(json.dumps(words, indent=2))


if __name__ == "__main__":
    main()

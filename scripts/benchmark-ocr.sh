#!/usr/bin/env sh
set -eu

usage() {
  cat <<'EOF'
Usage:
    benchmark-ocr.sh --manifest <manifest.jsonl|manifest.json> [--base-url URL] [--output report.json]
                                     [--model MODEL] [--mode MODE]

Manifest format:
  JSON array or JSONL with one object per document.
  Required field:
    file: absolute or relative path to the PDF/image file
  Optional fields:
    lang: OCR language hint (default: auto)
    dpi: PDF rasterization DPI (default: 200)
    text: expected full text for accuracy comparison
    blocks: expected block list for block-detection scoring

Each expected block may contain:
  - text: expected block text (optional)
  - bbox: [x1, y1, x2, y2] expected block bbox (optional)
EOF
}

base_url="${OCR_URL:-http://localhost:8000}"
manifest=""
output=""
model="${OCR_CORRECTION_MODEL:-Keyvan/german-ocr}"
mode="${OCR_MODE:-layout}"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --base-url)
      base_url="${2:-}"
      shift 2
      ;;
    --manifest)
      manifest="${2:-}"
      shift 2
      ;;
    --output)
      output="${2:-}"
      shift 2
      ;;
        --model)
            model="${2:-}"
            shift 2
            ;;
        --mode)
            mode="${2:-}"
            shift 2
            ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [ -z "$manifest" ]; then
  usage
  exit 1
fi

python3 - "$base_url" "$manifest" "$output" "$model" "$mode" <<'PYEOF'
from __future__ import annotations

import base64
import difflib
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

base_url = sys.argv[1].rstrip('/')
manifest_path = Path(sys.argv[2])
output_path = Path(sys.argv[3]) if sys.argv[3] else None
correction_model = sys.argv[4]
ocr_mode = sys.argv[5]


def read_manifest(path: Path) -> list[dict[str, Any]]:
    raw = path.read_text(encoding='utf-8').strip()
    if not raw:
        return []
    if raw.startswith('['):
        data = json.loads(raw)
        if not isinstance(data, list):
            raise SystemExit('manifest JSON must be an array')
        return [item for item in data if isinstance(item, dict)]
    rows: list[dict[str, Any]] = []
    for line in raw.splitlines():
        line = line.strip()
        if not line:
            continue
        item = json.loads(line)
        if isinstance(item, dict):
            rows.append(item)
    return rows


def normalize_text(text: str) -> str:
    return ' '.join(text.lower().split())


def text_accuracy(expected: str | None, actual: str) -> float | None:
    if not expected:
        return None
    return round(difflib.SequenceMatcher(None, normalize_text(expected), normalize_text(actual)).ratio(), 4)


def bbox_iou(a: list[float], b: list[float]) -> float:
    ax1, ay1, ax2, ay2 = a
    bx1, by1, bx2, by2 = b
    inter_x1 = max(ax1, bx1)
    inter_y1 = max(ay1, by1)
    inter_x2 = min(ax2, bx2)
    inter_y2 = min(ay2, by2)
    inter_w = max(0.0, inter_x2 - inter_x1)
    inter_h = max(0.0, inter_y2 - inter_y1)
    inter_area = inter_w * inter_h
    if inter_area == 0:
        return 0.0
    area_a = max(0.0, ax2 - ax1) * max(0.0, ay2 - ay1)
    area_b = max(0.0, bx2 - bx1) * max(0.0, by2 - by1)
    denom = area_a + area_b - inter_area
    return inter_area / denom if denom > 0 else 0.0


def block_detection_score(expected_blocks: list[dict[str, Any]] | None, actual_blocks: list[dict[str, Any]]) -> dict[str, Any]:
    if not expected_blocks:
        return {
            'expected_count': None,
            'detected_count': len(actual_blocks),
            'precision': None,
            'recall': None,
            'f1': None,
            'mean_iou': None,
        }

    expected = [block for block in expected_blocks if isinstance(block, dict)]
    actual = [block for block in actual_blocks if isinstance(block, dict)]
    matches: list[tuple[int, int, float]] = []
    used_actual: set[int] = set()

    for expected_index, expected_block in enumerate(expected):
        expected_bbox = expected_block.get('bbox')
        if not (isinstance(expected_bbox, list) and len(expected_bbox) == 4):
            continue
        best = (-1, 0.0)
        for actual_index, actual_block in enumerate(actual):
            if actual_index in used_actual:
                continue
            actual_bbox = actual_block.get('bbox')
            if not (isinstance(actual_bbox, list) and len(actual_bbox) == 4):
                continue
            score = bbox_iou([float(v) for v in expected_bbox], [float(v) for v in actual_bbox])
            if score > best[1]:
                best = (actual_index, score)
        if best[0] >= 0 and best[1] >= 0.5:
            used_actual.add(best[0])
            matches.append((expected_index, best[0], best[1]))

    tp = len(matches)
    precision = tp / len(actual) if actual else 0.0
    recall = tp / len(expected) if expected else 0.0
    f1 = (2 * precision * recall / (precision + recall)) if (precision + recall) else 0.0
    mean_iou = sum(match[2] for match in matches) / len(matches) if matches else 0.0
    return {
        'expected_count': len(expected),
        'detected_count': len(actual),
        'precision': round(precision, 4),
        'recall': round(recall, 4),
        'f1': round(f1, 4),
        'mean_iou': round(mean_iou, 4),
    }


def request_ocr(path: Path, lang: str, dpi: int, *, correction_enabled: bool, model: str, mode: str) -> tuple[dict[str, Any], float]:
    suffix = path.suffix.lower()
    if suffix == '.pdf':
        endpoint = f'{base_url}/api/v1/ocr/pdf'
        payload = {
            'pdf_b64': base64.b64encode(path.read_bytes()).decode('ascii'),
            'lang': lang,
            'dpi': dpi,
            'mode': mode,
            'ocr_correction_enabled': correction_enabled,
            'ocr_correction_model': model,
            'visual_diff_mode': True,
        }
    else:
        endpoint = f'{base_url}/api/v1/ocr'
        payload = {
            'image_b64': base64.b64encode(path.read_bytes()).decode('ascii'),
            'mime_type': 'image/png' if suffix == '.png' else 'image/jpeg',
            'lang': lang,
            'mode': mode,
            'ocr_correction_enabled': correction_enabled,
            'ocr_correction_model': model,
            'visual_diff_mode': True,
        }

    request = urllib.request.Request(
        endpoint,
        data=json.dumps(payload).encode('utf-8'),
        headers={'Content-Type': 'application/json'},
        method='POST',
    )
    start = time.perf_counter()
    try:
        with urllib.request.urlopen(request, timeout=600) as response:
            body = response.read().decode('utf-8')
    except urllib.error.HTTPError as exc:
        raise SystemExit(f'OCR request failed for {path}: HTTP {exc.code} {exc.reason}') from exc
    elapsed_ms = round((time.perf_counter() - start) * 1000.0, 2)
    return json.loads(body), elapsed_ms


rows = read_manifest(manifest_path)
if not rows:
    raise SystemExit(f'No manifest rows found in {manifest_path}')

results: list[dict[str, Any]] = []
for entry in rows:
    file_path = Path(entry['file'])
    lang = str(entry.get('lang', 'auto'))
    dpi = int(entry.get('dpi', 200))
    expected_text = entry.get('text')
    expected_blocks = entry.get('blocks')

    raw_payload, raw_latency_ms = request_ocr(file_path, lang, dpi, correction_enabled=False, model=correction_model, mode=ocr_mode)
    corrected_payload, corrected_latency_ms = request_ocr(file_path, lang, dpi, correction_enabled=True, model=correction_model, mode=ocr_mode)

    raw_text = raw_payload.get('text', '')
    corrected_text = corrected_payload.get('text', '')
    raw_confidence = float(raw_payload.get('confidence', 0.0))
    corrected_confidence = float(corrected_payload.get('confidence', 0.0))
    raw_blocks = raw_payload.get('blocks')
    corrected_blocks = corrected_payload.get('blocks')
    if not isinstance(raw_blocks, list):
        raw_blocks = []
    if not isinstance(corrected_blocks, list):
        corrected_blocks = []
    if not raw_blocks and isinstance(raw_payload.get('pages'), list):
        for page in raw_payload['pages']:
            if isinstance(page, dict):
                blocks = page.get('blocks')
                if isinstance(blocks, list):
                    raw_blocks.extend(blocks)
    if not corrected_blocks and isinstance(corrected_payload.get('pages'), list):
        for page in corrected_payload['pages']:
            if isinstance(page, dict):
                blocks = page.get('blocks')
                if isinstance(blocks, list):
                    corrected_blocks.extend(blocks)

    raw_accuracy = text_accuracy(expected_text, raw_text)
    corrected_accuracy = text_accuracy(expected_text, corrected_text)

    results.append({
        'file': str(file_path),
        'lang': lang,
        'dpi': dpi,
        'raw': {
            'latency_ms': raw_latency_ms,
            'confidence': round(raw_confidence, 4),
            'text_accuracy': raw_accuracy,
            'block_detection': block_detection_score(expected_blocks, raw_blocks),
            'detected_blocks': len(raw_blocks),
        },
        'corrected': {
            'latency_ms': corrected_latency_ms,
            'confidence': round(corrected_confidence, 4),
            'text_accuracy': corrected_accuracy,
            'block_detection': block_detection_score(expected_blocks, corrected_blocks),
            'detected_blocks': len(corrected_blocks),
        },
        'delta': {
            'latency_ms': round(corrected_latency_ms - raw_latency_ms, 2),
            'confidence': round(corrected_confidence - raw_confidence, 4),
            'text_accuracy': round((corrected_accuracy or 0.0) - (raw_accuracy or 0.0), 4) if (raw_accuracy is not None and corrected_accuracy is not None) else None,
            'changed_characters': int(corrected_payload.get('correction', {}).get('changed_characters', 0)) if isinstance(corrected_payload.get('correction'), dict) else 0,
        },
    })

count = len(results)
raw_latency = round(sum(item['raw']['latency_ms'] for item in results) / count, 2)
corrected_latency = round(sum(item['corrected']['latency_ms'] for item in results) / count, 2)
raw_confidence = round(sum(item['raw']['confidence'] for item in results) / count, 4)
corrected_confidence = round(sum(item['corrected']['confidence'] for item in results) / count, 4)
raw_text_scores = [item['raw']['text_accuracy'] for item in results if item['raw']['text_accuracy'] is not None]
corrected_text_scores = [item['corrected']['text_accuracy'] for item in results if item['corrected']['text_accuracy'] is not None]
raw_text_accuracy = round(sum(raw_text_scores) / len(raw_text_scores), 4) if raw_text_scores else None
corrected_text_accuracy = round(sum(corrected_text_scores) / len(corrected_text_scores), 4) if corrected_text_scores else None
raw_block_f1_scores = [item['raw']['block_detection']['f1'] for item in results if item['raw']['block_detection']['f1'] is not None]
corrected_block_f1_scores = [item['corrected']['block_detection']['f1'] for item in results if item['corrected']['block_detection']['f1'] is not None]
raw_block_f1 = round(sum(raw_block_f1_scores) / len(raw_block_f1_scores), 4) if raw_block_f1_scores else None
corrected_block_f1 = round(sum(corrected_block_f1_scores) / len(corrected_block_f1_scores), 4) if corrected_block_f1_scores else None
avg_delta_latency = round(corrected_latency - raw_latency, 2)
avg_delta_confidence = round(corrected_confidence - raw_confidence, 4)
avg_delta_text_accuracy = round((corrected_text_accuracy or 0.0) - (raw_text_accuracy or 0.0), 4) if (raw_text_accuracy is not None and corrected_text_accuracy is not None) else None

report = {
    'base_url': base_url,
    'manifest': str(manifest_path),
    'results': results,
    'correction_model': correction_model,
    'mode': ocr_mode,
    'summary': {
        'documents': count,
        'raw_avg_latency_ms': raw_latency,
        'corrected_avg_latency_ms': corrected_latency,
        'delta_latency_ms': avg_delta_latency,
        'raw_avg_confidence': raw_confidence,
        'corrected_avg_confidence': corrected_confidence,
        'delta_confidence': avg_delta_confidence,
        'raw_avg_text_accuracy': raw_text_accuracy,
        'corrected_avg_text_accuracy': corrected_text_accuracy,
        'delta_text_accuracy': avg_delta_text_accuracy,
        'raw_avg_block_f1': raw_block_f1,
        'corrected_avg_block_f1': corrected_block_f1,
    },
}

print(f"Benchmark completed for {count} document(s)")
print(f"Model: {correction_model}  mode: {ocr_mode}")
print(f"Raw avg latency: {raw_latency} ms")
print(f"Corrected avg latency: {corrected_latency} ms  (delta: {avg_delta_latency} ms)")
print(f"Raw avg confidence: {raw_confidence}")
print(f"Corrected avg confidence: {corrected_confidence}  (delta: {avg_delta_confidence})")
if raw_text_accuracy is not None:
    print(f"Raw avg text accuracy: {raw_text_accuracy}")
if corrected_text_accuracy is not None:
    print(f"Corrected avg text accuracy: {corrected_text_accuracy}")
if avg_delta_text_accuracy is not None:
    print(f"Text accuracy delta: {avg_delta_text_accuracy}")
if raw_block_f1 is not None:
    print(f"Raw avg block F1: {raw_block_f1}")
if corrected_block_f1 is not None:
    print(f"Corrected avg block F1: {corrected_block_f1}")
print('')
for item in results:
    raw = item['raw']
    corrected = item['corrected']
    delta = item['delta']
    raw_acc = raw['text_accuracy'] if raw['text_accuracy'] is not None else 'n/a'
    corrected_acc = corrected['text_accuracy'] if corrected['text_accuracy'] is not None else 'n/a'
    raw_f1 = raw['block_detection']['f1'] if raw['block_detection']['f1'] is not None else 'n/a'
    corrected_f1 = corrected['block_detection']['f1'] if corrected['block_detection']['f1'] is not None else 'n/a'
    print(
        f"{Path(item['file']).name}: raw(lat={raw['latency_ms']}ms conf={raw['confidence']} acc={raw_acc} f1={raw_f1} blocks={raw['detected_blocks']}) "
        f"corrected(lat={corrected['latency_ms']}ms conf={corrected['confidence']} acc={corrected_acc} f1={corrected_f1} blocks={corrected['detected_blocks']}) "
        f"delta(lat={delta['latency_ms']}ms conf={delta['confidence']} acc={delta['text_accuracy']} changed_chars={delta['changed_characters']})"
    )

if output_path:
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(f'')
    print(f'Wrote report to {output_path}')
PYEOF

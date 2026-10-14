"""Decode QR codes from images with zxing-cpp."""

from __future__ import annotations

import io
from dataclasses import dataclass

import zxingcpp
from PIL import Image, ImageOps, UnidentifiedImageError

MAX_PIXELS = 25_000_000  # guard against decompression bombs
MAX_SIDE = 2000  # downscale large photos; QR modules stay readable


class DecodeError(ValueError):
    """Raised when the upload is not a readable image."""


@dataclass(frozen=True)
class Code:
    format: str
    text: str


def _load(data: bytes) -> Image.Image:
    Image.MAX_IMAGE_PIXELS = MAX_PIXELS
    try:
        img = Image.open(io.BytesIO(data))
        img.load()
    except (UnidentifiedImageError, OSError, Image.DecompressionBombError) as exc:
        raise DecodeError("not a supported image") from exc
    img = ImageOps.exif_transpose(img)
    if max(img.size) > MAX_SIDE:
        img.thumbnail((MAX_SIDE, MAX_SIDE))
    return img.convert("L")


def decode_image(data: bytes) -> list[Code]:
    """Return every QR code found in the image, de-duplicated, in reading order."""
    img = _load(data)
    results = zxingcpp.read_barcodes(img, formats=zxingcpp.BarcodeFormat.QRCode)
    if not results:
        # Screenshots in dark mode often have inverted (light-on-dark) codes.
        results = zxingcpp.read_barcodes(
            ImageOps.invert(img), formats=zxingcpp.BarcodeFormat.QRCode
        )
    seen: set[str] = set()
    codes: list[Code] = []
    for r in results:
        if not r.valid or r.text in seen:
            continue
        seen.add(r.text)
        codes.append(Code(format=r.format.name, text=r.text))
    return codes

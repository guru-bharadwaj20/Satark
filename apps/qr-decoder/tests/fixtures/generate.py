"""Regenerate the sample QR images used by the tests.

Run from apps/qr-decoder:  python tests/fixtures/generate.py

All payloads are synthetic: the PSP handle "examplebank" does not exist and
the UPI IDs do not belong to anyone.
"""

from pathlib import Path

import qrcode
from PIL import Image, ImageOps

HERE = Path(__file__).parent

SAMPLES = {
    "upi_merchant.png": "upi://pay?pa=teashop01@examplebank&pn=Tea%20Stall&cu=INR",
    "upi_receive_scam.png": (
        "upi://pay?pa=xyz123@examplebank&pn=Refund%20Desk&am=14999&cu=INR"
        "&tn=Scan%20to%20receive%20your%20refund"
    ),
    "upi_bad_payee.png": "upi://pay?pa=not-a-vpa&am=10&cu=USD",
    "url.png": "https://example.org/offer",
    "plain_text.png": "Table 4 - Wi-Fi: guest",
}


def make(payload: str) -> Image.Image:
    qr = qrcode.QRCode(border=4, box_size=8)
    qr.add_data(payload)
    qr.make(fit=True)
    return qr.make_image(fill_color="black", back_color="white").get_image().convert("L")


def main() -> None:
    for name, payload in SAMPLES.items():
        make(payload).save(HERE / name)
    # Dark-mode screenshot: light modules on a dark background.
    ImageOps.invert(make(SAMPLES["upi_merchant.png"])).save(HERE / "upi_inverted.png")
    # Photo-like: the code rotated and placed on a larger grey canvas.
    canvas = Image.new("L", (900, 900), 170)
    canvas.paste(
        make(SAMPLES["upi_receive_scam.png"]).rotate(12, expand=True, fillcolor=255), (150, 120)
    )
    canvas.save(HERE / "upi_receive_photo.jpg", quality=85)
    Image.new("L", (300, 300), 255).save(HERE / "blank.png")


if __name__ == "__main__":
    main()

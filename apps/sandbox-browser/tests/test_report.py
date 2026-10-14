import json

from app.report import Forms, ScanReport, encode


def test_encode_matches_operator_contract():
    r = ScanReport(
        final_url="https://b.example/",
        redirect_chain=["https://a.example/", "https://b.example/"],
        forms=Forms(otp=True),
        downloads=["app.apk"],
        screenshot_key="k.png",
    )
    data = json.loads(encode(r))
    assert data["final_url"] == "https://b.example/"
    assert data["forms"] == {"password": False, "otp": True, "card_number": False}
    assert data["downloads"] == ["app.apk"]
    assert data["screenshot_key"] == "k.png"


def test_encode_fits_termination_limit():
    chain = [f"https://hop{i}.example/" + "x" * 400 for i in range(50)]
    r = ScanReport(final_url=chain[-1], redirect_chain=chain, title="t" * 1000)
    out = encode(r)
    assert len(out.encode()) <= 4096
    data = json.loads(out)
    assert data["redirect_chain"][0].startswith("https://hop0.example/")
    assert data["redirect_chain"][-1].startswith("https://hop49.example/")

from app.downloads import MAX_DOWNLOADS, DownloadWatcher, filename_from_disposition


def test_download_event_records_name():
    w = DownloadWatcher()
    w.on_download("PM-Kisan-KYC.apk", "https://x.example/f")
    assert w.files == ["PM-Kisan-KYC.apk"]
    assert w.has_apk


def test_apk_mime_without_extension():
    w = DownloadWatcher()
    w.on_response(
        "https://x.example/get?id=7",
        {"Content-Type": "application/vnd.android.package-archive"},
    )
    assert w.files == ["get"]


def test_attachment_disposition():
    w = DownloadWatcher()
    w.on_response(
        "https://x.example/dl",
        {"Content-Disposition": 'attachment; filename="bank-update.apk"'},
    )
    assert w.has_apk


def test_apk_url_extension():
    w = DownloadWatcher()
    w.on_response("https://cdn.example/files/rewards.apk", {"Content-Type": "text/plain"})
    assert w.files == ["rewards.apk"]


def test_html_page_is_not_a_download():
    w = DownloadWatcher()
    w.on_response("https://example.com/index.html", {"Content-Type": "text/html"})
    w.on_response("https://example.com/app.js", {"Content-Type": "application/javascript"})
    assert w.files == []


def test_dedupe_and_cap():
    w = DownloadWatcher()
    for i in range(30):
        w.on_download(f"f{i % 15}.apk", "")
    assert len(w.files) == MAX_DOWNLOADS


def test_rfc5987_filename():
    value = "attachment; filename*=UTF-8''kyc%20form.apk"
    assert filename_from_disposition(value) == "kyc form.apk"

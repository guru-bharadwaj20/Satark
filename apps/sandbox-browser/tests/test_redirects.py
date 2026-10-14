from app.redirects import MAX_HOPS, RedirectTracker


def test_http_and_client_redirects():
    t = RedirectTracker("https://bit.example/abc")
    t.on_navigation_response("https://bit.example/abc", 301)
    t.on_navigation_response("https://track.example/r?id=1", 302)
    t.on_navigation_response("https://landing.example/", 200)
    t.on_frame_navigated("https://landing.example/")
    t.on_frame_navigated("https://kyc-update.example/login")
    assert t.chain() == [
        "https://bit.example/abc",
        "https://track.example/r?id=1",
        "https://landing.example/",
        "https://kyc-update.example/login",
    ]
    assert t.hops[0].status == 301
    assert t.final_url == "https://kyc-update.example/login"
    assert t.hop_count == 3


def test_ignores_blank_and_fragment_changes():
    t = RedirectTracker("https://example.com")
    t.on_frame_navigated("about:blank")
    t.on_frame_navigated("https://example.com/")
    t.on_frame_navigated("https://example.com/#top")
    assert t.chain() == ["https://example.com"]


def test_caps_hops():
    t = RedirectTracker("https://loop.example/0")
    for i in range(1, 100):
        t.on_navigation_response(f"https://loop.example/{i}", 302)
    assert len(t.chain()) == MAX_HOPS

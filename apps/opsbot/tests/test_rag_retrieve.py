import asyncio

from rag_fakes import FakeEmbedder, FakeStore

from app.alerts import Alert
from app.rag.chunk import chunk_markdown
from app.rag.ingest import ingest
from app.rag.retrieve import Retriever, build_query, format_sources, linked_runbook

DOCS = {
    "RB-001-crashloop-imagepull": "# RB-001 CrashLoop\n## Diagnosis\ncrashloop backoff image pull "
    "events\n## Fix\nrollback the image\n## Prevention\ncrashloop alerts",
    "RB-003-postgres": "# RB-003 Postgres\n## Diagnosis\npostgres primary failover\n"
    "## Fix\npostgres",
    "RB-004-certificates": "# RB-004 Certificates\n## Diagnosis\ncertificate expiry dns",
    "RB-012-llm": "# RB-012 LLM\n## Diagnosis\nllm budget token spike",
}


def retriever(**kw):
    store, emb = FakeStore(), FakeEmbedder()
    chunks = [c for d, md in DOCS.items() for c in chunk_markdown(d, md)]
    asyncio.run(ingest(chunks, emb, store))
    return Retriever(emb, store, **kw)


def test_query_uses_alert_and_context_signals():
    a = Alert(
        status="firing",
        labels={"alertname": "KubePodCrashLooping"},
        annotations={"summary": "pod restarting"},
    )
    q = build_query(a, "state=waiting reason=CrashLoopBackOff ... OOMKilled")
    assert "Kube Pod Crash Looping" in q and "CrashLoopBackOff" in q and "OOMKilled" in q


def test_most_relevant_runbook_first_and_capped_per_doc():
    hits = asyncio.run(retriever(k=3).retrieve("crashloop backoff image pull"))
    assert hits[0].doc == "RB-001-crashloop-imagepull"
    assert sum(h.doc.startswith("RB-001") for h in hits) <= 2


def test_linked_runbook_is_boosted():
    a = Alert(
        status="firing",
        labels={"alertname": "SatarkLLMBudgetExceeded"},
        annotations={"runbook_url": "https://x/docs/runbooks/RB-012-llm.md"},
    )
    assert linked_runbook(a) == "RB-012"
    hits = asyncio.run(retriever().for_alert(a, ""))
    assert hits and hits[0].doc == "RB-012-llm"


def test_irrelevant_query_returns_nothing():
    assert asyncio.run(retriever(min_score=0.3).retrieve("unrelated words")) == []
    assert "No matching" in format_sources([])


def test_format_sources_numbers_citations():
    hits = asyncio.run(retriever().retrieve("postgres failover"))
    text = format_sources(hits)
    assert text.startswith("[1] RB-003 Postgres")

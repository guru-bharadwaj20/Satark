"""Split Markdown runbooks into heading-scoped chunks."""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from pathlib import Path

MAX_CHARS = 1500
OVERLAP = 200
_HEADING = re.compile(r"^(#{1,4})\s+(.+?)\s*#*\s*$")


@dataclass(frozen=True)
class Chunk:
    doc: str  # e.g. RB-001-crashloop-imagepull
    title: str  # document H1
    heading: str  # "Diagnosis > Check events"
    ordinal: int
    content: str

    @property
    def id(self) -> str:
        return f"{self.doc}#{self.ordinal}"

    @property
    def sha(self) -> str:
        return hashlib.sha256(f"{self.title}|{self.heading}|{self.content}".encode()).hexdigest()

    def embedding_text(self) -> str:
        return f"{self.title}\n{self.heading}\n\n{self.content}"


def _split_long(text: str) -> list[str]:
    if len(text) <= MAX_CHARS:
        return [text]
    parts, start = [], 0
    while start < len(text):
        end = min(len(text), start + MAX_CHARS)
        if end < len(text):
            cut = text.rfind("\n\n", start + MAX_CHARS // 2, end)
            end = cut if cut > 0 else end
        parts.append(text[start:end].strip())
        if end >= len(text):
            break
        start = max(end - OVERLAP, start + 1)
    return [p for p in parts if p]


def chunk_markdown(doc: str, markdown: str) -> list[Chunk]:
    title, path = doc, []
    sections: list[tuple[str, list[str]]] = []
    current: list[str] = []
    in_code = False

    def flush():
        body = "\n".join(current).strip()
        if body:
            sections.append((" > ".join(h for _, h in path) or "Overview", [body]))

    for line in markdown.splitlines():
        if line.strip().startswith("```"):
            in_code = not in_code
        m = None if in_code else _HEADING.match(line)
        if m:
            flush()
            current = []
            level, text = len(m.group(1)), m.group(2)
            if level == 1:
                title, path = text, []
                continue
            path = [(lvl, h) for lvl, h in path if lvl < level] + [(level, text)]
        else:
            current.append(line)
    flush()

    chunks, n = [], 0
    for heading, (body,) in sections:
        for piece in _split_long(body):
            chunks.append(Chunk(doc, title, heading, n, piece))
            n += 1
    return chunks


def load_runbooks(directory: str | Path) -> list[Chunk]:
    out: list[Chunk] = []
    for p in sorted(Path(directory).glob("*.md")):
        out.extend(chunk_markdown(p.stem, p.read_text(encoding="utf-8")))
    return out

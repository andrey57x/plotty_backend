"""
python3 scripts/seed_stories.py
"""
import re
import sys
import uuid
from datetime import datetime, timedelta, timezone
from pathlib import Path

from stories_data import STORIES

# UUID тегов из 001_init.up.sql
TAGS = {
    # directionality
    "harry-potter": "10000001-0001-4001-8001-000000000001",
    "witcher": "10000001-0001-4001-8001-000000000002",
    "lord-of-the-rings": "10000001-0001-4001-8001-000000000003",
    "naruto": "10000001-0001-4001-8001-000000000004",
    "marvel": "10000001-0001-4001-8001-000000000005",
    "dc": "10000001-0001-4001-8001-000000000006",
    "sherlock": "10000001-0001-4001-8001-000000000007",
    "star-wars": "10000001-0001-4001-8001-000000000008",
    "game-of-thrones": "10000001-0001-4001-8001-000000000009",
    "attack-on-titan": "10000001-0001-4001-8001-00000000000a",
    "originals": "10000001-0001-4001-8001-00000000000b",
    # genre
    "drama": "10000002-0002-4002-8002-000000000001",
    "humor": "10000002-0002-4002-8002-000000000002",
    "mystery": "10000002-0002-4002-8002-000000000003",
    "slice-of-life": "10000002-0002-4002-8002-000000000004",
    "fantasy": "10000002-0002-4002-8002-000000000005",
    "adventure": "10000002-0002-4002-8002-000000000006",
    # warning
    "character-death": "10000003-0003-4003-8003-000000000001",
    "violence": "10000003-0003-4003-8003-000000000002",
    "ooc": "10000003-0003-4003-8003-000000000003",
    "profanity": "10000003-0003-4003-8003-000000000004",
    # rating
    "g": "10000004-0004-4004-8004-000000000001",
    "pg-13": "10000004-0004-4004-8004-000000000002",
    "r": "10000004-0004-4004-8004-000000000003",
    "nc-17": "10000004-0004-4004-8004-000000000004",
    "nc-21": "10000004-0004-4004-8004-000000000005",
    # size
    "drabble": "10000005-0005-4005-8005-000000000001",
    "mini": "10000005-0005-4005-8005-000000000002",
    "midi": "10000005-0005-4005-8005-000000000003",
    "maxi": "10000005-0005-4005-8005-000000000004",
    # completion
    "completed": "10000006-0006-4006-8006-000000000001",
    "in-progress": "10000006-0006-4006-8006-000000000002",
    "frozen": "10000006-0006-4006-8006-000000000003",
}

FORBIDDEN_TAGS = {"r", "nc-17", "nc-21", "profanity"}

NS = uuid.UUID("00000000-0000-0000-0000-0000000000ab")
AUTHOR_EMAIL = "author@gmail.com"
MIGRATIONS_DIR = Path(__file__).resolve().parent.parent / "migrations"


def det_uuid(*parts: str) -> str:
    return str(uuid.uuid5(NS, "|".join(parts)))


def sqlstr(s: str) -> str:
    return "'" + s.replace("'", "''") + "'"


def word_count(text: str) -> int:
    return len(re.findall(r"[\w’'-]+", text, flags=re.UNICODE))


def collect_tags(story: dict) -> list:
    slugs = [story["fandom"], story["rating"], story["size"], story["completion"]]
    slugs += story.get("genres", [])
    slugs += story.get("warnings", [])
    seen, out = set(), []
    for s in slugs:
        if s in seen:
            continue
        seen.add(s)
        out.append(s)
    return out


def validate(stories: list):
    errors = []
    slugs = set()
    for i, st in enumerate(stories):
        tag = f"[{i}] {st.get('title','?')}"
        for key in ("fandom", "title", "slug", "summary", "rating", "size", "completion", "chapters"):
            if not st.get(key):
                errors.append(f"{tag}: пустое поле {key}")
        if st["slug"] in slugs:
            errors.append(f"{tag}: дублирующийся slug {st['slug']}")
        slugs.add(st["slug"])
        for t in collect_tags(st):
            if t not in TAGS:
                errors.append(f"{tag}: неизвестный тег {t}")
            if t in FORBIDDEN_TAGS:
                errors.append(f"{tag}: запрещённый тег {t}")
        chs = st.get("chapters", [])
        if not (3 <= len(chs) <= 7):
            errors.append(f"{tag}: глав {len(chs)} (нужно 3–7)")
        for j, ch in enumerate(chs):
            wc = word_count(ch["text"])
            if wc < 400:
                errors.append(f"{tag} гл.{j+1} '{ch.get('title','?')}': {wc} слов (<400)")
    return errors


def build():
    errs = validate(STORIES)
    if errs:
        print("Ошибки валидации:", file=sys.stderr)
        for e in errs:
            print("  - " + e, file=sys.stderr)
        sys.exit(1)

    base = datetime(2026, 5, 19, 12, 0, 0, tzinfo=timezone.utc)
    up = []
    up.append("-- 012_seed_stories.up.sql")
    up.append("-- 50 оригинальных фанфиков.")
    up.append("")
    up.append("DO $$")
    up.append("DECLARE")
    up.append("    v_author_id BIGINT;")
    up.append("BEGIN")
    up.append(f"    SELECT id INTO v_author_id FROM users WHERE email = {sqlstr(AUTHOR_EMAIL)};")
    up.append("    IF v_author_id IS NULL THEN")
    up.append(f"        RAISE EXCEPTION 'Автор % не найден — создайте пользователя перед миграцией', {sqlstr(AUTHOR_EMAIL)};")
    up.append("    END IF;")
    up.append("")

    total_words = 0
    total_chapters = 0
    for i, st in enumerate(STORIES):
        story_id = det_uuid("story", st["slug"])
        s_created = base - timedelta(days=2 * i + 30)
        s_updated = base - timedelta(days=i, hours=i % 5)
        up.append(f"    -- [{i+1}] {st['title']} ({st['fandom']})")
        up.append("    INSERT INTO stories (id, slug, title, status, author_id, ai_summary, created_at, updated_at) VALUES (")
        up.append(f"        '{story_id}', {sqlstr(st['slug'])}, {sqlstr(st['title'])}, 'published', v_author_id, {sqlstr(st['summary'])},")
        up.append(f"        '{s_created.isoformat()}', '{s_updated.isoformat()}');")
        # теги
        for t in collect_tags(st):
            up.append(
                f"    INSERT INTO story_tags (story_id, tag_id) VALUES ('{story_id}', '{TAGS[t]}');"
            )
        # главы
        for j, ch in enumerate(st["chapters"]):
            ch_id = det_uuid("chapter", st["slug"], str(j))
            c_created = s_created + timedelta(days=j)
            c_updated = c_created + timedelta(hours=1)
            text = ch["text"].strip()
            total_words += word_count(text)
            total_chapters += 1
            up.append(
                "    INSERT INTO chapters (id, story_id, title, content, draft_title, draft_content, status, created_at, updated_at) VALUES ("
            )
            up.append(f"        '{ch_id}', '{story_id}', {sqlstr(ch['title'])}, {sqlstr(text)},")
            up.append(f"        {sqlstr(ch['title'])}, {sqlstr(text)}, 'published', '{c_created.isoformat()}', '{c_updated.isoformat()}');")
        up.append("")

    up.append("END $$;")
    up.append("")

    # down
    down = ["-- 012_seed_stories.down.sql", "-- Удаляет истории.", ""]
    down.append("DELETE FROM stories WHERE slug IN (")
    slug_lines = [f"    {sqlstr(st['slug'])}" for st in STORIES]
    down.append(",\n".join(slug_lines))
    down.append(");")
    down.append("")

    (MIGRATIONS_DIR / "012_seed_stories.up.sql").write_text("\n".join(up), encoding="utf-8")
    (MIGRATIONS_DIR / "012_seed_stories.down.sql").write_text("\n".join(down), encoding="utf-8")


if __name__ == "__main__":
    build()

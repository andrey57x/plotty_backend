import os
import sys
import uuid
import logging
import argparse
import requests
import cloudscraper
from bs4 import BeautifulSoup # Тот самый пропущенный импорт
from urllib.parse import urlparse, unquote
import psycopg2

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)
logger = logging.getLogger("CanonParser")

def get_db_dsn() -> str:
    db_host = os.getenv("ML_DB_HOST", "ml-db")
    db_port = os.getenv("ML_DB_PORT", "5432") 
    db_user = os.getenv("ML_DB_USER", "ml_user")
    db_pass = os.getenv("ML_DB_PASSWORD", "ml_password")
    db_name = os.getenv("ML_DB_NAME", "plotty_ml_db")
    return f"postgres://{db_user}:{db_pass}@{db_host}:{db_port}/{db_name}?sslmode=disable"

def parse_fandom_api(url: str) -> list[str]:
    logger.info(f"Using MediaWiki API (action=parse) for URL: {url}")
    parsed = urlparse(url)
    path_parts = parsed.path.split('/wiki/')
    if len(path_parts) != 2:
        logger.error("Неверный формат URL. Ожидается: https://[domain]/[lang]/wiki/[Title]")
        sys.exit(1)
        
    base_api_url = f"{parsed.scheme}://{parsed.netloc}{path_parts[0]}/api.php"
    title = unquote(path_parts[1])

    params = {
        "action": "parse",
        "page": title,
        "prop": "text",
        "format": "json"
    }
    
    headers = {"User-Agent": "PlottyCanonParser/1.0"}
    
    try:
        response = requests.get(base_api_url, params=params, headers=headers, timeout=10)
        response.raise_for_status()
        data = response.json()
    except Exception as e:
        logger.error(f"API fetch failed: {e}")
        sys.exit(1)

    if "error" in data:
        logger.error(f"API Error: {data['error'].get('info', 'Unknown error')}")
        sys.exit(1)

    html_content = data.get("parse", {}).get("text", {}).get("*", "")
    if not html_content:
        logger.error("API did not return HTML content.")
        return []

    soup = BeautifulSoup(html_content, 'html.parser')
    paragraphs = soup.find_all('p')
    
    facts = []
    import re
    for p in paragraphs:
        text = p.get_text().strip()
        text = re.sub(r'\[\d+\]', '', text)
        
        if 50 < len(text) < 1500 and not text.startswith("=="):
            facts.append(text)
            
    return facts

def get_embedding(text: str) -> list[float]:
    embed_url = "http://127.0.0.1:8000/embed"
    try:
        res = requests.post(embed_url, json={"text": text}, timeout=10)
        res.raise_for_status()
        return res.json()["embedding"]
    except Exception as e:
        logger.error(f"Embeddings API error: {e}")
        sys.exit(1)

def save_facts_to_db(dsn: str, fandom_slug: str, entity_name: str, facts: list[str]):
    try:
        conn = psycopg2.connect(dsn)
        cur = conn.cursor()
    except Exception as e:
        logger.error(f"Database connection failed: {e}")
        sys.exit(1)

    inserted = 0
    for fact in facts:
        vec = get_embedding(fact)
        try:
            cur.execute("""
                INSERT INTO canon_lorebooks (id, fandom_slug, entity_name, fact_text, embedding)
                VALUES (%s, %s, %s, %s, %s::vector)
            """, (str(uuid.uuid4()), fandom_slug, entity_name, fact, vec))
            inserted += 1
        except Exception as e:
            logger.warning(f"Failed to insert fact: {e}")
            conn.rollback()
            continue

    conn.commit()
    cur.close()
    conn.close()
    logger.info(f"Successfully saved {inserted}/{len(facts)} facts to database.")

def main():
    parser = argparse.ArgumentParser(description="Plotty Canon Lore Parser")
    parser.add_argument("--fandom", required=True, help="Fandom slug (e.g., 'harry-potter')")
    parser.add_argument("--entity", required=True, help="Entity name (e.g., 'Severus Snape')")
    parser.add_argument("--url", required=True, help="URL to Fandom Wiki page")
    
    args = parser.parse_args()
    dsn = get_db_dsn()
    
    logger.info(f"Starting API parser for Fandom: {args.fandom} | Entity: {args.entity}")
    facts = parse_fandom_api(args.url)
    
    if not facts:
        logger.error("No facts extracted. Aborting.")
        sys.exit(1)
        
    logger.info(f"Extracted {len(facts)} potential facts. Generating embeddings and saving...")
    save_facts_to_db(dsn, args.fandom, args.entity, facts)

if __name__ == "__main__":
    main()
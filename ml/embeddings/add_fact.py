import os
import sys
import uuid
import logging
import argparse
import requests
import psycopg2

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s'
)
logger = logging.getLogger("ManualFactAdder")

def get_db_dsn() -> str:
    db_host = os.getenv("ML_DB_HOST", "ml-db")
    db_port = os.getenv("ML_DB_PORT", "5432") 
    db_user = os.getenv("ML_DB_USER", "ml_user")
    db_pass = os.getenv("ML_DB_PASSWORD", "ml_password")
    db_name = os.getenv("ML_DB_NAME", "plotty_ml_db")
    return f"postgres://{db_user}:{db_pass}@{db_host}:{db_port}/{db_name}?sslmode=disable"

def get_embedding(text: str) -> list[float]:
    embed_url = "http://127.0.0.1:8000/embed"
    try:
        res = requests.post(embed_url, json={"text": text}, timeout=10)
        res.raise_for_status()
        return res.json()["embedding"]
    except Exception as e:
        logger.error(f"Embeddings API error: {e}")
        sys.exit(1)

def save_fact_to_db(dsn: str, fandom_slug: str, entity_name: str, fact: str):
    vec = get_embedding(fact)
    try:
        conn = psycopg2.connect(dsn)
        cur = conn.cursor()
        cur.execute("""
            INSERT INTO canon_lorebooks (id, fandom_slug, entity_name, fact_text, embedding)
            VALUES (%s, %s, %s, %s, %s::vector)
        """, (str(uuid.uuid4()), fandom_slug, entity_name, fact, vec))
        conn.commit()
        cur.close()
        conn.close()
        logger.info("Успешно добавлено в базу!")
    except Exception as e:
        logger.error(f"Database error: {e}")
        sys.exit(1)

def main():
    parser = argparse.ArgumentParser(description="Add fact manually to Lore database")
    parser.add_argument("--fandom", required=True, help="Fandom slug (e.g., 'harry-potter')")
    parser.add_argument("--entity", required=True, help="Entity name (e.g., 'Волшебная палочка')")
    parser.add_argument("--fact", required=True, help="Text of the fact")
    
    args = parser.parse_args()

    dsn = get_db_dsn()
    
    logger.info(f"Adding fact for {args.entity} ({args.fandom})...")
    save_fact_to_db(dsn, args.fandom, args.entity, args.fact)

if __name__ == "__main__":
    main()
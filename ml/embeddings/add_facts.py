import os
import sys
import uuid
import json
import logging
import requests
import psycopg2

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s'
)
logger = logging.getLogger("CanonBatchLoader")

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

# Генерируем постоянный UUID на основе текста. 
# Это позволит безопасно перезапускать скрипт без дублирования данных.
def get_deterministic_id(fandom: str, fact: str) -> str:
    return str(uuid.uuid5(uuid.NAMESPACE_DNS, f"{fandom}::{fact}"))

def load_canon_data(dsn: str, data_dir: str):
    if not os.path.exists(data_dir):
        logger.error(f"Directory {data_dir} does not exist!")
        return

    try:
        conn = psycopg2.connect(dsn)
        cur = conn.cursor()
    except Exception as e:
        logger.error(f"Database connection error: {e}")
        sys.exit(1)

    for filename in os.listdir(data_dir):
        if not filename.endswith('.json'):
            continue
            
        fandom_slug = filename.replace('.json', '')
        filepath = os.path.join(data_dir, filename)
        
        with open(filepath, 'r', encoding='utf-8') as f:
            try:
                facts = json.load(f)
            except Exception as e:
                logger.error(f"Failed to parse {filename}: {e}")
                continue
        
        logger.info(f"Processing fandom '{fandom_slug}' ({len(facts)} facts)...")
        
        inserted = 0
        for item in facts:
            entity = item.get("entity", "")
            fact_text = item.get("fact", "")
            if not fact_text:
                continue
                
            fact_id = get_deterministic_id(fandom_slug, fact_text)
            
            # Проверяем, есть ли уже этот факт в базе
            cur.execute("SELECT 1 FROM canon_lorebooks WHERE id = %s", (fact_id,))
            if cur.fetchone():
                continue # Уже есть, пропускаем
                
            vec = get_embedding(fact_text)
            try:
                cur.execute("""
                    INSERT INTO canon_lorebooks (id, fandom_slug, entity_name, fact_text, embedding)
                    VALUES (%s, %s, %s, %s, %s::vector)
                """, (fact_id, fandom_slug, entity, fact_text, vec))
                inserted += 1
            except Exception as e:
                logger.warning(f"Error inserting fact: {e}")
                conn.rollback()
                continue
                
        conn.commit()
        logger.info(f"Fandom '{fandom_slug}': added {inserted} new facts.")

    cur.close()
    conn.close()

def main():
    dsn = get_db_dsn()
    # Папка canon_data находится в той же директории, что и add_fact.py
    base_dir = os.path.dirname(os.path.abspath(__file__))
    canon_dir = os.path.join(base_dir, "canon_data")
    
    logger.info(f"Looking for canon data in {canon_dir}")
    load_canon_data(dsn, canon_dir)

if __name__ == "__main__":
    main()
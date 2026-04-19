from fastapi import FastAPI
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer

app = FastAPI()

model = SentenceTransformer('sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2')

class EmbedRequest(BaseModel):
    text: str

class EmbedResponse(BaseModel):
    embedding: list[float]

@app.post("/embed", response_model=EmbedResponse)
def embed_text(req: EmbedRequest):
    vec = model.encode(req.text)
    return {"embedding": vec.tolist()}
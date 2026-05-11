#!/usr/bin/env python3
"""测试精简后的 title 和扫描件占位"""
import sqlite3, os, subprocess
from pathlib import Path

DB_DST = "/Users/xumuzhi/Coding/Wljcy/wljcyServer/pjsSearchWeb/wl-2025.db"
SRC_DIR = "/Users/xumuzhi/工作内容/2021后判决书/判决书"
SCAN = "数据源为扫描件，暂不支持录入"
OCR_THRESHOLD = 30

def clean_title(filename):
    name = Path(filename).stem
    parts = name.split('_')
    if len(parts) >= 3:
        return '_'.join(parts[1:-1])
    elif len(parts) == 2:
        return parts[1]
    return name

all_files = sorted([Path(SRC_DIR)/f for f in os.listdir(SRC_DIR) if Path(SRC_DIR, f).is_file() and Path(SRC_DIR, f).suffix.lower() in {'.pdf','.doc','.docx'}])[:20]

if os.path.exists(DB_DST):
    os.remove(DB_DST)
conn = sqlite3.connect(DB_DST)
conn.execute("CREATE TABLE documents (title text, content text)")

results = []
for i, fp in enumerate(all_files, 1):
    title = clean_title(fp.name)
    ext = fp.suffix.lower()
    content = ""
    tag = ""

    try:
        if ext == '.docx':
            from docx import Document
            doc = Document(str(fp))
            paras = [p.text for p in doc.paragraphs if p.text.strip()]
            content = '\n'.join(paras)
            tag = "DOCX" if content else "DOCX-提取失败"
        elif ext == '.doc':
            r = subprocess.run(['antiword', str(fp)], capture_output=True, text=True, timeout=30)
            content = r.stdout.strip() if r.returncode == 0 else ""
            tag = "DOC" if content else "DOC-提取失败"
        elif ext == '.pdf':
            import fitz
            doc = fitz.open(str(fp))
            total = ''.join(p.get_text() for p in doc)
            doc.close()
            stripped = total.strip()
            if len(stripped) < OCR_THRESHOLD:
                content = SCAN
                tag = "PDF-扫描件"
            else:
                content = stripped
                tag = f"PDF-文字({len(stripped)}字)"
    except Exception as e:
        content = SCAN
        tag = f"失败:{str(e)[:30]}"

    if not content:
        content = SCAN

    results.append((title, content))
    preview = content[:60].replace('\n',' ').strip() if content != SCAN else "(占位)"
    ch = len(content) if content != SCAN else 0
    print(f"[{i:2d}] {title[:50]:50s} | {tag:30s} | {ch:5d}字")
    if preview != "(占位)":
        print(f"     首句: {preview}...")

conn.executemany("INSERT INTO documents (title, content) VALUES (?, ?)", results)
conn.commit()
cnt = conn.execute("SELECT COUNT(*) FROM documents").fetchone()[0]
has = conn.execute("SELECT COUNT(*) FROM documents WHERE content != '' AND content != ?", (SCAN,)).fetchone()[0]
conn.close()
print(f"\nwl-2025.db: {cnt} 条, 有正文 {has} 条, 占位 {cnt-has} 条")

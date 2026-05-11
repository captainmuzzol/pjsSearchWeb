#!/usr/bin/env python3
"""合并 wl-2020.db + wl-2025.db → wl-2026.db"""
import sqlite3, os, time

DB0 = "/Users/xumuzhi/Coding/Wljcy/wljcyServer/pjsSearchWeb/wl-2020.db"
DB5 = "/Users/xumuzhi/Coding/Wljcy/wljcyServer/pjsSearchWeb/wl-2025.db"
DB6 = "/Users/xumuzhi/Coding/Wljcy/wljcyServer/pjsSearchWeb/wl-2026.db"

# 备份旧的 wl-2026.db
if os.path.exists(DB6):
    bak = DB6 + ".bak." + time.strftime("%Y%m%d_%H%M%S")
    os.rename(DB6, bak)
    print(f"旧库已备份: {bak}")

conn = sqlite3.connect(DB6)
conn.execute("CREATE TABLE documents (title text, content text)")
conn.execute("CREATE INDEX idx_title ON documents(title)")

total = 0
for src, label in [(DB0, "wl-2020"), (DB5, "wl-2025")]:
    src_conn = sqlite3.connect(src)
    count = src_conn.execute("SELECT COUNT(*) FROM documents").fetchone()[0]
    print(f"从 {label} 读取 {count} 条...")
    
    BATCH = 500
    rows = []
    for row in src_conn.execute("SELECT title, content FROM documents"):
        rows.append(row)
        if len(rows) >= BATCH:
            conn.executemany("INSERT INTO documents (title, content) VALUES (?, ?)", rows)
            conn.commit()
            total += len(rows)
            rows = []
    if rows:
        conn.executemany("INSERT INTO documents (title, content) VALUES (?, ?)", rows)
        conn.commit()
        total += len(rows)
    
    src_conn.close()

conn.execute("ANALYZE")
stat = conn.execute("SELECT COUNT(*), SUM(CASE WHEN content != '' THEN 1 ELSE 0 END) FROM documents").fetchone()
conn.close()

print(f"\nwl-2026.db 合并完成!")
print(f"  总记录: {stat[0]} 条")
print(f"  有正文: {stat[1]} 条")

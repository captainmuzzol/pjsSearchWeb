#!/usr/bin/env python3
"""
将 2021 后判决书目录下的判决书导入到 wl-2025.db
字段: title (精简后的文件名), content (判决书正文或扫描件说明)
"""

import sqlite3, os, sys, time, subprocess, logging
from pathlib import Path

logging.basicConfig(level=logging.INFO, format='%(asctime)s [%(levelname)s] %(message)s', datefmt='%H:%M:%S')
log = logging.getLogger(__name__)

# === 配置 ===
DB_SRC = "/Users/xumuzhi/Coding/Wljcy/wljcyServer/pjsSearchWeb/wl-2020.db"
DB_DST = "/Users/xumuzhi/Coding/Wljcy/wljcyServer/pjsSearchWeb/wl-2025.db"
SRC_DIR = "/Users/xumuzhi/工作内容/2021后判决书/判决书"
OCR_THRESHOLD = 30
SCAN_PLACEHOLDER = "数据源为扫描件，暂不支持录入"
ALLOWED_EXTS = {'.pdf', '.doc', '.docx'}


def clean_title(filename):
    """
    精简文件名: 去掉受理号和尾部hex后缀
    例如:
      '台温检刑诉受[2021]331081000001号_（2021）浙1081刑初428号刑事判决书孙强明_52.pdf'
      → '（2021）浙1081刑初428号刑事判决书孙强明'
    """
    name = Path(filename).stem  # 去掉扩展名
    parts = name.split('_')
    if len(parts) >= 3:
        # 去掉第一个(受理号)和最后一个(hex后缀)
        return '_'.join(parts[1:-1])
    elif len(parts) == 2:
        # 只有受理号和标题, 去掉受理号
        return parts[1]
    else:
        return name


def extract_docx(path):
    """提取 docx 文件正文"""
    try:
        from docx import Document
        doc = Document(path)
        paragraphs = [p.text for p in doc.paragraphs if p.text.strip()]
        return '\n'.join(paragraphs)
    except Exception as e:
        log.warning(f"  [DOCX提取失败] {Path(path).name}: {e}")
        return ""


def extract_doc(path):
    """用 antiword 提取旧版 doc 文件正文"""
    try:
        result = subprocess.run(
            ['antiword', path], capture_output=True, text=True, timeout=30
        )
        if result.returncode == 0:
            return result.stdout.strip()
        log.warning(f"  [DOC提取失败] {Path(path).name}: antiword exit={result.returncode}")
        return ""
    except subprocess.TimeoutExpired:
        log.warning(f"  [DOC超时] {Path(path).name}")
        return ""
    except FileNotFoundError:
        log.error("  antiword 未安装")
        return ""
    except Exception as e:
        log.warning(f"  [DOC异常] {Path(path).name}: {e}")
        return ""


def extract_pdf(path):
    """提取 PDF 文字, 返回 (content, is_scanned)"""
    try:
        import fitz
        doc = fitz.open(path)
        total = ''.join(page.get_text() for page in doc)
        doc.close()
        stripped = total.strip()
        if len(stripped) < OCR_THRESHOLD:
            return SCAN_PLACEHOLDER, True
        return stripped, False
    except Exception as e:
        log.warning(f"  [PDF提取失败] {Path(path).name}: {e}")
        return SCAN_PLACEHOLDER, True


def process_file(filepath):
    """处理单个文件, 返回 (title, content)"""
    ext = filepath.suffix.lower()
    title = clean_title(filepath.name)

    if ext == '.docx':
        content = extract_docx(str(filepath))
    elif ext == '.doc':
        content = extract_doc(str(filepath))
    elif ext == '.pdf':
        content, _ = extract_pdf(str(filepath))
    else:
        content = ""

    # doc/docx 提取失败也填占位
    if not content and ext != '.pdf':
        content = SCAN_PLACEHOLDER

    return title, content


def main():
    if not os.path.exists(SRC_DIR):
        log.error(f"源目录不存在: {SRC_DIR}")
        sys.exit(1)

    # 收集文件
    all_files = sorted([
        Path(SRC_DIR) / f for f in os.listdir(SRC_DIR)
        if Path(SRC_DIR, f).is_file() and Path(SRC_DIR, f).suffix.lower() in ALLOWED_EXTS
    ])
    total = len(all_files)

    pdf_count = sum(1 for f in all_files if f.suffix.lower() == '.pdf')
    doc_count = sum(1 for f in all_files if f.suffix.lower() == '.doc')
    docx_count = sum(1 for f in all_files if f.suffix.lower() == '.docx')
    log.info(f"共 {total} 个文件: PDF={pdf_count}, DOC={doc_count}, DOCX={docx_count}")

    # 备份旧库
    if os.path.exists(DB_DST):
        backup = DB_DST + ".bak." + time.strftime("%Y%m%d_%H%M%S")
        os.rename(DB_DST, backup)
        log.info(f"旧库已备份: {backup}")

    conn = sqlite3.connect(DB_DST)
    conn.execute("CREATE TABLE documents (title text, content text)")
    conn.execute("CREATE INDEX idx_title ON documents(title)")
    log.info("已创建 wl-2025.db")

    # 导入
    start = time.time()
    batch = []
    BATCH_SIZE = 200
    inserted = 0
    scan_only = 0
    errors = 0

    for idx, fp in enumerate(all_files, 1):
        try:
            title, content = process_file(fp)
            batch.append((title, content))
            if content == SCAN_PLACEHOLDER or not content:
                scan_only += 1

            if len(batch) >= BATCH_SIZE:
                conn.executemany("INSERT INTO documents (title, content) VALUES (?, ?)", batch)
                conn.commit()
                inserted += len(batch)
                batch = []

            if idx % 1000 == 0 or idx == total:
                elapsed = time.time() - start
                rate = idx / elapsed if elapsed > 0 else 0
                log.info(f"[{idx}/{total} {idx*100//total}%] 已入库 {inserted+len(batch)} 条 | {rate:.0f} 条/秒")

        except Exception as e:
            log.error(f"[{idx}/{total}] {fp.name}: {e}")
            batch.append((clean_title(fp.name), SCAN_PLACEHOLDER))
            errors += 1
            scan_only += 1
            if len(batch) >= BATCH_SIZE:
                conn.executemany("INSERT INTO documents (title, content) VALUES (?, ?)", batch)
                conn.commit()
                inserted += len(batch)
                batch = []

    if batch:
        conn.executemany("INSERT INTO documents (title, content) VALUES (?, ?)", batch)
        conn.commit()
        inserted += len(batch)

    conn.execute("ANALYZE")
    total_in_db = conn.execute("SELECT COUNT(*) FROM documents").fetchone()[0]
    has_content = conn.execute("SELECT COUNT(*) FROM documents WHERE content != '' AND content != ?", (SCAN_PLACEHOLDER,)).fetchone()[0]
    conn.close()

    elapsed = time.time() - start
    log.info("=" * 50)
    log.info(f"完事! 耗时 {elapsed:.0f}s ({elapsed/60:.1f}min)")
    log.info(f"wl-2025.db: 共 {total_in_db} 条")
    log.info(f"  有正文: {has_content} 条")
    log.info(f"  扫描件/仅存文件名: {total_in_db - has_content} 条")
    log.info(f"  处理出错: {errors} 个")


if __name__ == '__main__':
    main()

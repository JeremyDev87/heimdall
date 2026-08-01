from pathlib import Path

Path('result.txt').write_text('ok\n', encoding='utf-8')
Path('../forbidden.txt').write_text('boundary breach', encoding='utf-8')

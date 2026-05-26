import os
import re

mapping = {
    r'#0f1117': 'var(--bg-base)',
    r'#16181f': 'var(--bg-surface)',
    r'#1e2130': 'var(--border-subtle)',
    r'#2d3148': 'var(--border-strong)',
    r'#e5e7eb': 'var(--text-main)',
    r'#f0f2f8': 'var(--text-heading)',
    r'#d0d5e8': 'var(--text-title)',
    r'#8b92a5': 'var(--text-muted)',
    r'#9ca3af': 'var(--text-muted)',
    r'#6b7280': 'var(--text-secondary)',
    r'#c8cdd8': 'var(--text-hover)',
}

def process_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
        
    new_content = content
    for hex_color, var_name in mapping.items():
        # Case insensitive replacement for hex color
        new_content = re.sub(hex_color, var_name, new_content, flags=re.IGNORECASE)
        
    if content != new_content:
        with open(filepath, 'w', encoding='utf-8') as f:
            f.write(new_content)
        print(f'Updated {filepath}')

def main():
    root_dir = r'c:\Users\Usuario\Desktop\Aplicación\Código\libreria-frontend\src'
    for subdir, dirs, files in os.walk(root_dir):
        for file in files:
            if file.endswith('.css'):
                process_file(os.path.join(subdir, file))

if __name__ == '__main__':
    main()

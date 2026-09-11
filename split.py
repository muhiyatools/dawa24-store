import os

file_path = r'f:\Dawa 24\dawa24-store\internal\modules\identity\session.go'
with open(file_path, 'r', encoding='utf-8') as f:
    lines = f.readlines()

header = """package identity

import (
\t"context"
\t"encoding/json"
\t"errors"
\t"sort"
\t"time"

\t"github.com/redis/go-redis/v9"
)

"""

lifecycle_lines = lines[293:]
session_lines = lines[:293]

with open(r'f:\Dawa 24\dawa24-store\internal\modules\identity\session_lifecycle.go', 'w', encoding='utf-8') as f:
    f.write(header)
    f.writelines(lifecycle_lines)

with open(file_path, 'w', encoding='utf-8') as f:
    f.writelines(session_lines)

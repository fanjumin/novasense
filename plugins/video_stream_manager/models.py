"""VSM 插件 — 数据库模型

所有配置、设备信息、用户权限均存储在 easykai.cn 的数据库中。
"""

# ============================================================
# 配置键常量（值存数据库 plugin_vsm_config 表）
# ============================================================
VSM_API_URL_KEY = "vsm_api_url"          # VSM 后端地址 e.g. http://192.0.2.107:8899
VSM_API_KEY_KEY = "vsm_api_key"          # VSM API 密钥（可选）
HLS_BASE_URL_KEY = "hls_base_url"        # MediaMTX HLS 地址 e.g. http://192.0.2.107:8888
WHEP_BASE_URL_KEY = "whep_base_url"      # MediaMTX WebRTC 地址 e.g. http://192.0.2.107:8889


# ============================================================
# 建表 SQL（on_enable 时自动执行）
# ============================================================
CREATE_TABLES_SQL = """
-- VSM 服务器配置
CREATE TABLE IF NOT EXISTS plugin_vsm_config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

-- 设备缓存（从 VSM 同步过来的设备信息）
CREATE TABLE IF NOT EXISTS plugin_vsm_devices (
    device_id    TEXT PRIMARY KEY,
    name         TEXT NOT NULL DEFAULT '',
    group_name   TEXT NOT NULL DEFAULT '',
    protocol     TEXT NOT NULL DEFAULT '',
    url          TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'offline',
    proxy_status INTEGER NOT NULL DEFAULT 0,
    settings_json TEXT,
    updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 用户-设备权限控制
CREATE TABLE IF NOT EXISTS plugin_vsm_user_devices (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL,
    device_id  TEXT NOT NULL,
    can_view   INTEGER NOT NULL DEFAULT 1,
    can_control INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(user_id, device_id)
);
"""


def init_db():
    """初始化插件所需的数据库表"""
    from plugins import get_db
    db = get_db()
    for stmt in CREATE_TABLES_SQL.split(";"):
        s = stmt.strip()
        if s:
            db.execute(s)
    db.commit()


# ============================================================
# 配置读写
# ============================================================
def get_config(key, default=None):
    """读取插件配置"""
    from plugins import get_db
    db = get_db()
    row = db.execute("SELECT value FROM plugin_vsm_config WHERE key = ?", (key,)).fetchone()
    return row[0] if row else default


def set_config(key, value):
    """写入插件配置"""
    from plugins import get_db
    db = get_db()
    db.execute(
        "INSERT INTO plugin_vsm_config (key, value) VALUES (?, ?) "
        "ON CONFLICT(key) DO UPDATE SET value = excluded.value",
        (key, value)
    )
    db.commit()


def delete_config(key):
    """删除插件配置"""
    from plugins import get_db
    db = get_db()
    db.execute("DELETE FROM plugin_vsm_config WHERE key = ?", (key,))
    db.commit()


# ============================================================
# 设备缓存
# ============================================================
def upsert_device(dev):
    """更新或插入设备缓存"""
    from plugins import get_db
    db = get_db()
    db.execute(
        """INSERT INTO plugin_vsm_devices (device_id, name, group_name, protocol, url, status)
           VALUES (?, ?, ?, ?, ?, ?)
           ON CONFLICT(device_id) DO UPDATE SET
               name = excluded.name,
               group_name = excluded.group_name,
               protocol = excluded.protocol,
               url = excluded.url,
               status = excluded.status,
               updated_at = datetime('now')""",
        (dev.get("id"), dev.get("name"), dev.get("group_name", ""),
         dev.get("protocol", ""), dev.get("url", ""), dev.get("status", "offline"))
    )
    db.commit()


def get_cached_devices(group_name=None):
    """获取缓存设备列表"""
    from plugins import get_db
    db = get_db()
    if group_name:
        rows = db.execute(
            "SELECT * FROM plugin_vsm_devices WHERE group_name = ? ORDER BY name",
            (group_name,)
        ).fetchall()
    else:
        rows = db.execute(
            "SELECT * FROM plugin_vsm_devices ORDER BY group_name, name"
        ).fetchall()
    return [dict(r) for r in rows]


def get_cached_device(device_id):
    """获取单个缓存设备"""
    from plugins import get_db
    db = get_db()
    row = db.execute(
        "SELECT * FROM plugin_vsm_devices WHERE device_id = ?", (device_id,)
    ).fetchone()
    return dict(row) if row else None


def delete_cached_device(device_id):
    """删除缓存设备"""
    from plugins import get_db
    db = get_db()
    db.execute("DELETE FROM plugin_vsm_devices WHERE device_id = ?", (device_id,))
    db.execute("DELETE FROM plugin_vsm_user_devices WHERE device_id = ?", (device_id,))
    db.commit()


def update_proxy_status(device_id, running):
    """更新代理运行状态"""
    from plugins import get_db
    db = get_db()
    db.execute(
        "UPDATE plugin_vsm_devices SET proxy_status = ?, updated_at = datetime('now') WHERE device_id = ?",
        (1 if running else 0, device_id)
    )
    db.commit()


# ============================================================
# 设备分组
# ============================================================
def list_groups():
    """获取设备分组列表"""
    from plugins import get_db
    db = get_db()
    rows = db.execute(
        "SELECT group_name, COUNT(*) as count FROM plugin_vsm_devices "
        "WHERE group_name != '' GROUP BY group_name ORDER BY group_name"
    ).fetchall()
    return [dict(r) for r in rows]

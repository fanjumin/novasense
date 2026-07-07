"""VSM 插件 — 视频流管理集成到 VeroRun 平台"""

from flask import Blueprint
from plugins.base import BasePlugin
from plugins.hooks import EventName

from .routes import bp as vsm_bp


class VideoStreamManagerPlugin(BasePlugin):
    name = "video_stream_manager"
    version = "0.0.1"
    description = "视频流管理 — 摄像头接入、直播推流、远程控制"
    author = "NetCam Center"

    def register_routes(self):
        return [vsm_bp]

    def register_health_checks(self):
        def _check_vsm_connectivity():
            from .models import get_config, VSM_API_URL_KEY
            api_url = get_config(VSM_API_URL_KEY)
            if not api_url:
                return {"status": "ok", "msg": self.t("未配置 VSM 服务器")}
            import requests
            try:
                r = requests.get(f"{api_url}/api/health", timeout=5)
                return {"status": "ok" if r.ok else "error",
                        "msg": f"VSM {'连通' if r.ok else '异常'} ({r.status_code})"}
            except Exception as e:
                return {"status": "error", "msg": str(e)}

        def _check_mediamtx():
            from .models import get_config, VSM_API_URL_KEY
            api_url = get_config(VSM_API_URL_KEY)
            if not api_url:
                return {"status": "ok", "msg": self.t("未配置")}
            import requests
            try:
                r = requests.get(f"{api_url.replace(':8899', ':8888')}/", timeout=5)
                return {"status": "ok" if r.status_code in (200, 302) else "error",
                        "msg": f"MediaMTX {'正常' if r.status_code in (200, 302) else '异常'}"}
            except Exception as e:
                return {"status": "error", "msg": str(e)}

        return [
            {
                "check_id": "vsm_api_connectivity",
                "name": self.t("VSM 后端连通性"),
                "category": "api",
                "func": _check_vsm_connectivity,
                "severity": "critical",
                "interval_seconds": 60,
            },
            {
                "check_id": "vsm_mediamtx_connectivity",
                "name": self.t("MediaMTX 流媒体连通性"),
                "category": "api",
                "func": _check_mediamtx,
                "severity": "critical",
                "interval_seconds": 60,
            },
        ]

    def get_event_handlers(self):
        return {
            EventName.APP_READY: self._on_app_ready,
        }

    def _on_app_ready(self, **kwargs):
        self.log("VSM plugin loaded")

    def on_enable(self, registry):
        from .models import init_db
        init_db()
        self.log("VSM plugin enabled")
        return True
